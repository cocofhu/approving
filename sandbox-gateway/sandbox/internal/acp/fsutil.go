package acp

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadTextFile reads a file with optional 1-based line start and max lines (0 = all).
func ReadTextFile(absPath string, lineStart int, limit int) (string, error) {
	b, err := os.ReadFile(absPath)
	if err != nil {
		return "", err
	}
	if lineStart <= 1 && (limit <= 0) {
		return string(b), nil
	}
	s := bufio.NewScanner(strings.NewReader(string(b)))
	var lines []string
	n := 0
	for s.Scan() {
		n++
		if n < lineStart {
			continue
		}
		lines = append(lines, s.Text())
		if limit > 0 && len(lines) >= limit {
			break
		}
	}
	return strings.Join(lines, "\n"), s.Err()
}

// WriteTextFile writes text, creating parent dirs. Path must be absolute.
func WriteTextFile(absPath string, content string) error {
	if !filepath.IsAbs(absPath) {
		return fmt.Errorf("path must be absolute: %s", absPath)
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// EnsurePathAllowed checks path is under root (if root non-empty).
func EnsurePathAllowed(root, path string) error {
	if root == "" {
		return nil
	}
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	rel, err := filepath.Rel(root, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return fmt.Errorf("path outside workspace: %s", path)
	}
	return nil
}
