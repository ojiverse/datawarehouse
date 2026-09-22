package projection_test

import (
	"math/rand"
	"reflect"
	"testing"

	"github.com/ojiverse/datawarehouse/internal/observation"
	"github.com/ojiverse/datawarehouse/internal/projection"
)

// TestReduceOfChunksMatchesWholeProjection is the determinism proof required
// by docs/domain/processing/projection.md: splitting the same Observation set
// into an arbitrary number of chunks, projecting each chunk independently,
// and reducing the per-chunk results must equal projecting the whole set at
// once. A resumable materializer relies on exactly this property to process
// Archive chunks independently and still converge on the same Current State.
func TestReduceOfChunksMatchesWholeProjection(t *testing.T) {
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	want, err := projection.Project(envs)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("empty projection")
	}

	r := rand.New(rand.NewSource(7))
	for trial, numChunks := range []int{1, 2, 3, 5, len(envs)} {
		if numChunks == 0 {
			continue
		}
		shuffled := append([]observation.Envelope(nil), envs...)
		r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })

		chunks := make([][]observation.Envelope, numChunks)
		for i, e := range shuffled {
			c := i % numChunks
			chunks[c] = append(chunks[c], e)
		}

		var groups [][]projection.CanonicalMessage
		for _, c := range chunks {
			if len(c) == 0 {
				continue
			}
			rows, err := projection.Project(c)
			if err != nil {
				t.Fatal(err)
			}
			groups = append(groups, rows)
		}
		got := projection.Reduce(groups...)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("trial %d (numChunks=%d): reduced chunks differ from whole projection", trial, numChunks)
		}
	}
}

func TestReduceIsOrderIndependent(t *testing.T) {
	envs, err := observation.GenerateFixture(observation.DefaultFixtureSpec())
	if err != nil {
		t.Fatal(err)
	}
	a, err := projection.Project(envs[:len(envs)/2])
	if err != nil {
		t.Fatal(err)
	}
	b, err := projection.Project(envs[len(envs)/2:])
	if err != nil {
		t.Fatal(err)
	}
	got1 := projection.Reduce(a, b)
	got2 := projection.Reduce(b, a)
	if !reflect.DeepEqual(got1, got2) {
		t.Fatal("Reduce is not order independent")
	}
}
