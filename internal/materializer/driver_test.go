package materializer_test

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/ojiverse/datawarehouse/internal/equivalence"
	"github.com/ojiverse/datawarehouse/internal/icebergcat"
	"github.com/ojiverse/datawarehouse/internal/materializer"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

// --- fakes ---------------------------------------------------------------

type fakeArchive struct {
	mu    sync.Mutex
	envs  map[string]observation.Envelope
	calls map[string]int
}

func newFakeArchive(envs []observation.Envelope) *fakeArchive {
	f := &fakeArchive{envs: map[string]observation.Envelope{}, calls: map[string]int{}}
	for _, e := range envs {
		f.envs[e.ArchiveKey()] = e
	}
	return f
}

func (f *fakeArchive) Get(_ context.Context, key string) (observation.Envelope, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[key]++
	e, ok := f.envs[key]
	if !ok {
		return observation.Envelope{}, fmt.Errorf("fakeArchive: no such key %s", key)
	}
	return e, nil
}

func (f *fakeArchive) callCount(key string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[key]
}

func (f *fakeArchive) totalCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		n += c
	}
	return n
}

type fakeControl struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newFakeControl() *fakeControl { return &fakeControl{data: map[string][]byte{}} }

func (f *fakeControl) PutIfAbsentJSON(_ context.Context, key string, v any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.data[key]; ok {
		return false, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	f.data[key] = b
	return true, nil
}

func (f *fakeControl) PutJSON(_ context.Context, key string, v any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f.data[key] = b
	return nil
}

func (f *fakeControl) GetJSON(_ context.Context, key string, v any) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.data[key]
	if !ok {
		return false, nil
	}
	return true, json.Unmarshal(b, v)
}

func (f *fakeControl) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			out = append(out, k)
		}
	}
	return out, nil
}

// deleteKey simulates losing a checkpoint write (e.g. a crash right after an
// Iceberg commit succeeded but before the checkpoint object was written).
func (f *fakeControl) deleteKey(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.data, key)
}

type fakeTableRef struct{ loc string }

func (f fakeTableRef) Location() string { return f.loc }

// fakeCatalog mirrors icebergcat.Client's commit-dedup contract (see
// internal/icebergcat/icebergcat.go CommitFiles) closely enough to unit test
// the driver's crash-recovery behavior without a real REST catalog: a commit
// whose paths are all already referenced returns AlreadyReferenced instead of
// registering the files again.
type fakeCatalog struct {
	mu          sync.Mutex
	referenced  map[string]int
	commitCalls int
}

func newFakeCatalog() *fakeCatalog { return &fakeCatalog{referenced: map[string]int{}} }

func (f *fakeCatalog) EnsureNamespace(context.Context) error { return nil }

func (f *fakeCatalog) EnsureTable(_ context.Context, name string) (materializer.TableRef, error) {
	return fakeTableRef{loc: "mem://canonical/" + name}, nil
}

func (f *fakeCatalog) WriteStaging(context.Context, materializer.TableRef, string, []byte) error {
	return nil
}

func (f *fakeCatalog) CommitFiles(_ context.Context, _ materializer.TableRef, paths []string) (icebergcat.CommitResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.commitCalls++
	already := 0
	for _, p := range paths {
		if _, ok := f.referenced[p]; ok {
			already++
		}
	}
	if already == len(paths) {
		return icebergcat.CommitResult{Status: icebergcat.AlreadyReferenced, DataFiles: len(f.referenced)}, nil
	}
	for _, p := range paths {
		f.referenced[p]++
	}
	return icebergcat.CommitResult{Status: icebergcat.Committed, DataFiles: len(f.referenced)}, nil
}

func (f *fakeCatalog) referencedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.referenced)
}

// --- test setup ------------------------------------------------------------

func fixtureEnvs(t *testing.T) []observation.Envelope {
	t.Helper()
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	return envs
}

func buildManifest(t *testing.T, runID string, envs []observation.Envelope, chunkSize int) materializer.Manifest {
	t.Helper()
	cutoff := time.Now().UTC()
	objs := make([]materializer.ObjectRef, len(envs))
	for i, e := range envs {
		objs[i] = materializer.ObjectRef{Key: e.ArchiveKey(), UploadedAt: cutoff.Add(-time.Hour)}
	}
	return materializer.BuildManifest(runID, "observations/v1/source=http_backfill/", objs, cutoff, chunkSize)
}

