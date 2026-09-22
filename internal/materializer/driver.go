package materializer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/apache/iceberg-go/table"
	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/canonical"
	"github.com/ojiverse/datawarehouse/internal/equivalence"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

// ArchiveReader is the subset of *archive.Client the driver needs. It is an
// interface so the resumability/crash-recovery logic can be unit tested
// against an in-memory fake without R2/MinIO; the concrete *archive.Client
// (a fixed dependency, see AGENTS.md) satisfies it unchanged.
type ArchiveReader interface {
	Get(ctx context.Context, key string) (observation.Envelope, error)
}

// TableRef is the minimal handle the driver needs for a loaded Iceberg table.
// It exists so CatalogClient can be faked in unit tests without constructing
// a real *table.Table (which only iceberg-go's own catalog/table-file
// machinery can build). *table.Table already has a Location method, so
// icebergcat.Client's production methods satisfy this via catalogAdapter
// below without changing icebergcat's own public API.
type TableRef interface {
	Location() string
}

// CatalogClient is the subset of *icebergcat.Client the driver needs,
// expressed over TableRef instead of the concrete *table.Table so it can be
// faked in unit tests.
type CatalogClient interface {
	EnsureNamespace(ctx context.Context) error
	EnsureTable(ctx context.Context, name string) (TableRef, error)
	WriteStaging(ctx context.Context, tbl TableRef, path string, data []byte) error
	CommitFiles(ctx context.Context, tbl TableRef, paths []string) (icebergcat.CommitResult, error)
}

// catalogAdapter adapts *icebergcat.Client (whose methods take/return the
// concrete *table.Table) to CatalogClient, without changing icebergcat's
// public API. It downcasts TableRef back to *table.Table, which is safe
// because every TableRef this package hands back to a caller-supplied
// CatalogClient originated from this same adapter's EnsureTable.
type catalogAdapter struct{ c *icebergcat.Client }

// NewCatalogClient wraps a production *icebergcat.Client for use with Run.
func NewCatalogClient(c *icebergcat.Client) CatalogClient { return catalogAdapter{c: c} }

func (a catalogAdapter) EnsureNamespace(ctx context.Context) error { return a.c.EnsureNamespace(ctx) }

func (a catalogAdapter) EnsureTable(ctx context.Context, name string) (TableRef, error) {
	return a.c.EnsureTable(ctx, name)
}

func (a catalogAdapter) WriteStaging(ctx context.Context, tbl TableRef, path string, data []byte) error {
	t, err := asTable(tbl)
	if err != nil {
		return err
	}
	return a.c.WriteStaging(ctx, t, path, data)
}

func (a catalogAdapter) CommitFiles(ctx context.Context, tbl TableRef, paths []string) (icebergcat.CommitResult, error) {
	t, err := asTable(tbl)
	if err != nil {
		return icebergcat.CommitResult{}, err
	}
	return a.c.CommitFiles(ctx, t, paths)
}

func asTable(ref TableRef) (*table.Table, error) {
	t, ok := ref.(*table.Table)
	if !ok {
		return nil, fmt.Errorf("materializer: unexpected TableRef implementation %T", ref)
	}
	return t, nil
}

// RunConfig configures one materializer invocation. A single RunConfig value
// (same RunID) may be passed to Run any number of times, including after a
// crash or a deliberate stop: each call resumes from durable ControlStore
// state rather than trusting anything held in memory.
type RunConfig struct {
	// RunID identifies the rebuild run (UUIDv7, see docs/domain/observations/identity.md).
	// Required: the caller decides once, up front, whether a call starts a
	// new run or resumes an existing one, rather than the driver guessing.
	RunID string
	// CandidateManifest is what this invocation would use as the input
	// manifest if no run with RunID exists yet. It is normally built by
	// calling ListWithTimestamps + BuildManifest immediately before Run. If a
	// manifest already exists in the control store for RunID (a resume), the
	// existing manifest is used instead and CandidateManifest is ignored —
	// this is what keeps the rebuild input immutable across resumes.
	CandidateManifest Manifest
	// TableName is the Canonical Iceberg table name (canonical.TableName).
	TableName string
	// CommitBatchSize bounds how many Current-State rows go into one staging
	// Parquet file / one Iceberg commit, so a run with tens of thousands of
	// rows does not depend on a single huge transaction succeeding atomically.
	CommitBatchSize int
	// ExtractConcurrency bounds how many manifest chunks are fetched and
	// locally projected in parallel. <=1 means sequential.
	ExtractConcurrency int
	// MaxUnitsThisInvocation caps how many chunks/commit-batches this single
	// call actually performs new work on (0 = unlimited). It lets one
	// invocation process a bounded slice of a run of any size — including
	// tens of thousands of objects — without assuming its own process
	// outlives the whole run; the caller loops, calling Run again, until the
	// returned RunStatus.Phase is PhaseCompleted. It is also the fault
	// injection hook used by resumability tests: setting a small value
	// simulates a crash after partial progress.
	MaxUnitsThisInvocation int
	Log                    *slog.Logger
}

