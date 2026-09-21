package canonical

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// ChunkID derives a deterministic identity for a set of input Observations:
// the SHA-256 of the sorted Observation IDs. The same input set therefore maps
// to the same staging file regardless of listing order or retry count, which
// is what makes "retry does not double-register the same data file" checkable.
func ChunkID(ids []observation.ObservationID) string {
	sorted := make([]string, len(ids))
	for i, id := range ids {
		sorted[i] = id.String()
	}
	sort.Strings(sorted)
	h := sha256.New()
	for _, s := range sorted {
		h.Write([]byte(s))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// StagingPath returns the data-file location for a rebuild run + chunk under
// the table location. It lives in the Canonical bucket so that the catalog's
// vended credentials cover it; the control bucket contract is unaffected.
func StagingPath(tableLocation, runID, chunkID string) string {
	return fmt.Sprintf("%s/data/staging/run=%s/chunk=%s.parquet", tableLocation, runID, chunkID)
}
