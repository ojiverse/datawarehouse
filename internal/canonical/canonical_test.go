package canonical_test

import (
	"bytes"
	"testing"

	"github.com/ojiverse/datawarehouse/internal/canonical"
	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

func fixtureRows(t *testing.T) ([]observation.Envelope, []projection.CanonicalMessage) {
	t.Helper()
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := projection.Project(envs)
	if err != nil {
		t.Fatal(err)
	}
	return envs, rows
}

func TestWriteParquetUsesZstdAndKeepsRows(t *testing.T) {
	_, rows := fixtureRows(t)
	var buf bytes.Buffer
	if err := canonical.WriteParquet(&buf, rows); err != nil {
		t.Fatal(err)
	}
	n, codecs, err := canonical.InspectParquet(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(rows)) {
		t.Fatalf("rows %d, want %d", n, len(rows))
	}
	if _, ok := codecs["ZSTD"]; !ok || len(codecs) != 1 {
		t.Fatalf("codecs %v, want only ZSTD", codecs)
	}
}

func TestArrowSchemaCarriesFieldIDs(t *testing.T) {
	sc, err := canonical.ArrowSchema()
	if err != nil {
		t.Fatal(err)
	}
	if sc.NumFields() != len(canonical.MessageSchema().Fields()) {
		t.Fatalf("arrow fields %d != iceberg fields %d", sc.NumFields(), len(canonical.MessageSchema().Fields()))
	}
	if v, ok := sc.Field(0).Metadata.GetValue("PARQUET:field_id"); !ok || v != "1" {
		t.Fatalf("field id metadata missing: %v", sc.Field(0).Metadata)
	}
}

func TestChunkIDIsOrderIndependent(t *testing.T) {
	envs, _ := fixtureRows(t)
	ids := make([]observation.ObservationID, 0, len(envs))
	for _, e := range envs {
		ids = append(ids, e.ObservationID)
	}
	a := canonical.ChunkID(ids)
	rev := make([]observation.ObservationID, len(ids))
	for i := range ids {
		rev[len(ids)-1-i] = ids[i]
	}
	if b := canonical.ChunkID(rev); a != b {
		t.Fatalf("chunk id depends on order: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Fatalf("chunk id %q is not sha256 hex", a)
	}
}