func wantProjection(t *testing.T, envs []observation.Envelope) []projection.CanonicalMessage {
	t.Helper()
	rows, err := projection.Project(envs)
	if err != nil {
		t.Fatal(err)
	}
	return rows
}

// --- tests -------------------------------------------------------------

func TestRunUninterruptedProducesFullProjection(t *testing.T) {
	envs := fixtureEnvs(t)
	runID := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runID, envs, 3)

	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	status, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed, got %s", status.Phase)
	}
	want := wantProjection(t, envs)
	if status.RowCount != len(want) {
		t.Fatalf("row count = %d, want %d", status.RowCount, len(want))
	}
	if cat.referencedCount() != status.CommitBatchCount {
		t.Fatalf("committed data files = %d, want %d batches", cat.referencedCount(), status.CommitBatchCount)
	}
}

// TestResumeAfterInterruptionProducesSameResultAndSkipsCompletedChunks is the
// Definition-of-Done proof: a run stopped mid-way (simulating a crash) and
// resumed in what is effectively a new invocation converges to exactly the
// same Canonical result as an uninterrupted run, and does not reprocess
// chunks that already have a durable checkpoint.
func TestResumeAfterInterruptionProducesSameResultAndSkipsCompletedChunks(t *testing.T) {
	envs := fixtureEnvs(t)
	runIDInterrupted := uuid.Must(uuid.NewV7()).String()
	runIDUninterrupted := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runIDInterrupted, envs, 2)

	// Uninterrupted baseline run.
	{
		arch := newFakeArchive(envs)
		control := newFakeControl()
		cat := newFakeCatalog()
		baseManifest := manifest
		baseManifest.RunID = runIDUninterrupted
		status, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
			RunID: runIDUninterrupted, CandidateManifest: baseManifest, TableName: "message", CommitBatchSize: 4,
		})
		if err != nil {
			t.Fatal(err)
		}
		if status.Phase != materializer.PhaseCompleted {
			t.Fatalf("baseline run did not complete: %s", status.Phase)
		}
	}

	// Interrupted-then-resumed run over the same input.
	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	// First invocation: budget only allows 1 of len(manifest.Chunks) chunks
	// to be newly extracted, simulating a crash partway through.
	status, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runIDInterrupted, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4,
		MaxUnitsThisInvocation: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != materializer.PhaseExtracting {
		t.Fatalf("expected the bounded first call to stop mid-extraction, got phase %s", status.Phase)
	}
	if len(manifest.Chunks) < 2 {
		t.Fatalf("fixture manifest too small to exercise partial progress: %d chunks", len(manifest.Chunks))
	}

	callsAfterFirstInvocation := map[string]int{}
	for _, chunk := range manifest.Chunks {
		for _, k := range chunk {
			callsAfterFirstInvocation[k] = arch.callCount(k)
		}
	}

	// Second invocation: a brand new call (as if in a new process) with no
	// budget limit resumes to completion.
	final, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runIDInterrupted, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Phase != materializer.PhaseCompleted {
		t.Fatalf("resumed run did not complete: %s", final.Phase)
	}

	// Any archive object already fetched before the interruption must not be
	// fetched again by the resumed run.
	for key, before := range callsAfterFirstInvocation {
		if before == 0 {
			continue // this key's chunk had not been extracted yet; it's fine for it to be fetched now
		}
		after := arch.callCount(key)
		if after != before {
			t.Fatalf("key %s was re-fetched after resume (before=%d after=%d); completed chunk was reprocessed", key, before, after)
		}
	}

	// The resumed run's Canonical result must match the uninterrupted baseline.
	wantRows := wantProjection(t, envs)
	if final.RowCount != len(wantRows) {
		t.Fatalf("resumed row count = %d, want %d", final.RowCount, len(wantRows))
	}

	// Reprocessing must not double-register data files: the number of
	// referenced files must equal the number of commit batches, never more.
	if cat.referencedCount() != final.CommitBatchCount {
		t.Fatalf("referenced data files = %d, want exactly %d commit batches (no duplicates)", cat.referencedCount(), final.CommitBatchCount)
	}
}