// Run executes (or resumes) one rebuild run's worth of work, bounded by
// cfg.MaxUnitsThisInvocation. It never trusts in-memory state from a prior
// call: every decision about what is already done is re-derived from
// ControlStore (and, for the commit phase, from a freshly loaded Iceberg
// table) so that resuming in a brand new process produces the same Canonical
// result as an uninterrupted run.
func Run(ctx context.Context, archiveClient ArchiveReader, control ControlStore, catalog CatalogClient, cfg RunConfig) (*RunStatus, error) {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	if cfg.RunID == "" {
		return nil, fmt.Errorf("materializer: RunID is required")
	}
	if _, err := uuid.Parse(cfg.RunID); err != nil {
		return nil, fmt.Errorf("materializer: RunID must be a UUID: %w", err)
	}

	manifest, err := fixManifest(ctx, control, cfg)
	if err != nil {
		return nil, err
	}

	budget := newBudget(cfg.MaxUnitsThisInvocation)

	if err := extractPhase(ctx, archiveClient, control, manifest, cfg, budget, log); err != nil {
		return nil, err
	}
	if budget.exhausted() {
		return writeRunStatus(ctx, control, cfg.RunID, PhaseExtracting, 0, 0, "", log)
	}

	rows, err := reducePhase(ctx, control, manifest, log)
	if err != nil {
		return nil, err
	}

	batches := commitBatches(rows, cfg.CommitBatchSize)
	if _, err := writeRunStatus(ctx, control, cfg.RunID, PhaseCommitting, len(rows), len(batches), "", log); err != nil {
		return nil, err
	}

	if err := commitPhase(ctx, control, catalog, manifest, cfg, batches, budget, log); err != nil {
		return nil, err
	}
	if budget.exhausted() {
		return writeRunStatus(ctx, control, cfg.RunID, PhaseCommitting, len(rows), len(batches), "", log)
	}

	return writeRunStatus(ctx, control, cfg.RunID, PhaseCompleted, len(rows), len(batches), "", log)
}

// fixManifest fixes the immutable input manifest for RunID: the first call
// wins (via a create-only control-store write), every later call for the
// same RunID reloads exactly what was fixed, ignoring its own freshly-listed
// candidate.
func fixManifest(ctx context.Context, control ControlStore, cfg RunConfig) (Manifest, error) {
	key := manifestKey(cfg.RunID)
	candidate := cfg.CandidateManifest
	candidate.RunID = cfg.RunID
	created, err := control.PutIfAbsentJSON(ctx, key, candidate)
	if err != nil {
		return Manifest{}, fmt.Errorf("fix manifest: %w", err)
	}
	if created {
		return candidate, nil
	}
	var existing Manifest
	found, err := control.GetJSON(ctx, key, &existing)
	if err != nil {
		return Manifest{}, fmt.Errorf("load fixed manifest: %w", err)
	}
	if !found {
		return Manifest{}, fmt.Errorf("materializer: manifest for run %s vanished from control store", cfg.RunID)
	}
	return existing, nil
}

// unitBudget bounds how many new (non-skipped) units of work one Run call performs.
type unitBudget struct {
	remaining int // <0 means unlimited
	mu        sync.Mutex
	stopped   bool
}

func newBudget(max int) *unitBudget {
	if max <= 0 {
		return &unitBudget{remaining: -1}
	}
	return &unitBudget{remaining: max}
}

// take reports whether the caller may perform one more unit of new work.
func (b *unitBudget) take() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.remaining == 0 {
		b.stopped = true
		return false
	}
	if b.remaining > 0 {
		b.remaining--
	}
	return true
}

func (b *unitBudget) exhausted() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stopped
}

