package runtime

import (
	"path/filepath"
	"testing"
)

func TestSafeProfileNameAndDir(t *testing.T) {
	t.Parallel()
	if safeProfileName("..") != "" {
		t.Fatal(".. must be rejected")
	}
	if safeProfileName("../evil") != "" && safeProfileName("../evil") == ".." {
		t.Fatal("traversal must be rejected")
	}
	if safeProfileName("good-agent") != "good-agent" {
		t.Fatalf("got %q", safeProfileName("good-agent"))
	}
	if safeProfileName("has spaces") != "" {
		t.Fatal("spaces must be rejected")
	}

	root := t.TempDir()
	dir, err := profileDir(root, "good-agent")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(filepath.Join(root, "good-agent"))
	if dir != want {
		t.Fatalf("got %q want %q", dir, want)
	}
	if _, err := profileDir(root, ".."); err == nil {
		t.Fatal("expected error for ..")
	}
}
