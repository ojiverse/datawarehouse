// Package spike runs the #36 closed loop:
// Archive fixture → Go materializer → Parquet → iceberg-go → REST Catalog →
// R2 SQL → drop → rebuild → semantic equivalence, and records evidence for
// every Success Criterion of the issue.
package spike

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/apache/iceberg-go/table"
	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/canonical"
	"github.com/ojiverse/datawarehouse/internal/equivalence"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
	"github.com/ojiverse/datawarehouse/internal/r2sql"
)

// Config is the full loop configuration.
type Config struct {
	Archive       archive.Config
	ArchivePrefix string
	Catalog       icebergcat.Config
	TableName     string
	// R2SQL is optional: nil skips the R2 SQL criteria (local catalog runs).
	R2SQL *r2sql.Config
	// R2SQLTable is the fully qualified name R2 SQL expects (namespace.table).
	R2SQLTable string
	Fixture    observation.FixtureSpec
	// ContractFixtureDir holds the shared cross-language Envelope fixtures
	// (contracts/observation-envelope/v1); they are archived alongside the
	// generated pages so the loop consumes the authoritative v1 documents.
	ContractFixtureDir string
	// DiscordEnvKeys are environment variables that must be absent during rebuild.
	DiscordEnvKeys []string
}

// Criterion is one Success Criterion result with evidence.
type Criterion struct {
	Name     string `json:"name"`
	Passed   bool   `json:"passed"`
	Skipped  bool   `json:"skipped,omitempty"`
	Evidence string `json:"evidence"`
}

// StepTiming records wall time of one loop step.
type StepTiming struct {
	Step       string `json:"step"`
	WallMillis int64  `json:"wall_ms"`
}

// Report is the machine-readable outcome.
type Report struct {
	RunID          string       `json:"run_id"`
	RebuildRunID   string       `json:"rebuild_run_id"`
	Criteria       []Criterion  `json:"criteria"`
	Steps          []StepTiming `json:"steps"`
	CPUUser        string       `json:"cpu_user"`
	CPUSystem      string       `json:"cpu_system"`
	MaxRSSBytes    int64        `json:"max_rss_bytes"`
	HeapSysBytes   uint64       `json:"heap_sys_bytes"`
	GoVersion      string       `json:"go_version"`
	R2SQLRawSample string       `json:"r2sql_raw_sample,omitempty"`
	Failure        string       `json:"failure,omitempty"`
}

// AllPassed is true when no non-skipped criterion failed.
func (r Report) AllPassed() bool {
	for _, c := range r.Criteria {
		if !c.Skipped && !c.Passed {
			return false
		}
	}
	return r.Failure == ""
}

type runner struct {
	cfg    Config
	log    *slog.Logger
	arch   *archive.Client
	cat    *icebergcat.Client
	sql    *r2sql.Client
	report *Report
}

func (r *runner) pass(name, evidence string) {
	r.report.Criteria = append(r.report.Criteria, Criterion{Name: name, Passed: true, Evidence: evidence})
	r.log.Info("criterion passed", slog.String("criterion", name), slog.String("evidence", evidence))
}

func (r *runner) skip(name, why string) {
	r.report.Criteria = append(r.report.Criteria, Criterion{Name: name, Skipped: true, Evidence: why})
}

func (r *runner) fail(name string, err error) error {
	r.report.Criteria = append(r.report.Criteria, Criterion{Name: name, Passed: false, Evidence: err.Error()})
	r.log.Error("criterion failed", slog.String("criterion", name), slog.String("error", err.Error()))
	return fmt.Errorf("%s: %w", name, err)
}

func (r *runner) timed(step string, fn func() error) error {
	start := time.Now()
	err := fn()
	r.report.Steps = append(r.report.Steps, StepTiming{Step: step, WallMillis: time.Since(start).Milliseconds()})
	return err
}

