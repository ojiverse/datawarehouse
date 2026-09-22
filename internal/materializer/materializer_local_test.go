package materializer_test

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/apache/iceberg-go"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/archive"
	"github.com/ojiverse/datawarehouse/internal/equivalence"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/materializer"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

// These tests run the resumable materializer end to end against the same
// local stand-in used by the #36 spike (spike/local/docker-compose.yml: MinIO
// + apache/iceberg-rest-fixture), so they exercise the real Iceberg commit
// dedup/reload behavior that a fake catalog cannot. They are gated the same
// way internal/spike's integration test is gated
// (DWH_SPIKE_LOCAL_CATALOG=1 for spike, DWH_MATERIALIZER_LOCAL_CATALOG=1
// here) rather than always-on, since they need the docker stack running:
//
//	docker compose -f spike/local/docker-compose.yml up -d
//	DWH_MATERIALIZER_LOCAL_CATALOG=1 go test ./internal/materializer/... -run LocalCatalog -v

const (
	localArchiveEndpoint = "http://localhost:9000"
	localControlBucket   = "dwh-control"
	localCatalogURI      = "http://localhost:8181"
)

func requireLocalCatalog(t *testing.T) {
	t.Helper()
	if os.Getenv("DWH_MATERIALIZER_LOCAL_CATALOG") != "1" {
		t.Skip("set DWH_MATERIALIZER_LOCAL_CATALOG=1 with spike/local/docker-compose.yml running")
	}
}

func localArchiveConfig() archive.Config {
	return archive.Config{
		Endpoint: localArchiveEndpoint, Region: "us-east-1", Bucket: "dwh-observations",
		AccessKeyID: "dwhspike", SecretAccessKey: "dwhspike-secret", UsePathStyle: true,
	}
}

func localControlConfig() materializer.ControlConfig {
	return materializer.ControlConfig{
		Endpoint: localArchiveEndpoint, Region: "us-east-1", Bucket: localControlBucket,
		AccessKeyID: "dwhspike", SecretAccessKey: "dwhspike-secret", UsePathStyle: true,
	}
}

func localCatalogConfig(namespace string) icebergcat.Config {
	return icebergcat.Config{
		URI: localCatalogURI, Warehouse: "s3://dwh-canonical/", Namespace: namespace,
		Props: iceberg.Properties{
			"s3.endpoint": localArchiveEndpoint, "s3.region": "us-east-1",
			"s3.access-key-id": "dwhspike", "s3.secret-access-key": "dwhspike-secret",
			"s3.force-virtual-addressing": "false",
		},
	}
}

// seedArchive writes Envelopes via the real Archive create-only write path
// and returns the ObjectRefs (with real S3-recorded LastModified) needed to
// build a manifest. The shared "observations/v1/source=http_backfill/" prefix
// in the local MinIO bucket is also used by internal/spike's own integration
// test and may run concurrently in a different `go test ./...` package
// process, so the raw prefix listing is filtered down to exactly the keys
// this call just wrote: a test must never pick up another test's (or a
// concurrent run's) objects, and must never race with another test's cleanup
// deleting them mid-run.
func seedArchive(t *testing.T, ctx context.Context, arch *archive.Client, envs []observation.Envelope) []materializer.ObjectRef {
	t.Helper()
	want := make(map[string]struct{}, len(envs))
	for _, e := range envs {
		if _, err := arch.PutCreateOnly(ctx, e); err != nil {
			t.Fatalf("seed archive: %v", err)
		}
		want[e.ArchiveKey()] = struct{}{}
	}
	listed, err := materializer.ListWithTimestamps(ctx, localArchiveConfig(), "observations/v1/source=http_backfill/")
	if err != nil {
		t.Fatalf("list archive objects: %v", err)
	}
	objs := make([]materializer.ObjectRef, 0, len(envs))
	for _, o := range listed {
		if _, ok := want[o.Key]; ok {
			objs = append(objs, o)
		}
	}
	if len(objs) != len(envs) {
		t.Fatalf("seeded %d envelopes but only listed %d back", len(envs), len(objs))
	}
	return objs
}

