package observation_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// contractDir is the shared cross-language fixture directory (ADR-0014).
const contractDir = "../../contracts/observation-envelope/v1"

// canonical re-serialises arbitrary JSON with sorted keys and no insignificant
// whitespace so two documents can be compared byte-for-byte.
func canonical(t *testing.T, data []byte) []byte {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestSharedFixtureRoundTrip is the compatibility oracle: every shared fixture
// must decode into the Go Envelope and re-encode to the same canonical JSON,
// including JSON nulls (pagination.after) and the diagnostic rate_limit block.
func TestSharedFixtureRoundTrip(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join(contractDir, "*.json"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no shared fixtures found in %s (%v)", contractDir, err)
	}
	for _, p := range paths {
		t.Run(filepath.Base(p), func(t *testing.T) {
			data, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			env, err := observation.DecodeJSON(data)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			encoded, err := json.Marshal(env)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := canonical(t, encoded), canonical(t, data); !bytes.Equal(got, want) {
				t.Fatalf("canonical round-trip differs\n got: %s\nwant: %s", got, want)
			}
			gz, err := observation.EncodeGzipJSON(env)
			if err != nil {
				t.Fatal(err)
			}
			again, err := observation.DecodeGzipJSON(bytes.NewReader(gz))
			if err != nil {
				t.Fatal(err)
			}
			// encoding/json compacts RawMessage, so compare the payload
			// canonically and everything else structurally.
			if !bytes.Equal(canonical(t, again.Payload), canonical(t, env.Payload)) {
				t.Fatal("gzip round-trip changed the payload")
			}
			again.Payload, env.Payload = nil, nil
			if !reflect.DeepEqual(again, env) {
				t.Fatalf("gzip round-trip changed the envelope\n got: %+v\nwant: %+v", again, env)
			}
		})
	}
}

// TestSharedFixtureFieldsAreTyped guards the fields the review called out so a
// silent regression to map-typed pagination or a renamed provenance key fails.
func TestSharedFixtureFieldsAreTyped(t *testing.T) {
	envs, err := observation.LoadContractFixtures(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	env := envs[0]
	p := env.Provenance
	if p == nil || p.Endpoint == "" || p.RateLimit == nil {
		t.Fatalf("provenance not decoded: %+v", p)
	}
	if p.Pagination.Before == nil || *p.Pagination.Before != "1000003" || p.Pagination.After != nil {
		t.Fatalf("pagination %+v", p.Pagination)
	}
	if p.RateLimit.ResetAfterSeconds != 1.5 || p.RateLimit.Bucket != "route-bucket-hash" {
		t.Fatalf("rate_limit %+v", p.RateLimit)
	}
	msgs, err := observation.DecodeHTTPPage(env.Payload)
	if err != nil || len(msgs) != 2 || msgs[1].EditedTimestamp == nil {
		t.Fatalf("payload decode %v %+v", err, msgs)
	}
}

// TestGeneratedFixtureMatchesContractShape ensures the spike generator emits
// the same top-level and provenance keys as the shared fixture.
func TestGeneratedFixtureMatchesContractShape(t *testing.T) {
	shared, err := os.ReadFile(filepath.Join(contractDir, "http-backfill-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	gen, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	genJSON, err := json.Marshal(gen[0])
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keySet(t, genJSON), keySet(t, shared); !reflect.DeepEqual(got, want) {
		t.Fatalf("key shape differs\n got: %v\nwant: %v", got, want)
	}
}

func keySet(t *testing.T, data []byte) map[string][]string {
	t.Helper()
	var doc struct {
		Top        map[string]json.RawMessage `json:"-"`
		Provenance map[string]json.RawMessage `json:"provenance"`
	}
	if err := json.Unmarshal(data, &doc.Top); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	out := map[string][]string{"top": sortedKeys(doc.Top), "provenance": sortedKeys(doc.Provenance)}
	var pag map[string]json.RawMessage
	if err := json.Unmarshal(doc.Provenance["pagination"], &pag); err != nil {
		t.Fatal(err)
	}
	out["pagination"] = sortedKeys(pag)
	return out
}

func sortedKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := range keys {
		for j := i + 1; j < len(keys); j++ {
			if keys[j] < keys[i] {
				keys[i], keys[j] = keys[j], keys[i]
			}
		}
	}
	return keys
}
