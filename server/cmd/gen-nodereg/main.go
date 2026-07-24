package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cocofhu/approving/internal/nodereg"
)

func workspaceRoot() string {
	if root := os.Getenv("APPROVING_ROOT"); root != "" {
		return root
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	dir := wd
	for i := 0; i < 6; i++ {
		if _, err := os.Stat(filepath.Join(dir, "web")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "server")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

// Generates web/src/data/nodeManifest.generated.json from the Go node registry.
func main() {
	root := workspaceRoot()
	out := filepath.Join(root, "web", "src", "data", "nodeManifest.generated.json")
	m := nodereg.BuildManifest()
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal manifest: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "write %s: %v\n", out, err)
		os.Exit(1)
	}
	fmt.Println("wrote", out)
}