func cleanupArchive(ctx context.Context, arch *archive.Client, envs []observation.Envelope) {
	for _, e := range envs {
		_ = arch.Delete(ctx, e.ArchiveKey())
	}
}

// deleteControlPrefix removes every control object under prefix. It uses its
// own S3 client (mirroring ListWithTimestamps) purely for test cleanup;
// production code never deletes control state itself.
func deleteControlPrefix(ctx context.Context, t *testing.T, prefix string) {
	t.Helper()
	cl := s3.New(s3.Options{
		BaseEndpoint: aws.String(localArchiveEndpoint), Region: "us-east-1",
		Credentials:  credentials.NewStaticCredentialsProvider("dwhspike", "dwhspike-secret", ""),
		UsePathStyle: true,
	})
	p := s3.NewListObjectsV2Paginator(cl, &s3.ListObjectsV2Input{Bucket: aws.String(localControlBucket), Prefix: aws.String(prefix)})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			t.Fatalf("list control state: %v", err)
		}
		for _, o := range page.Contents {
			_, _ = cl.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(localControlBucket), Key: o.Key})
		}
	}
}

// countingArchive wraps a real *archive.Client to record how many times each
// key is fetched, proving a resumed run does not re-fetch already-extracted
// chunks even against the real Archive store.
type countingArchive struct {
	inner *archive.Client
	mu    sync.Mutex
	calls map[string]int
}

func (c *countingArchive) Get(ctx context.Context, key string) (observation.Envelope, error) {
	c.mu.Lock()
	c.calls[key]++
	c.mu.Unlock()
	return c.inner.Get(ctx, key)
}

func (c *countingArchive) callCount(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls[key]
}

func scanDigest(t *testing.T, ctx context.Context, cat *icebergcat.Client, tableName string) string {
	t.Helper()
	tbl, err := cat.LoadTable(ctx, tableName)
	if err != nil {
		t.Fatalf("load table: %v", err)
	}
	rows, err := icebergcat.ScanRows(ctx, tbl)
	if err != nil {
		t.Fatalf("scan rows: %v", err)
	}
	return equivalence.Digest(rows)
}

// TestLocalCatalog_FullRebuildFromArchiveOnly proves Canonical Messages can
// be generated from the Archive alone, using #39's projection rule, with
// every row traceable back to its adopted Observation ID.
func TestLocalCatalog_FullRebuildFromArchiveOnly(t *testing.T) {
	requireLocalCatalog(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	arch, err := archive.New(localArchiveConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupArchive(context.Background(), arch, envs) })
	objs := seedArchive(t, ctx, arch, envs)

	namespace := "dwh_materializer_test_" + uuid.New().String()[:8]
	catCli, err := icebergcat.New(ctx, localCatalogConfig(namespace), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = catCli.DropTable(context.Background(), "message") })

	control, err := materializer.NewControlStore(localControlConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	runID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteControlPrefix(context.Background(), t, "runs/"+runID+"/") })

	manifest := materializer.BuildManifest(runID, "observations/v1/source=http_backfill/", objs, time.Now().UTC(), 20)

	status, err := materializer.Run(ctx, arch, control, materializer.NewCatalogClient(catCli), materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 30, ExtractConcurrency: 4, Log: log,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if status.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed, got %s", status.Phase)
	}

	want, err := projection.Project(envs)
	if err != nil {
		t.Fatal(err)
	}
	if status.RowCount != len(want) {
		t.Fatalf("row count = %d, want %d", status.RowCount, len(want))
	}

	tbl, err := catCli.LoadTable(ctx, "message")
	if err != nil {
		t.Fatal(err)
	}
	scanned, err := icebergcat.ScanRows(ctx, tbl)
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned) != len(want) {
		t.Fatalf("scanned %d rows, want %d", len(scanned), len(want))
	}
	for _, row := range scanned {
		if row.ObservationID == "" {
			t.Fatalf("row %s has no traceable Observation ID", row.MessageID)
		}
	}
	gotDigest := equivalence.Digest(scanned)
	wantDigest := equivalence.Digest(equivalence.FromCanonical(want))
	if gotDigest != wantDigest {
		t.Fatalf("scanned Canonical rows do not match projection: %v", equivalence.Diff(equivalence.FromCanonical(want), scanned))
	}
}