// extractPhase projects each manifest chunk into candidate rows and
// checkpoints the result, skipping chunks whose checkpoint already exists.
func extractPhase(ctx context.Context, archiveClient ArchiveReader, control ControlStore, manifest Manifest, cfg RunConfig, budget *unitBudget, log *slog.Logger) error {
	conc := cfg.ExtractConcurrency
	if conc < 1 {
		conc = 1
	}
	sem := make(chan struct{}, conc)
	var wg sync.WaitGroup
	errs := make(chan error, len(manifest.Chunks))

	for i, chunkKeys := range manifest.Chunks {
		var existing ExtractStatus
		found, err := control.GetJSON(ctx, extractKey(cfg.RunID, i), &existing)
		if err != nil {
			return fmt.Errorf("extract chunk %d: check checkpoint: %w", i, err)
		}
		if found {
			continue // crash-safe reuse: already extracted, do not re-fetch or re-project
		}
		if !budget.take() {
			break
		}

		wg.Add(1)
		sem <- struct{}{}
		go func(i int, keys []string) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := extractChunk(ctx, archiveClient, control, cfg.RunID, i, keys); err != nil {
				errs <- fmt.Errorf("extract chunk %d: %w", i, err)
				return
			}
			log.Info("chunk extracted", slog.String("run_id", cfg.RunID), slog.Int("chunk_index", i))
		}(i, chunkKeys)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func extractChunk(ctx context.Context, archiveClient ArchiveReader, control ControlStore, runID string, index int, keys []string) error {
	ids := make([]observation.ObservationID, 0, len(keys))
	envs := make([]observation.Envelope, 0, len(keys))
	for _, key := range keys {
		env, err := archiveClient.Get(ctx, key)
		if err != nil {
			return fmt.Errorf("get %s: %w", key, err)
		}
		ids = append(ids, env.ObservationID)
		envs = append(envs, env)
	}
	rows, err := projection.Project(envs)
	if err != nil {
		return fmt.Errorf("project: %w", err)
	}
	chunkID := canonical.ChunkID(ids)
	digest := equivalence.Digest(equivalence.FromCanonical(rows))

	if err := control.PutJSON(ctx, candidatesKey(runID, index), candidateSet{Rows: rows}); err != nil {
		return fmt.Errorf("store candidates: %w", err)
	}
	status := ExtractStatus{ChunkIndex: index, ChunkID: chunkID, ObjectKeys: len(keys), RowCount: len(rows), Digest: digest}
	if err := control.PutJSON(ctx, extractKey(runID, index), status); err != nil {
		return fmt.Errorf("store checkpoint: %w", err)
	}
	return nil
}

// reducePhase merges every chunk's candidate rows into the Current State row
// set. It re-reads candidates from the control store rather than trusting
// anything held in memory, so it produces the same result whether it runs in
// the same process as extraction or in a resumed one.
func reducePhase(ctx context.Context, control ControlStore, manifest Manifest, log *slog.Logger) ([]projection.CanonicalMessage, error) {
	groups := make([][]projection.CanonicalMessage, 0, len(manifest.Chunks))
	for i := range manifest.Chunks {
		var cs candidateSet
		found, err := control.GetJSON(ctx, candidatesKey(manifest.RunID, i), &cs)
		if err != nil {
			return nil, fmt.Errorf("reduce: read candidates %d: %w", i, err)
		}
		if !found {
			return nil, fmt.Errorf("reduce: chunk %d has no candidates (extraction incomplete)", i)
		}
		groups = append(groups, cs.Rows)
	}
	rows := projection.Reduce(groups...)
	log.Info("reduce complete", slog.String("run_id", manifest.RunID), slog.Int("row_count", len(rows)))
	return rows, nil
}

// commitBatches splits an already-deterministically-sorted row set into
// contiguous, deterministically-indexed batches for staging/commit.
func commitBatches(rows []projection.CanonicalMessage, size int) [][]projection.CanonicalMessage {
	if size < 1 {
		size = 1
	}
	var batches [][]projection.CanonicalMessage
	for i := 0; i < len(rows); i += size {
		end := i + size
		if end > len(rows) {
			end = len(rows)
		}
		batches = append(batches, rows[i:end])
	}
	return batches
}

// commitPhase registers each commit batch's staging Parquet file with the
// Iceberg catalog. It reloads the table fresh via catalog.EnsureTable on
// every call (never caches a *table.Table across Run invocations), so a
// crash between a prior commit and its checkpoint write is recovered by
// re-deriving already-referenced files from the Catalog itself
// (icebergcat.CommitFiles), not by trusting local/checkpoint state.
func commitPhase(ctx context.Context, control ControlStore, catalog CatalogClient, manifest Manifest, cfg RunConfig, batches [][]projection.CanonicalMessage, budget *unitBudget, log *slog.Logger) error {
	if len(batches) == 0 {
		return nil
	}
	if err := catalog.EnsureNamespace(ctx); err != nil {
		return fmt.Errorf("commit: ensure namespace: %w", err)
	}
	tbl, err := catalog.EnsureTable(ctx, cfg.TableName)
	if err != nil {
		return fmt.Errorf("commit: ensure table: %w", err)
	}

	for j, batch := range batches {
		var existing CommitStatus
		found, err := control.GetJSON(ctx, commitKey(manifest.RunID, j), &existing)
		if err != nil {
			return fmt.Errorf("commit batch %d: check checkpoint: %w", j, err)
		}
		if found && existing.Committed {
			continue
		}
		if !budget.take() {
			return nil
		}

		chunkID := commitBatchID(batch)
		data, err := marshalParquet(batch)
		if err != nil {
			return fmt.Errorf("commit batch %d: build parquet: %w", j, err)
		}
		stagingPath := canonical.StagingPath(tbl.Location(), manifest.RunID, chunkID)
		if err := catalog.WriteStaging(ctx, tbl, stagingPath, data); err != nil {
			return fmt.Errorf("commit batch %d: write staging: %w", j, err)
		}
		result, err := catalog.CommitFiles(ctx, tbl, []string{stagingPath})
		if err != nil {
			return fmt.Errorf("commit batch %d: %w", j, err)
		}
		log.Info("commit batch registered",
			slog.String("run_id", manifest.RunID), slog.Int("batch_index", j),
			slog.String("status", string(result.Status)), slog.Int("row_count", len(batch)))

		status := CommitStatus{BatchIndex: j, ChunkID: chunkID, StagingKey: stagingPath, RowCount: len(batch), Committed: true}
		if err := control.PutJSON(ctx, commitKey(manifest.RunID, j), status); err != nil {
			return fmt.Errorf("commit batch %d: store checkpoint: %w", j, err)
		}
	}
	return nil
}

// commitBatchID derives a deterministic identity for one commit batch's exact
// row content (Message ID + adopted Observation ID pairs). It is distinct
// from canonical.ChunkID, which identifies an input Observation *set*: two
// different commit batches can legitimately be projected from overlapping or
// even identical contributing Observations (one HTTP page commonly supplies
// the winning snapshot for many different Message IDs), so batch identity
// must be a function of which rows are in the batch, not just which
// Observations fed them, or unrelated batches could collide on the same
// staging path.
func commitBatchID(rows []projection.CanonicalMessage) string {
	keys := make([]string, len(rows))
	for i, r := range rows {
		keys[i] = r.MessageID + "|" + r.ObservationID.String()
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func marshalParquet(rows []projection.CanonicalMessage) ([]byte, error) {
	var buf byteBuffer
	if err := canonical.WriteParquet(&buf, rows); err != nil {
		return nil, err
	}
	return buf.b, nil
}

// byteBuffer is a minimal io.Writer sink; avoids pulling in bytes.Buffer just
// for this one call site's semantics.
type byteBuffer struct{ b []byte }

func (w *byteBuffer) Write(p []byte) (int, error) {
	w.b = append(w.b, p...)
	return len(p), nil
}

// writeRunStatus persists the run-level checkpoint and returns it. It
// preserves StartedAt across calls (the run-status object is upserted, not
// replaced from scratch), so RunStatus.StartedAt reflects the first call for
// this RunID even when later calls only report incremental progress.
func writeRunStatus(ctx context.Context, control ControlStore, runID string, phase RunPhase, rowCount, commitBatchCount int, failure string, log *slog.Logger) (*RunStatus, error) {
	var existing RunStatus
	found, err := control.GetJSON(ctx, runStatusKey(runID), &existing)
	if err != nil {
		return nil, fmt.Errorf("write run status: load existing: %w", err)
	}
	started := time.Now().UTC()
	if found {
		started = existing.StartedAt
	}
	status := RunStatus{
		RunID: runID, Phase: phase, StartedAt: started, UpdatedAt: time.Now().UTC(),
		RowCount: rowCount, CommitBatchCount: commitBatchCount, Failure: failure,
	}
	if err := control.PutJSON(ctx, runStatusKey(runID), status); err != nil {
		return nil, fmt.Errorf("write run status: %w", err)
	}
	log.Info("run status updated", slog.String("run_id", runID), slog.String("phase", string(phase)))
	return &status, nil
}
