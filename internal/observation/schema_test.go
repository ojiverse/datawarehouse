package observation_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/ojiverse/datawarehouse/internal/observation"
)

// schemaPath is the authoritative Envelope v1 wire contract (ADR-0015).
const schemaPath = contractDir + "/schema.json"

func compileSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	c := jsonschema.NewCompiler()
	// Format assertions are optional in Draft 2020-12; the contract relies on
	// date-time and we want it enforced, not merely annotated.
	c.AssertFormat()
	sch, err := c.Compile(schemaPath)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

func validate(t *testing.T, sch *jsonschema.Schema, doc []byte) error {
	t.Helper()
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	return sch.Validate(inst)
}

// TestSharedFixtureConformsToSchema: the compatibility fixture must itself be
// a valid instance of the authoritative schema.
func TestSharedFixtureConformsToSchema(t *testing.T) {
	sch := compileSchema(t)
	paths, err := filepath.Glob(filepath.Join(contractDir, "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, p := range paths {
		if filepath.Base(p) == "schema.json" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := validate(t, sch, data); err != nil {
			t.Fatalf("%s does not conform to schema: %v", p, err)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no fixture validated")
	}
}

// TestDecodedFixtureReEncodesToValidSchemaInstance: the Go implementation
// artifact must produce schema-valid JSON after a decode → encode cycle.
func TestDecodedFixtureReEncodesToValidSchemaInstance(t *testing.T) {
	sch := compileSchema(t)
	envs, err := observation.LoadContractFixtures(contractDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, env := range envs {
		out, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if err := validate(t, sch, out); err != nil {
			t.Fatalf("re-encoded envelope violates schema: %v", err)
		}
	}
}

// TestGeneratedFixtureConformsToSchema: every page the spike generator emits
// must validate, so the closed loop always consumes contract-valid input.
func TestGeneratedFixtureConformsToSchema(t *testing.T) {
	sch := compileSchema(t)
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	for i, env := range envs {
		out, err := json.Marshal(env)
		if err != nil {
			t.Fatal(err)
		}
		if err := validate(t, sch, out); err != nil {
			t.Fatalf("generated page %d violates schema: %v", i, err)
		}
	}
}

// TestSchemaRejectsMissingProvenance guards that the validator is actually
// asserting the HTTP branch (a schema that accepts everything proves nothing).
func TestSchemaRejectsMissingProvenance(t *testing.T) {
	sch := compileSchema(t)
	data, err := os.ReadFile(filepath.Join(contractDir, "http-backfill-page.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc["provenance"].(map[string]any), "rate_limit")
	broken, _ := json.Marshal(doc)
	if err := validate(t, sch, broken); err == nil {
		t.Fatal("schema accepted an HTTP envelope without rate_limit")
	}
}
