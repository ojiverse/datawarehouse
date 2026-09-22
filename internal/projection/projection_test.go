package projection_test

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

func env(t *testing.T, observed time.Time, payload string) observation.Envelope {
	t.Helper()
	id, err := observation.NewObservationID()
	if err != nil {
		t.Fatal(err)
	}
	return observation.Envelope{
		EnvelopeVersion: observation.EnvelopeVersion, ObservationID: id,
		SourceKind: observation.SourceHTTPBackfill, ObservedAt: observation.TS(observed),
		Payload:    json.RawMessage(payload),
		Provenance: &observation.HTTPProvenance{GuildID: "1", ChannelID: "2"},
	}
}

const msgTmpl = `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"%s","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":%s}]`

func TestEditedTimestampWinsOverObservedAt(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	older := env(t, t0.Add(time.Hour), `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"edited","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":"2026-01-01T00:30:00Z"}]`)
	newer := env(t, t0.Add(2*time.Hour), `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"unedited-later","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":null}]`)
	got, err := projection.Project([]observation.Envelope{newer, older})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "edited" || got[0].ObservationID != older.ObservationID {
		t.Fatalf("expected edited snapshot to win, got %+v", got)
	}
}

func TestObservedAtBreaksTieAndDuplicateObservationIgnored(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := env(t, t0, `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"a","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":null}]`)
	b := env(t, t0.Add(time.Minute), `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"b","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":null}]`)
	got, err := projection.Project([]observation.Envelope{a, b, a, b})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Content != "b" {
		t.Fatalf("expected later observed_at to win, got %+v", got)
	}
}

func TestUUIDOrderBreaksFullTie(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a := env(t, t0, `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"a","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":null}]`)
	b := env(t, t0, `[{"id":"175928847299117063","channel_id":"2","author":{"id":"9"},"content":"b","timestamp":"2016-04-30T11:18:25.796Z","edited_timestamp":null}]`)
	want := "a"
	if a.ObservationID.Less(b.ObservationID) {
		want = "b"
	}
	for _, order := range [][]observation.Envelope{{a, b}, {b, a}} {
		got, err := projection.Project(order)
		if err != nil {
			t.Fatal(err)
		}
		if got[0].Content != want {
			t.Fatalf("tie-break not deterministic: got %s want %s", got[0].Content, want)
		}
	}
}

func TestProjectionIsOrderIndependentOnFixture(t *testing.T) {
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	base, err := projection.Project(envs)
	if err != nil {
		t.Fatal(err)
	}
	if len(base) == 0 {
		t.Fatal("empty projection")
	}
	r := rand.New(rand.NewSource(42))
	for i := 0; i < 20; i++ {
		shuffled := append([]observation.Envelope(nil), envs...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
		got, err := projection.Project(shuffled)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, base) {
			t.Fatalf("projection differs after shuffle %d", i)
		}
	}
}