// TestLocalCatalog_StopAndResumeMatchesUninterrupted is the Definition-of-Done
// proof against a real Iceberg REST Catalog: deliberately stopping a run
// mid-chunk and resuming it in what is effectively a new invocation produces
// the same Canonical result as an uninterrupted run, and the resumed run does
// not re-fetch Archive objects whose chunk was already checkpointed.
func TestLocalCatalog_StopAndResumeMatchesUninterrupted(t *testing.T) {
	requireLocalCatalog(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	arch, err := archive.New(localArchiveConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	spec := observation.DefaultFixtureSpec()
	spec.Pages = 6
	envs, err := observation.GenerateFixture(spec)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupArchive(context.Background(), arch, envs) })
	objs := seedArchive(t, ctx, arch, envs)

	control, err := materializer.NewControlStore(localControlConfig(), log)
	if err != nil {
		t.Fatal(err)
	}

	// Uninterrupted baseline, in its own namespace/table.
	baselineNS := "dwh_materializer_test_" + uuid.New().String()[:8]
	baselineCat, err := icebergcat.New(ctx, localCatalogConfig(baselineNS), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = baselineCat.DropTable(context.Background(), "message") })
	baselineRunID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteControlPrefix(context.Background(), t, "runs/"+baselineRunID+"/") })
	baselineManifest := materializer.BuildManifest(baselineRunID, "observations/v1/source=http_backfill/", objs, time.Now().UTC(), 2)
	baselineStatus, err := materializer.Run(ctx, arch, control, materializer.NewCatalogClient(baselineCat), materializer.RunConfig{
		RunID: baselineRunID, CandidateManifest: baselineManifest, TableName: "message", CommitBatchSize: 25, Log: log,
	})
	if err != nil {
		t.Fatalf("baseline run: %v", err)
	}
	if baselineStatus.Phase != materializer.PhaseCompleted {
		t.Fatalf("baseline did not complete: %s", baselineStatus.Phase)
	}
	baselineDigest := scanDigest(t, ctx, baselineCat, "message")

	// Interrupted-then-resumed run, in its own namespace/table.
	resumeNS := "dwh_materializer_test_" + uuid.New().String()[:8]
	resumeCat, err := icebergcat.New(ctx, localCatalogConfig(resumeNS), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = resumeCat.DropTable(context.Background(), "message") })
	resumeRunID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteControlPrefix(context.Background(), t, "runs/"+resumeRunID+"/") })
	resumeManifest := materializer.BuildManifest(resumeRunID, "observations/v1/source=http_backfill/", objs, time.Now().UTC(), 2)
	if len(resumeManifest.Chunks) < 2 {
		t.Fatalf("need at least 2 chunks to prove partial progress, got %d", len(resumeManifest.Chunks))
	}

	counting := &countingArchive{inner: arch, calls: map[string]int{}}

	// Deliberately stop after one chunk of new work: simulates a crash.
	partial, err := materializer.Run(ctx, counting, control, materializer.NewCatalogClient(resumeCat), materializer.RunConfig{
		RunID: resumeRunID, CandidateManifest: resumeManifest, TableName: "message", CommitBatchSize: 25,
		MaxUnitsThisInvocation: 1, Log: log,
	})
	if err != nil {
		t.Fatalf("partial run: %v", err)
	}
	if partial.Phase == materializer.PhaseCompleted {
		t.Fatal("expected the bounded first call to stop before completion")
	}
	callsBeforeResume := map[string]int{}
	for _, chunk := range resumeManifest.Chunks {
		for _, k := range chunk {
			callsBeforeResume[k] = counting.callCount(k)
		}
	}

	// Resume: a call with no bound, as if a fresh process picked the run back up.
	final, err := materializer.Run(ctx, counting, control, materializer.NewCatalogClient(resumeCat), materializer.RunConfig{
		RunID: resumeRunID, CandidateManifest: resumeManifest, TableName: "message", CommitBatchSize: 25, Log: log,
	})
	if err != nil {
		t.Fatalf("resume run: %v", err)
	}
	if final.Phase != materializer.PhaseCompleted {
		t.Fatalf("resumed run did not complete: %s", final.Phase)
	}
	for key, before := range callsBeforeResume {
		if before == 0 {
			continue
		}
		if after := counting.callCount(key); after != before {
			t.Fatalf("key %s re-fetched after resume (before=%d after=%d)", key, before, after)
		}
	}

	resumeDigest := scanDigest(t, ctx, resumeCat, "message")
	if resumeDigest != baselineDigest {
		t.Fatal("resumed run's Canonical result differs from the uninterrupted baseline")
	}
}

