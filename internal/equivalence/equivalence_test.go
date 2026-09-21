package equivalence_test

import (
	"testing"

	"github.com/ojiverse/datawarehouse/internal/equivalence"
)

func TestDiffIgnoresOrderAndDetectsChange(t *testing.T) {
	a := []equivalence.Row{{MessageID: "2", Content: "b"}, {MessageID: "1", Content: "a"}}
	b := []equivalence.Row{{MessageID: "1", Content: "a"}, {MessageID: "2", Content: "b"}}
	if d := equivalence.Diff(a, b); len(d) != 0 {
		t.Fatalf("expected no diff, got %v", d)
	}
	if equivalence.Digest(a) != equivalence.Digest(b) {
		t.Fatal("digest depends on order")
	}
	b[1].Content = "changed"
	if d := equivalence.Diff(a, b); len(d) != 2 {
		t.Fatalf("expected 2 diff lines, got %v", d)
	}
}