// Run executes the loop; the Report is always returned, even on failure, so
// partial evidence survives for the Failure Rule of #36.
func Run(ctx context.Context, cfg Config, log *slog.Logger) (*Report, error) {
	rep := &Report{GoVersion: runtime.Version()}
	r := &runner{cfg: cfg, log: log, report: rep}
	err := r.run(ctx)
	if err != nil {
		rep.Failure = err.Error()
	}
	r.collectResources()
	return rep, err
}

func (r *runner) run(ctx context.Context) error {
	var err error
	r.arch, err = archive.New(r.cfg.Archive, r.log)
	if err != nil {
		return err
	}
	r.cat, err = icebergcat.New(ctx, r.cfg.Catalog, r.log)
	if err != nil {
		return r.fail("connect_rest_catalog", err)
	}
	if r.cfg.R2SQL != nil {
		r.sql, err = r2sql.New(*r.cfg.R2SQL)
		if err != nil {
			return err
		}
	}
	runID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	r.report.RunID = runID.String()

	// 1. Envelope v1 fixture → Archive (create-only put + idempotent retry).
	var envs []observation.Envelope
	if err := r.timed("archive_put", func() error {
		envs, err = observation.GenerateFixture(r.cfg.Fixture)
		if err != nil {
			return err
		}
		shared := 0
		if r.cfg.ContractFixtureDir != "" {
			contract, err := observation.LoadContractFixtures(r.cfg.ContractFixtureDir)
			if err != nil {
				return fmt.Errorf("load contract fixtures: %w", err)
			}
			shared = len(contract)
			envs = append(envs, contract...)
		}
		created := 0
		for _, e := range envs {
			res, err := r.arch.PutCreateOnly(ctx, e)
			if err != nil {
				return err
			}
			if res == archive.PutCreated {
				created++
			}
		}
		retry, err := r.arch.PutCreateOnly(ctx, envs[0])
		if err != nil {
			return err
		}
		if retry != archive.PutIdempotentRetry {
			return fmt.Errorf("retry of existing object returned %s", retry)
		}
		r.pass("archive_put_envelope_v1", fmt.Sprintf("%d of %d objects created under %s (%d shared contract fixtures included); retry of %s was idempotent", created, len(envs), r.cfg.ArchivePrefix, shared, envs[0].ArchiveKey()))
		return nil
	}); err != nil {
		return r.fail("archive_put_envelope_v1", err)
	}

	// 2. Materialize from the Archive listing only.
	buildA, err := r.materialize(ctx, r.report.RunID, "materialize_parquet_from_archive")
	if err != nil {
		return err
	}

	// 3-6. Catalog connect, namespace/table, commit, duplicate protection.
	var tbl *table.Table
	if err := r.timed("catalog_namespace_table", func() error {
		if err := r.cat.EnsureNamespace(ctx); err != nil {
			return err
		}
		tbl, err = r.cat.EnsureTable(ctx, r.cfg.TableName)
		return err
	}); err != nil {
		return r.fail("catalog_connect_and_create", err)
	}
	r.pass("rest_catalog_connect", "iceberg-go rest.Catalog connected to "+r.cfg.Catalog.URI)
	r.pass("namespace_and_table_created", fmt.Sprintf("%s.%s at %s", r.cfg.Catalog.Namespace, r.cfg.TableName, tbl.Location()))

	if err := r.commitAndVerifyDedup(ctx, tbl, buildA, true); err != nil {
		return err
	}

	// 7. Query: iceberg-go scan (engine independent) and R2 SQL.
	var rowsA []equivalence.Row
	if err := r.timed("scan_iceberg_go", func() error {
		rowsA, err = icebergcat.ScanRows(ctx, tbl)
		return err
	}); err != nil {
		return r.fail("iceberg_go_scan", err)
	}
	if d := equivalence.Diff(equivalence.FromCanonical(buildA.rows), rowsA); len(d) != 0 {
		return r.fail("iceberg_go_scan", fmt.Errorf("scan differs from projection: %d lines, first %q", len(d), d[0]))
	}
	r.pass("iceberg_go_scan_matches_projection", fmt.Sprintf("%d rows, digest %s", len(rowsA), equivalence.Digest(rowsA)))
	if err := r.queryR2SQL(ctx, buildA.rows, "r2_sql_query"); err != nil {
		return err
	}

	// 8. Delete.
	if err := r.timed("drop_table", func() error {
		if err := r.cat.DropTable(ctx, r.cfg.TableName); err != nil {
			return err
		}
		exists, err := r.cat.TableExists(ctx, r.cfg.TableName)
		if err != nil {
			return err
		}
		if exists {
			return errors.New("table still exists after drop")
		}
		return nil
	}); err != nil {
		return r.fail("canonical_table_deleted", err)
	}
	r.pass("canonical_table_deleted", "drop/purge confirmed via CheckTableExists=false")

	// 9-10. Rebuild without Discord credentials, then compare.
	for _, k := range r.cfg.DiscordEnvKeys {
		if _, set := os.LookupEnv(k); set {
			return r.fail("rebuild_without_discord", fmt.Errorf("environment variable %s is set; rebuild must run without Discord credentials", k))
		}
	}
	rebuildID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	r.report.RebuildRunID = rebuildID.String()
	buildB, err := r.materialize(ctx, r.report.RebuildRunID, "rebuild_materialize")
	if err != nil {
		return err
	}
	if buildB.manifestDigest != buildA.manifestDigest {
		return r.fail("rebuild_without_discord", fmt.Errorf("archive manifest changed between builds (%s vs %s)", buildA.manifestDigest, buildB.manifestDigest))
	}
	if err := r.timed("rebuild_table", func() error {
		tbl, err = r.cat.EnsureTable(ctx, r.cfg.TableName)
		return err
	}); err != nil {
		return r.fail("rebuild_without_discord", err)
	}
	if err := r.commitAndVerifyDedup(ctx, tbl, buildB, false); err != nil {
		return err
	}
	r.pass("rebuild_without_discord", fmt.Sprintf("rebuilt from %d archive objects with %v unset", buildB.objectCount, r.cfg.DiscordEnvKeys))

	var rowsB []equivalence.Row
	if err := r.timed("scan_rebuild", func() error {
		rowsB, err = icebergcat.ScanRows(ctx, tbl)
		return err
	}); err != nil {
		return r.fail("rebuild_semantic_equivalence", err)
	}
	if d := equivalence.Diff(rowsA, rowsB); len(d) != 0 {
		return r.fail("rebuild_semantic_equivalence", fmt.Errorf("%d differing lines, first %q", len(d), d[0]))
	}
	r.pass("rebuild_semantic_equivalence", fmt.Sprintf("digest before %s == after %s (%d rows); chunk id %s == %s",
		equivalence.Digest(rowsA), equivalence.Digest(rowsB), len(rowsB), buildA.chunkID, buildB.chunkID))
	return r.queryR2SQL(ctx, buildB.rows, "r2_sql_query_after_rebuild")
}

