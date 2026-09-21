package observation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// FixtureSpec controls the deterministic Envelope v1 fixture set.
//
// The fixture deliberately produces overlapping pages: the same Message ID
// appears in several Observations with different edited_timestamp / observed_at
// combinations so that every branch of the projection rule is exercised.
type FixtureSpec struct {
	GuildID   string
	ChannelID string
	// Pages is the number of HTTP response pages (Observations).
	Pages int
	// MessagesPerPage is the page size; consecutive pages overlap by Overlap messages.
	MessagesPerPage int
	Overlap         int
	// Base is the observed_at of the first page; later pages are one minute apart.
	Base time.Time
}

// DefaultFixtureSpec is small enough for the R2 SQL default 500-row cap and
// still exercises overlap and edit precedence.
func DefaultFixtureSpec() FixtureSpec {
	return FixtureSpec{
		GuildID: "100000000000000001", ChannelID: "200000000000000002",
		Pages: 4, MessagesPerPage: 25, Overlap: 5,
		Base: time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC),
	}
}

// GenerateFixture builds the Envelope set with the same physical JSON shape
// as contracts/observation-envelope/v1/http-backfill-page.json. Observation IDs
// are UUIDv7 and thus differ per run; the domain content is deterministic.
func GenerateFixture(spec FixtureSpec) ([]Envelope, error) {
	if spec.Pages <= 0 || spec.MessagesPerPage <= 0 || spec.Overlap >= spec.MessagesPerPage {
		return nil, fmt.Errorf("invalid fixture spec %+v", spec)
	}
	envs := make([]Envelope, 0, spec.Pages)
	stride := spec.MessagesPerPage - spec.Overlap
	for p := 0; p < spec.Pages; p++ {
		observed := spec.Base.Add(time.Duration(p) * time.Minute)
		first := p * stride
		msgs := make([]json.RawMessage, 0, spec.MessagesPerPage)
		for i := first; i < first+spec.MessagesPerPage; i++ {
			raw, err := json.Marshal(fixtureMessage(spec, i, p, observed))
			if err != nil {
				return nil, err
			}
			msgs = append(msgs, raw)
		}
		payload, err := json.Marshal(msgs)
		if err != nil {
			return nil, err
		}
		id, err := NewObservationID()
		if err != nil {
			return nil, err
		}
		before := fixtureSnowflake(spec.Base, first+spec.MessagesPerPage)
		envs = append(envs, Envelope{
			EnvelopeVersion: EnvelopeVersion,
			ObservationID:   id,
			SourceKind:      SourceHTTPBackfill,
			ObservedAt:      TS(observed),
			Payload:         payload,
			Provenance: &HTTPProvenance{
				RunID: "0199a1b2-0000-7000-8000-00000000f1de", DiscordAPIVersion: "10",
				GuildID: spec.GuildID, ChannelID: spec.ChannelID,
				Operation:        "get_channel_messages",
				Endpoint:         "/channels/" + spec.ChannelID + "/messages",
				Pagination:       Pagination{Before: &before, After: nil},
				Limit:            spec.MessagesPerPage,
				RequestStartedAt: TS(observed.Add(-time.Second)), ResponseCompletedAt: TS(observed),
				HTTPStatus:   200,
				Capabilities: map[string]bool{"message_content": true},
				RateLimit:    &RateLimit{Limit: 5, Remaining: 5 - p%5, ResetAfterSeconds: 1.5, Bucket: "route-bucket-hash"},
			},
		})
	}
	return envs, nil
}

// LoadContractFixtures decodes every *.json Envelope document in dir (the
// shared cross-language fixtures) in file-name order.
func LoadContractFixtures(dir string) ([]Envelope, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	envs := make([]Envelope, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		e, err := DecodeJSON(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		envs = append(envs, e)
	}
	return envs, nil
}

// fixtureMessage returns a Discord-shaped message. Messages whose index is a
// multiple of 7 are "edited" and the edit time grows with the page number, so
// later pages win by edited_timestamp; every other overlapping message wins by
// observed_at only.
func fixtureMessage(spec FixtureSpec, index, page int, observed time.Time) map[string]any {
	created := spec.Base.Add(-time.Duration(1000-index) * time.Second)
	m := map[string]any{
		"id":         fixtureSnowflake(created, index),
		"channel_id": spec.ChannelID,
		"author": map[string]any{
			"id": fmt.Sprintf("3000000000000000%02d", index%10), "username": fmt.Sprintf("user%02d", index%10), "bot": false,
		},
		"content":          fmt.Sprintf("message %d seen on page %d", index, page),
		"timestamp":        created.Format(time.RFC3339Nano),
		"edited_timestamp": nil,
		"pinned":           index%11 == 0,
		"type":             0,
		"attachments":      []any{},
		"embeds":           []any{},
		"mention_everyone": false,
	}
	if index%7 == 0 {
		m["edited_timestamp"] = observed.Add(-30 * time.Second).Format(time.RFC3339Nano)
	}
	return m
}

func fixtureSnowflake(t time.Time, seq int) string {
	ms := uint64(t.UnixMilli() - DiscordEpochMillis)
	return fmt.Sprintf("%d", ms<<22|uint64(seq&0xFFF))
}
