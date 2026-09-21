package observation_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

func sampleEnvelope(t *testing.T) observation.Envelope {
	t.Helper()
	id, err := observation.NewObservationID()
	if err != nil {
		t.Fatal(err)
	}
	return observation.Envelope{
		EnvelopeVersion: observation.EnvelopeVersion,
		ObservationID:   id,
		SourceKind:      observation.SourceHTTPBackfill,
		ObservedAt:      time.Date(2026, 9, 21, 3, 4, 5, 0, time.UTC),
		Payload:         json.RawMessage(`[{"id":"1000","channel_id":"2","author":{"id":"3"},"content":"hi","timestamp":"2026-09-20T00:00:00Z","edited_timestamp":null}]`),
		HTTP: &observation.HTTPProvenance{
			RunID: "run", DiscordAPIVersion: "10", GuildID: "1", ChannelID: "2",
			Operation: "get_channel_messages", Pagination: map[string]string{"before": "1001"}, Limit: 100,
			RequestStartedAt: time.Now().UTC(), ResponseCompletedAt: time.Now().UTC(), HTTPStatus: 200,
			Capabilities: map[string]bool{"message_content": true},
		},
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	e := sampleEnvelope(t)
	b, err := observation.EncodeGzipJSON(e)
	if err != nil {
		t.Fatal(err)
	}
	got, err := observation.DecodeGzipJSON(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if got.ObservationID != e.ObservationID || got.SourceKind != e.SourceKind || !got.ObservedAt.Equal(e.ObservedAt) {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, e)
	}
	if got.PayloadSHA256() != e.PayloadSHA256() {
		t.Fatal("payload hash changed across round trip")
	}
}

func TestArchiveKeyLayout(t *testing.T) {
	e := sampleEnvelope(t)
	key := e.ArchiveKey()
	want := "observations/v1/source=http_backfill/year=2026/month=09/day=21/hour=03/" + e.ObservationID.String() + ".json.gz"
	if key != want {
		t.Fatalf("key %q, want %q", key, want)
	}
}

func TestValidateRejectsNonV7(t *testing.T) {
	if _, err := observation.ParseObservationID("123e4567-e89b-12d3-a456-426614174000"); err == nil {
		t.Fatal("expected uuid v1 to be rejected")
	}
}

func TestValidateRequiresHTTPProvenance(t *testing.T) {
	e := sampleEnvelope(t)
	e.HTTP = nil
	if err := e.Validate(); err == nil || !strings.Contains(err.Error(), "provenance") {
		t.Fatalf("expected provenance error, got %v", err)
	}
}

func TestDecodeHTTPPageAndSnowflakeTime(t *testing.T) {
	e := sampleEnvelope(t)
	msgs, err := observation.DecodeHTTPPage(e.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 1 || msgs[0].ID != "1000" || msgs[0].Author.ID != "3" {
		t.Fatalf("unexpected messages %+v", msgs)
	}
	// 175928847299117063 is the documented example snowflake (2016-04-30T11:18:25.796Z).
	ts, err := observation.SnowflakeTime("175928847299117063")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Format(time.RFC3339Nano) != "2016-04-30T11:18:25.796Z" {
		t.Fatalf("snowflake time %s", ts)
	}
}