type build struct {
	runID          string
	rows           []projection.CanonicalMessage
	parquet        []byte
	chunkID        string
	objectCount    int
	manifestDigest string
}

// materialize lists the Archive, fixes the manifest, projects and writes Parquet.
func (r *runner) materialize(ctx context.Context, runID, criterion string) (*build, error) {
	b := &build{runID: runID}
	err := r.timed(criterion, func() error {
		keys, err := r.arch.List(ctx, r.cfg.ArchivePrefix)
		if err != nil {
			return err
		}
		if len(keys) == 0 {
			return fmt.Errorf("no archive objects under %s", r.cfg.ArchivePrefix)
		}
		envs := make([]observation.Envelope, 0, len(keys))
		ids := make([]observation.ObservationID, 0, len(keys))
		for _, k := range keys {
			e, err := r.arch.Get(ctx, k)
			if err != nil {
				return err
			}
			envs = append(envs, e)
			ids = append(ids, e.ObservationID)
		}
		b.objectCount = len(keys)
		b.chunkID = canonical.ChunkID(ids)
		b.manifestDigest = b.chunkID
		b.rows, err = projection.Project(envs)
		if err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := canonical.WriteParquet(&buf, b.rows); err != nil {
			return err
		}
		n, codecs, err := canonical.InspectParquet(bytes.NewReader(buf.Bytes()))
		if err != nil {
			return err
		}
		if _, ok := codecs["ZSTD"]; !ok || len(codecs) != 1 {
			return fmt.Errorf("parquet codecs %v, want ZSTD only", codecs)
		}
		b.parquet = buf.Bytes()
		r.pass(criterion, fmt.Sprintf("%d archive objects → %d canonical rows → %d-byte zstd parquet (%d rows), chunk %s", len(keys), len(b.rows), len(b.parquet), n, b.chunkID[:12]))
		return nil
	})
	if err != nil {
		return nil, r.fail(criterion, err)
	}
	return b, nil
}

