package materializer

import (
	"fmt"
	"time"

	"github.com/ojiverse/datawarehouse/internal/projection"
)

// RunPhase is the coarse-grained state of a rebuild run, persisted so a
// different process (with no in-memory state at all) can resume it.
type RunPhase string

// Run phases, in order. A resumed run inspects checkpoints to find the first
// incomplete unit of work within its current phase rather than restarting.
const (
	PhaseExtracting RunPhase = "extracting" // per-chunk projection into candidate rows
	PhaseReducing   RunPhase = "reducing"   // merging candidates into Current State
	PhaseCommitting RunPhase = "committing" // registering commit-batches with the Iceberg catalog
	PhaseCompleted  RunPhase = "completed"
)

// RunStatus is the run-level checkpoint.
type RunStatus struct {
	RunID     string    `json:"run_id"`
	Phase     RunPhase  `json:"phase"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// RowCount and CommitBatchCount are filled in once Reduce has run, so a
	// resumed run does not need to keep the reduced set in memory to know how
	// many commit batches to expect.
	RowCount         int    `json:"row_count,omitempty"`
	CommitBatchCount int    `json:"commit_batch_count,omitempty"`
	Failure          string `json:"failure,omitempty"`
}

// ExtractStatus is a per-chunk checkpoint for the extraction phase: chunk i's
// Archive objects have been decoded, locally projected, and the result stored
// as candidate rows in the control bucket. A resumed run that finds this
// checkpoint for chunk i skips re-fetching and re-projecting it entirely.
type ExtractStatus struct {
	ChunkIndex int    `json:"chunk_index"`
	ChunkID    string `json:"chunk_id"`
	ObjectKeys int    `json:"object_keys"`
	RowCount   int    `json:"row_count"`
	Digest     string `json:"digest"`
}

// CommitStatus is a per-commit-batch checkpoint for the commit phase.
type CommitStatus struct {
	BatchIndex int    `json:"batch_index"`
	ChunkID    string `json:"chunk_id"`
	StagingKey string `json:"staging_key"`
	RowCount   int    `json:"row_count"`
	Committed  bool   `json:"committed"`
}

// candidateSet is what an extraction checkpoint's companion object stores:
// the locally-projected rows for one manifest chunk.
type candidateSet struct {
	Rows []projection.CanonicalMessage `json:"rows"`
}

// Control object key layout under the run's namespace. Keeping these as pure
// functions (no I/O) makes the layout itself independently testable and
// keeps the driver from hand-building paths in multiple places.
func manifestKey(runID string) string  { return fmt.Sprintf("runs/%s/manifest.json", runID) }
func runStatusKey(runID string) string { return fmt.Sprintf("runs/%s/status.json", runID) }
func extractKey(runID string, i int) string {
	return fmt.Sprintf("runs/%s/extract/%05d.json", runID, i)
}
func candidatesKey(runID string, i int) string {
	return fmt.Sprintf("runs/%s/candidates/%05d.json", runID, i)
}
func commitKey(runID string, i int) string { return fmt.Sprintf("runs/%s/commit/%05d.json", runID, i) }
func commitPrefix(runID string) string     { return fmt.Sprintf("runs/%s/commit/", runID) }
