package materializer_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/ojiverse/datawarehouse/internal/materializer"
)

func ref(key string, t time.Time) materializer.ObjectRef {
	return materializer.ObjectRef{Key: key, UploadedAt: t}
}

func TestBuildManifestExcludesObjectsAfterCutoff(t *testing.T) {
	cutoff := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	objs := []materializer.ObjectRef{
		ref("b", cutoff.Add(-time.Hour)),
		ref("a", cutoff),
		ref("c", cutoff.Add(time.Hour)), // after cutoff: excluded
	}
	m := materializer.BuildManifest("run-1", "observations/", objs, cutoff, 10)
	var keys []string
	for _, c := range m.Chunks {
		keys = append(keys, c...)
	}
	want := []string{"a", "b"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("got %v want %v", keys, want)
	}
}

func TestBuildManifestIsDeterministicRegardlessOfInputOrder(t *testing.T) {
	cutoff := time.Now().UTC()
	objs := []materializer.ObjectRef{
		ref("z", cutoff.Add(-time.Hour)),
		ref("a", cutoff.Add(-time.Hour)),
		ref("m", cutoff.Add(-time.Hour)),
	}
	reversed := []materializer.ObjectRef{objs[2], objs[1], objs[0]}

	m1 := materializer.BuildManifest("run-1", "p/", objs, cutoff, 2)
	m2 := materializer.BuildManifest("run-1", "p/", reversed, cutoff, 2)
	if !reflect.DeepEqual(m1.Chunks, m2.Chunks) {
		t.Fatalf("manifest chunking depends on input order: %v vs %v", m1.Chunks, m2.Chunks)
	}
}

func TestBuildManifestSplitsIntoChunksOfConfiguredSize(t *testing.T) {
	cutoff := time.Now().UTC()
	objs := make([]materializer.ObjectRef, 5)
	for i := range objs {
		objs[i] = ref(string(rune('a'+i)), cutoff.Add(-time.Hour))
	}
	m := materializer.BuildManifest("run-1", "p/", objs, cutoff, 2)
	if len(m.Chunks) != 3 {
		t.Fatalf("expected 3 chunks (2,2,1), got %d: %v", len(m.Chunks), m.Chunks)
	}
	if len(m.Chunks[0]) != 2 || len(m.Chunks[1]) != 2 || len(m.Chunks[2]) != 1 {
		t.Fatalf("unexpected chunk sizes: %v", m.Chunks)
	}
}

func TestBuildManifestEmptyInput(t *testing.T) {
	m := materializer.BuildManifest("run-1", "p/", nil, time.Now(), 10)
	if len(m.Chunks) != 0 {
		t.Fatalf("expected no chunks, got %v", m.Chunks)
	}
}
