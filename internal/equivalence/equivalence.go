// Package equivalence defines "semantically identical" for rebuild acceptance:
// a canonical serialization of domain rows sorted by primary key, excluding
// Iceberg snapshot IDs, file placement, row order and materialization times.
package equivalence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ojiverse/datawarehouse/internal/projection"
)

// Row is the domain projection of one Canonical record used for comparison.
// It is deliberately a flat, string-typed shape so that rows read back from
// R2 SQL (JSON) and rows produced locally (Go structs) serialize identically.
type Row struct {
	MessageID     string
	ChannelID     string
	GuildID       string
	AuthorID      string
	Content       string
	CreatedAt     string
	EditedAt      string
	Pinned        bool
	ObservationID string
	ObservedAt    string
	ProjectionVer string
}

// FromCanonical converts projected messages into comparison rows.
func FromCanonical(rows []projection.CanonicalMessage) []Row {
	out := make([]Row, 0, len(rows))
	for _, r := range rows {
		edited := ""
		if r.EditedAt != nil {
			edited = r.EditedAt.UTC().Format(time.RFC3339Nano)
		}
		out = append(out, Row{
			MessageID: r.MessageID, ChannelID: r.ChannelID, GuildID: r.GuildID, AuthorID: r.AuthorID,
			Content: r.Content, CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339Nano), EditedAt: edited,
			Pinned: r.Pinned, ObservationID: r.ObservationID.String(),
			ObservedAt: r.ObservedAt.UTC().Format(time.RFC3339Nano), ProjectionVer: r.ProjectionVer,
		})
	}
	return out
}

// Serialize returns the canonical text form: one line per row, sorted by
// message_id, tab-separated fields in a fixed order.
func Serialize(rows []Row) string {
	sorted := append([]Row(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].MessageID < sorted[j].MessageID })
	var sb strings.Builder
	for _, r := range sorted {
		fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\t%q\t%s\t%s\t%t\t%s\t%s\t%s\n",
			r.MessageID, r.ChannelID, r.GuildID, r.AuthorID, r.Content, r.CreatedAt, r.EditedAt,
			r.Pinned, r.ObservationID, r.ObservedAt, r.ProjectionVer)
	}
	return sb.String()
}

// Digest is the SHA-256 of Serialize, convenient for reports.
func Digest(rows []Row) string {
	sum := sha256.Sum256([]byte(Serialize(rows)))
	return hex.EncodeToString(sum[:])
}

// Diff returns the lines present in only one side. Empty means equivalent.
func Diff(a, b []Row) []string {
	la := strings.Split(strings.TrimSuffix(Serialize(a), "\n"), "\n")
	lb := strings.Split(strings.TrimSuffix(Serialize(b), "\n"), "\n")
	setA, setB := map[string]struct{}{}, map[string]struct{}{}
	for _, l := range la {
		setA[l] = struct{}{}
	}
	for _, l := range lb {
		setB[l] = struct{}{}
	}
	var out []string
	for _, l := range la {
		if _, ok := setB[l]; !ok {
			out = append(out, "- "+l)
		}
	}
	for _, l := range lb {
		if _, ok := setA[l]; !ok {
			out = append(out, "+ "+l)
		}
	}
	return out
}