// TestLocalCatalog_DeleteCanonicalAndControlStateAllowsFullRebuild proves that
// dropping the Canonical table and its control state and starting a fresh run
// reproduces the same Canonical result purely from the Archive.
func TestLocalCatalog_DeleteCanonicalAndControlStateAllowsFullRebuild(t *testing.T) {
	requireLocalCatalog(t)
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))

	arch, err := archive.New(localArchiveConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupArchive(context.Background(), arch, envs) })
	objs := seedArchive(t, ctx, arch, envs)

	namespace := "dwh_materializer_test_" + uuid.New().String()[:8]
	control, err := materializer.NewControlStore(localControlConfig(), log)
	if err != nil {
		t.Fatal(err)
	}

	runOnce := func() (string, string) {
		cat, err := icebergcat.New(ctx, localCatalogConfig(namespace), log)
		if err != nil {
			t.Fatal(err)
		}
		runID := uuid.Must(uuid.NewV7()).String()
		manifest := materializer.BuildManifest(runID, "observations/v1/source=http_backfill/", objs, time.Now().UTC(), 25)
		status, err := materializer.Run(ctx, arch, control, materializer.NewCatalogClient(cat), materializer.RunConfig{
			RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 30, Log: log,
		})
		if err != nil {
			t.Fatal(err)
		}
		if status.Phase != materializer.PhaseCompleted {
			t.Fatalf("run did not complete: %s", status.Phase)
		}
		digest := scanDigest(t, ctx, cat, "message")
		// Delete the Canonical table and this run's control state, as if a
		// full teardown happened before the next rebuild.
		if err := cat.DropTable(ctx, "message"); err != nil {
			t.Fatal(err)
		}
		deleteControlPrefix(ctx, t, "runs/"+runID+"/")
		return digest, runID
	}

	firstDigest, _ := runOnce()
	secondDigest, _ := runOnce()

	if firstDigest != secondDigest {
		t.Fatal("full rebuild after deleting Canonical + control state produced a different result")
	}
}

// TestLocalCatalog_NoDiscordCredentialsRequired proves the rebuild runs in a
// runtime with no Discord credential at all: it asserts common Discord
// credential env vars are unset for the whole call, mirroring the check
// internal/spike's Run already performs before its own rebuild pass.
func TestLocalCatalog_NoDiscordCredentialsRequired(t *testing.T) {
	requireLocalCatalog(t)
	for _, key := range []string{"DISCORD_BOT_TOKEN", "DISCORD_TOKEN", "DISCORD_CLIENT_SECRET"} {
		if v, ok := os.LookupEnv(key); ok && v != "" {
			t.Fatalf("test runtime must not have Discord credentials set (%s is set)", key)
		}
	}

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	arch, err := archive.New(localArchiveConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cleanupArchive(context.Background(), arch, envs) })
	objs := seedArchive(t, ctx, arch, envs)

	namespace := "dwh_materializer_test_" + uuid.New().String()[:8]
	cat, err := icebergcat.New(ctx, localCatalogConfig(namespace), log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cat.DropTable(context.Background(), "message") })
	control, err := materializer.NewControlStore(localControlConfig(), log)
	if err != nil {
		t.Fatal(err)
	}
	runID := uuid.Must(uuid.NewV7()).String()
	t.Cleanup(func() { deleteControlPrefix(context.Background(), t, "runs/"+runID+"/") })
	manifest := materializer.BuildManifest(runID, "observations/v1/source=http_backfill/", objs, time.Now().UTC(), 25)

	status, err := materializer.Run(ctx, arch, control, materializer.NewCatalogClient(cat), materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 30, Log: log,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed, got %s", status.Phase)
	}
}
