// Package projection implements the HTTP Message deterministic projection
// defined in docs/domain/processing/projection.md.
package projection

import (
	"fmt"
	"sort"
	"time"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// Version identifies the projection rule set; it is stamped into every
// Canonical row so a rebuild with a different rule is distinguishable.
const Version = "http-message-v1"

// CanonicalMessage is the best-known Message state chosen from one Observation.
// Provenance is exactly one adopted Observation ID (no field-by-field merge).
type CanonicalMessage struct {
	MessageID      string
	ChannelID      string
	GuildID        string
	AuthorID       string
	AuthorUsername string
	AuthorIsBot    bool
	Content        string
	CreatedAt      time.Time
	EditedAt       *time.Time
	Pinned         bool
	MessageType    int
	ObservationID  observation.ObservationID
	ObservedAt     time.Time
	SourceKind     observation.SourceKind
	ProjectionVer  string
	SnowflakeTime  time.Time
}

// Project selects one best-known snapshot per Message ID from the given
// Observations. The result does not depend on input order, on duplicate
// Observations (same Observation ID), or on how the caller chunked the input.
func Project(envs []observation.Envelope) ([]CanonicalMessage, error) {
	seen := make(map[observation.ObservationID]struct{}, len(envs))
	best := make(map[string]CanonicalMessage)
	for _, env := range envs {
		if _, dup := seen[env.ObservationID]; dup {
			continue // same evidence re-processed: never duplicate it
		}
		seen[env.ObservationID] = struct{}{}
		if env.SourceKind != observation.SourceHTTPBackfill && env.SourceKind != observation.SourceHTTPReconciliation {
			return nil, fmt.Errorf("projection %s does not accept source kind %s", Version, env.SourceKind)
		}
		msgs, err := observation.DecodeHTTPPage(env.Payload)
		if err != nil {
			return nil, fmt.Errorf("observation %s: %w", env.ObservationID, err)
		}
		for _, m := range msgs {
			cand, err := toCanonical(env, m)
			if err != nil {
				return nil, fmt.Errorf("observation %s: %w", env.ObservationID, err)
			}
			cur, ok := best[m.ID]
			if !ok || newer(cand, cur) {
				best[m.ID] = cand
			}
		}
	}
	out := make([]CanonicalMessage, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MessageID < out[j].MessageID })
	return out, nil
}

// Reduce merges already-projected row groups (for example the local winners
// produced by calling Project on disjoint chunks of the same Observation set)
// into the same result Project would produce over the union of their inputs.
//
// This holds because newer defines a strict total order over candidates for a
// given Message ID (no two distinct Observations tie: the final tie-break is
// Observation ID inequality), so picking the max within each group and then
// the max across group maxima is equivalent to picking the max over the
// union. Reduce lets a resumable materializer project chunks independently
// and combine the results without re-deriving Current State from scratch.
func Reduce(groups ...[]CanonicalMessage) []CanonicalMessage {
	best := make(map[string]CanonicalMessage)
	for _, g := range groups {
		for _, cand := range g {
			cur, ok := best[cand.MessageID]
			if !ok || newer(cand, cur) {
				best[cand.MessageID] = cand
			}
		}
	}
	out := make([]CanonicalMessage, 0, len(best))
	for _, c := range best {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].MessageID < out[j].MessageID })
	return out
}

// newer applies the precedence: edited_timestamp, then observed_at, then
// Observation UUID value. Returns true when a should replace b.
func newer(a, b CanonicalMessage) bool {
	switch {
	case editedAfter(a.EditedAt, b.EditedAt):
		return true
	case editedAfter(b.EditedAt, a.EditedAt):
		return false
	case a.ObservedAt.After(b.ObservedAt):
		return true
	case b.ObservedAt.After(a.ObservedAt):
		return false
	default:
		return b.ObservationID.Less(a.ObservationID)
	}
}

// editedAfter treats "unedited" (nil) as older than any edit.
func editedAfter(a, b *time.Time) bool {
	if a == nil {
		return false
	}
	if b == nil {
		return true
	}
	return a.After(*b)
}

func toCanonical(env observation.Envelope, m observation.MessageSnapshot) (CanonicalMessage, error) {
	st, err := observation.SnowflakeTime(m.ID)
	if err != nil {
		return CanonicalMessage{}, err
	}
	return CanonicalMessage{
		MessageID: m.ID, ChannelID: m.ChannelID, GuildID: env.Provenance.GuildID,
		AuthorID: m.Author.ID, AuthorUsername: m.Author.Username, AuthorIsBot: m.Author.Bot,
		Content: m.Content, CreatedAt: m.Timestamp.UTC(), EditedAt: utcPtr(m.EditedTimestamp),
		Pinned: m.Pinned, MessageType: m.Type,
		ObservationID: env.ObservationID, ObservedAt: env.ObservedAt.UTC(), SourceKind: env.SourceKind,
		ProjectionVer: Version, SnowflakeTime: st,
	}, nil
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