// commitAndVerifyDedup writes the staging file, commits it, and proves that a
// retry (both through our guard and through iceberg-go itself) never
// registers the same data file twice.
func (r *runner) commitAndVerifyDedup(ctx context.Context, tbl *table.Table, b *build, verifyDedup bool) error {
	path := canonical.StagingPath(tbl.Location(), b.runID, b.chunkID)
	var first icebergcat.CommitResult
	if err := r.timed("commit_"+b.runID[:8], func() error {
		if err := r.cat.WriteStaging(ctx, tbl, path, b.parquet); err != nil {
			return err
		}
		var err error
		first, err = r.cat.CommitFiles(ctx, tbl, []string{path})
		if err != nil {
			return err
		}
		if first.Status != icebergcat.Committed {
			return fmt.Errorf("first commit status %s", first.Status)
		}
		return nil
	}); err != nil {
		return r.fail("parquet_committed_to_iceberg", err)
	}
	if !verifyDedup {
		return nil
	}
	r.pass("parquet_committed_to_iceberg", fmt.Sprintf("snapshot %d references %d data file(s): %s", first.SnapshotID, first.DataFiles, path))

	if err := r.timed("commit_retry", func() error {
		// Simulated crash between commit and checkpoint: reload from catalog.
		reloaded, err := r.cat.LoadTable(ctx, r.cfg.TableName)
		if err != nil {
			return err
		}
		again, err := r.cat.CommitFiles(ctx, reloaded, []string{path})
		if err != nil {
			return err
		}
		if again.Status != icebergcat.AlreadyReferenced || again.SnapshotID != first.SnapshotID || again.DataFiles != first.DataFiles {
			return fmt.Errorf("retry produced %+v, want already_referenced at snapshot %d", again, first.SnapshotID)
		}
		// Library-level check: iceberg-go itself must reject the duplicate.
		rawErr := r.cat.AddFilesRaw(ctx, reloaded, []string{path})
		if rawErr == nil || !strings.Contains(rawErr.Error(), "already referenced") {
			return fmt.Errorf("iceberg-go AddFiles accepted a duplicate data file (err=%v)", rawErr)
		}
		final, err := r.cat.LoadTable(ctx, r.cfg.TableName)
		if err != nil {
			return err
		}
		if final.CurrentSnapshot() == nil || final.CurrentSnapshot().SnapshotID != first.SnapshotID {
			return errors.New("snapshot changed during retry")
		}
		refs, err := r.cat.ReferencedFiles(ctx, final)
		if err != nil {
			return err
		}
		if len(refs) != first.DataFiles {
			return fmt.Errorf("data files %d after retry, want %d", len(refs), first.DataFiles)
		}
		return nil
	}); err != nil {
		return r.fail("commit_retry_no_duplicate_file", err)
	}
	r.pass("commit_retry_no_duplicate_file", fmt.Sprintf("retry after reload: already_referenced; iceberg-go AddFiles rejected duplicate; snapshot stayed %d", first.SnapshotID))
	return nil
}

