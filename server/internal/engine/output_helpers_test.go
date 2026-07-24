package engine

import (
	"github.com/cocofhu/approving/internal/nodereg"
	"testing"
)

func TestIsKnownStructuredArtifact(t *testing.T) {
	m := nodereg.BuildManifest()
	if len(m.ArtifactToOutputJSON) == 0 {
		t.Fatal("empty manifest")
	}
	for name := range m.ArtifactToOutputJSON {
		if !isKnownStructuredArtifact(name) {
			t.Fatalf("expected known: %s", name)
		}
	}
	if isKnownStructuredArtifact("random.md") {
		t.Fatal("unknown should be false")
	}
}
