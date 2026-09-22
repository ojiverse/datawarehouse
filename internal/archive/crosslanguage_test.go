package archive_test

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// tsProducerFixtureDir holds a real Archive object body (gzip-compressed
// Envelope v1 JSON) produced by running the actual TypeScript writer
// (workers/backfill/src/observation/archive-object.ts, buildArchiveObject)
// against the shared contract fixture. It is regenerated with
// `pnpm --dir workers/backfill run generate:cross-language-fixture`
// (see workers/backfill/scripts/generate-cross-language-archive-fixture.ts).
//
// This is the cross-language integration proof required by #38: the Go
// replay reader must decode bytes the TypeScript producer actually emits,
// not only a hand-authored fixture that merely conforms to the JSON Schema.
const tsProducerFixtureDir = "testdata/ts_producer"

// tsArchiveObjectMetadata mirrors the R2 custom/http metadata the TypeScript
// producer attaches to the object (workers/backfill/src/observation/archive-object.ts).
type tsArchiveObjectMetadata struct {
	Key            string `json:"key"`
	ObservationID  string `json:"observation_id"`
	CustomMetadata struct {
		Format          string `json:"format"`
		Compression     string `json:"compression"`
		EnvelopeVersion string `json:"envelope_version"`
		ObservationID   string `json:"observation_id"`
		SourceKind      string `json:"source_kind"`
		PayloadSHA256   string `json:"payload_sha256"`
	} `json:"custom_metadata"`
	HTTPMetadata struct {
		ContentType     string `json:"contentType"`
		ContentEncoding string `json:"contentEncoding"`
	} `json:"http_metadata"`
}

func loadTSProducedObject(t *testing.T) ([]byte, tsArchiveObjectMetadata) {
	t.Helper()
	body, err := os.ReadFile(tsProducerFixtureDir + "/archive-object.json.gz")
	if err != nil {
		t.Fatalf("read ts-produced archive object (run `pnpm --dir workers/backfill run generate:cross-language-fixture`?): %v", err)
	}
	rawMeta, err := os.ReadFile(tsProducerFixtureDir + "/archive-object.metadata.json")
	if err != nil {
		t.Fatalf("read ts-produced archive object metadata: %v", err)
	}
	var meta tsArchiveObjectMetadata
	if err := json.Unmarshal(rawMeta, &meta); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	return body, meta
}

// TestDecodeRealTypeScriptProducedObject is the cross-language integration
// proof: it decodes gzip bytes the TypeScript writer actually produced (not a
// Go re-encoding of the shared fixture) using the same decoder the replay
// reader (Client.Get) uses, and checks the result against metadata the
// TypeScript side computed independently.
func TestDecodeRealTypeScriptProducedObject(t *testing.T) {
	body, meta := loadTSProducedObject(t)

	env, err := observation.DecodeGzipJSON(bytes.NewReader(body))
	if err != nil {
		t.Fatalf("decode ts-produced object: %v", err)
	}

	if env.EnvelopeVersion != observation.EnvelopeVersion {
		t.Fatalf("envelope_version = %q, want %q", env.EnvelopeVersion, observation.EnvelopeVersion)
	}
	if got, want := env.ObservationID.String(), meta.ObservationID; got != want {
		t.Fatalf("observation_id = %q, want %q", got, want)
	}
	if got, want := string(env.SourceKind), meta.CustomMetadata.SourceKind; got != want {
		t.Fatalf("source_kind = %q, want %q", got, want)
	}
	// The retry-consistency hash (archive-write-path.md "No Overwrite") must
	// agree even though it was computed by two independent implementations.
	if got, want := env.PayloadSHA256(), meta.CustomMetadata.PayloadSHA256; got != want {
		t.Fatalf("payload sha256 = %q, want %q (Go and TypeScript disagree on the retry-consistency hash)", got, want)
	}
	// The R2 key (docs/infrastructure/cloudflare/r2/README.md) is derived by
	// both languages from the same Envelope fields; agreement here proves the
	// partitioning rule, not just the string format, is shared.
	if got, want := env.ArchiveKey(), meta.Key; got != want {
		t.Fatalf("archive key = %q, want %q", got, want)
	}
	if err := env.Validate(); err != nil {
		t.Fatalf("ts-produced envelope failed validation: %v", err)
	}

	assertPayloadMatchesContractFixture(t, env.Payload)
}

// assertPayloadMatchesContractFixture proves the full HTTP response body
// survived the TypeScript encode -> gzip -> Go decode round trip without
// truncation or reinterpretation (docs/domain/observations/envelope.md "HTTP
// Observation": "Payload には response body 全体を保存する").
func assertPayloadMatchesContractFixture(t *testing.T, payload json.RawMessage) {
	t.Helper()
	fixture, err := os.ReadFile("../../contracts/observation-envelope/v1/http-backfill-page.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(fixture, &doc); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(doc.Payload, &want); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(got)
	wantJSON, _ := json.Marshal(want)
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("payload round trip lost or changed data\n got: %s\nwant: %s", gotJSON, wantJSON)
	}
}

// TestIdentityExtractedFromContentNotPlacement proves the Observation ID
// (docs/domain/observations/identity.md "物理配置との分離": "Observation ID は
// object key ... から独立する") comes from the decoded Envelope content, not
// from wherever the bytes happened to be stored. It stores the real
// TypeScript-produced bytes under a key that does not match the Envelope at
// all (as compaction or a rewrite might do) and confirms the decoded identity
// is unaffected.
func TestIdentityExtractedFromContentNotPlacement(t *testing.T) {
	body, meta := loadTSProducedObject(t)

	store := map[string][]byte{
		"some/unrelated/compacted-segment-0042.json.gz": body,
	}
	stored := store["some/unrelated/compacted-segment-0042.json.gz"]

	env, err := observation.DecodeGzipJSON(bytes.NewReader(stored))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got, want := env.ObservationID.String(), meta.ObservationID; got != want {
		t.Fatalf("observation id derived from placement instead of content: got %q, want %q", got, want)
	}
}

// TestMalformedTypeScriptEnvelopeIsRejected proves a structurally broken
// Envelope is reported as an error by the same decode path Client.Get uses,
// rather than silently accepted as a zero-value Envelope (#38 Success
// Criteria "malformed / unsupported Envelope is not silently ignored").
func TestMalformedTypeScriptEnvelopeIsRejected(t *testing.T) {
	body, _ := loadTSProducedObject(t)

	t.Run("truncated gzip stream", func(t *testing.T) {
		truncated := body[:len(body)/2]
		if _, err := observation.DecodeGzipJSON(bytes.NewReader(truncated)); err == nil {
			t.Fatal("expected an error decoding a truncated gzip stream, got nil")
		}
	})

	t.Run("missing required field after re-encoding", func(t *testing.T) {
		env, err := observation.DecodeGzipJSON(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		doc := map[string]any{}
		encoded, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &doc); err != nil {
			t.Fatal(err)
		}
		delete(doc, "observation_id")
		broken, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := observation.DecodeJSON(broken); err == nil {
			t.Fatal("expected an error decoding an envelope without observation_id, got nil")
		}
	})

	t.Run("unsupported envelope_version", func(t *testing.T) {
		env, err := observation.DecodeGzipJSON(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		doc := map[string]any{}
		encoded, _ := json.Marshal(env)
		if err := json.Unmarshal(encoded, &doc); err != nil {
			t.Fatal(err)
		}
		doc["envelope_version"] = "v2"
		broken, _ := json.Marshal(doc)
		if _, err := observation.DecodeJSON(broken); err == nil {
			t.Fatal("expected an error decoding envelope_version v2, got nil")
		}
	})
}