// queryR2SQL runs count and projection queries through R2 SQL and checks the
// domain identity set (message_id, observation_id, content) against the local
// projection. Timestamps are not compared here because R2 SQL's JSON encoding
// of timestamps is undocumented (beta); the iceberg-go scan covers them.
func (r *runner) queryR2SQL(ctx context.Context, rows []projection.CanonicalMessage, criterion string) error {
	if r.sql == nil {
		r.skip(criterion, "R2 SQL config not provided (local catalog run)")
		return nil
	}
	err := r.timed(criterion, func() error {
		var res r2sql.Result
		var err error
		// R2 SQL reads catalog metadata asynchronously from the commit, so a
		// short bounded retry absorbs propagation delay without hiding errors.
		for attempt := 1; attempt <= 6; attempt++ {
			res, err = r.sql.Query(ctx, fmt.Sprintf("SELECT message_id, observation_id, content FROM %s ORDER BY message_id LIMIT 500", r.cfg.R2SQLTable))
			if err == nil && len(res.Rows) == len(rows) {
				break
			}
			r.log.Warn("r2 sql not consistent yet", slog.Int("attempt", attempt), slog.Int("rows", len(res.Rows)), slog.Any("error", err))
			time.Sleep(time.Duration(attempt) * 5 * time.Second)
		}
		if err != nil {
			return err
		}
		if r.report.R2SQLRawSample == "" {
			raw := string(res.Raw)
			if len(raw) > 600 {
				raw = raw[:600] + "..."
			}
			r.report.R2SQLRawSample = raw
		}
		want := map[string]struct{}{}
		for _, m := range rows {
			want[m.MessageID+"|"+m.ObservationID.String()+"|"+m.Content] = struct{}{}
		}
		got := map[string]struct{}{}
		for _, row := range res.Rows {
			got[fmt.Sprintf("%v|%v|%v", row["message_id"], row["observation_id"], row["content"])] = struct{}{}
		}
		if len(got) != len(want) {
			return fmt.Errorf("r2 sql returned %d distinct rows, want %d", len(got), len(want))
		}
		for k := range want {
			if _, ok := got[k]; !ok {
				return fmt.Errorf("r2 sql missing row %q", k)
			}
		}
		cnt, err := r.sql.Query(ctx, fmt.Sprintf("SELECT COUNT(*) AS n FROM %s", r.cfg.R2SQLTable))
		if err != nil {
			return err
		}
		r.pass(criterion, fmt.Sprintf("%d rows matched projection identity set; COUNT(*) response %s; latency %s", len(res.Rows), strings.TrimSpace(string(cnt.Raw)), res.Elapsed))
		return nil
	})
	if err != nil {
		return r.fail(criterion, err)
	}
	return nil
}

func (r *runner) collectResources() {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err == nil {
		r.report.CPUUser = time.Duration(ru.Utime.Nano()).String()
		r.report.CPUSystem = time.Duration(ru.Stime.Nano()).String()
		r.report.MaxRSSBytes = maxRSSBytes(ru.Maxrss)
	}
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	r.report.HeapSysBytes = ms.HeapSys
}

// Cleanup drops the table and deletes fixture objects (spike hygiene only).
func Cleanup(ctx context.Context, cfg Config, log *slog.Logger) error {
	arch, err := archive.New(cfg.Archive, log)
	if err != nil {
		return err
	}
	keys, err := arch.List(ctx, cfg.ArchivePrefix)
	if err != nil {
		return err
	}
	for _, k := range keys {
		if err := arch.Delete(ctx, k); err != nil {
			return err
		}
	}
	log.Info("archive objects deleted", slog.Int("count", len(keys)), slog.String("prefix", cfg.ArchivePrefix))
	cat, err := icebergcat.New(ctx, cfg.Catalog, log)
	if err != nil {
		return err
	}
	if err := cat.DropTable(ctx, cfg.TableName); err != nil {
		return err
	}
	log.Info("table dropped", slog.String("table", cfg.TableName))
	return nil
}