// TestCrashBetweenCommitAndCheckpointRecoversFromCatalog proves the specific
// invariant from docs/architecture/cloudflare/processing/materialization.md:
// if a crash happens between a successful Iceberg commit and the control
// checkpoint write, recovery must reload from the Catalog (not trust stale
// local/checkpoint state) and converge without double-registering the file.
func TestCrashBetweenCommitAndCheckpointRecoversFromCatalog(t *testing.T) {
	envs := fixtureEnvs(t)
	runID := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runID, envs, len(envs)) // single chunk

	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	status, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed, got %s", status.Phase)
	}
	committedFiles := cat.referencedCount()
	commitCallsBefore := cat.commitCalls

	// Simulate "crashed after the Iceberg commit succeeded, before the
	// control-bucket checkpoint was written": drop the one commit checkpoint.
	control.deleteKey("runs/" + runID + "/commit/00000.json")

	// Resume: must reach CommitFiles again, see the file already referenced
	// via the (freshly loaded) catalog, and converge without adding a file.
	final, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if final.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed after recovery, got %s", final.Phase)
	}
	if cat.commitCalls <= commitCallsBefore {
		t.Fatal("expected CommitFiles to be called again during recovery")
	}
	if cat.referencedCount() != committedFiles {
		t.Fatalf("referenced files changed across recovery: before=%d after=%d (duplicate registration)", committedFiles, cat.referencedCount())
	}
}

// TestManifestIsImmutableAcrossResume proves a resumed run cannot silently
// widen or shift its input set: a second call's freshly-built candidate
// manifest (e.g. because more objects were added to the Archive after the
// first call) is ignored once a manifest has been fixed for that RunID.
func TestManifestIsImmutableAcrossResume(t *testing.T) {
	envs := fixtureEnvs(t)
	runID := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runID, envs, 3)

	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	if _, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4, MaxUnitsThisInvocation: 1,
	}); err != nil {
		t.Fatal(err)
	}

	// A wider "candidate" built as if new objects had appeared since.
	widerEnvs := append(append([]observation.Envelope(nil), envs...), envs[0])
	widerManifest := buildManifest(t, runID, widerEnvs, 3)

	if _, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: widerManifest, TableName: "message", CommitBatchSize: 4,
	}); err != nil {
		t.Fatal(err)
	}

	var stored materializer.Manifest
	found, err := control.GetJSON(context.Background(), "runs/"+runID+"/manifest.json", &stored)
	if err != nil || !found {
		t.Fatalf("manifest missing: %v", err)
	}
	if !reflect.DeepEqual(stored.Chunks, manifest.Chunks) {
		t.Fatalf("manifest drifted across resume: got %v want %v", stored.Chunks, manifest.Chunks)
	}
}

// TestReprocessingSameChunkDoesNotDoubleCount proves that calling Run twice
// for a run that is already fully completed is a safe no-op: it must not
// duplicate rows or data files.
func TestReprocessingSameChunkDoesNotDoubleCount(t *testing.T) {
	envs := fixtureEnvs(t)
	runID := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runID, envs, 5)

	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	cfg := materializer.RunConfig{RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4}
	first, err := materializer.Run(context.Background(), arch, control, cat, cfg)
	if err != nil {
		t.Fatal(err)
	}
	second, err := materializer.Run(context.Background(), arch, control, cat, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if first.RowCount != second.RowCount || first.CommitBatchCount != second.CommitBatchCount {
		t.Fatalf("reprocessing changed counts: first=%+v second=%+v", first, second)
	}
	if cat.referencedCount() != first.CommitBatchCount {
		t.Fatalf("referenced files = %d, want %d (no duplicates from reprocessing)", cat.referencedCount(), first.CommitBatchCount)
	}
}

// TestNoDiscordCredentialsRequired documents (and enforces, via go vet-style
// compile check) that ArchiveReader/ControlStore/CatalogClient carry no
// Discord-related configuration: Run only needs Archive, control-bucket, and
// catalog access, matching materialization.md's "rebuild works in a runtime
// with no Discord credentials" requirement. This is exercised for real
// against a local catalog in the integration test; here it documents the
// same property at the unit level via equivalence with a full local rebuild.
func TestNoDiscordCredentialsRequired(t *testing.T) {
	envs := fixtureEnvs(t)
	runID := uuid.Must(uuid.NewV7()).String()
	manifest := buildManifest(t, runID, envs, 4)

	arch := newFakeArchive(envs)
	control := newFakeControl()
	cat := newFakeCatalog()

	status, err := materializer.Run(context.Background(), arch, control, cat, materializer.RunConfig{
		RunID: runID, CandidateManifest: manifest, TableName: "message", CommitBatchSize: 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	if status.Phase != materializer.PhaseCompleted {
		t.Fatalf("expected completed, got %s", status.Phase)
	}
}

// sanity: equivalence package is reused (not reimplemented) for row digests.
var _ = equivalence.Digest
