// Package mermaidvalidate runs Mermaid 11.x parse() via Node to keep set_plan
// diagram syntax checks aligned with the PlanView frontend.
package mermaidvalidate

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed bundle.mjs
var bundleMJS []byte

var (
	nodeOnce   sync.Once
	nodePath   string
	nodeErr    error
	bundleOnce sync.Once
	bundleFile string
	bundleErr  error
)

// Check parses source with Mermaid 11.x. Empty source is the caller's concern.
func Check(source string) error {
	src := strings.TrimSpace(source)
	if src == "" {
		return errors.New("empty source")
	}
	nodeBin, err := lookUpNode()
	if err != nil {
		return fmt.Errorf("mermaid 语法校验需要 node: %w", err)
	}
	bundle, err := materializeBundle()
	if err != nil {
		return fmt.Errorf("mermaid 语法校验脚本不可用: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, nodeBin, bundle, "-")
	cmd.Stdin = strings.NewReader(src)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	if runErr == nil {
		return nil
	}
	msg := strings.TrimSpace(stderr.String())
	if msg == "" {
		msg = strings.TrimSpace(stdout.String())
	}
	if msg == "" {
		msg = runErr.Error()
	}
	// Single-line reason for MCP tool errors.
	msg = strings.Join(strings.Fields(msg), " ")
	if len(msg) > 400 {
		msg = msg[:397] + "..."
	}
	return errors.New(msg)
}

func lookUpNode() (string, error) {
	nodeOnce.Do(func() {
		if p := strings.TrimSpace(os.Getenv("APPROVING_NODE")); p != "" {
			nodePath = p
			return
		}
		p, err := exec.LookPath("node")
		if err != nil {
			nodeErr = err
			return
		}
		nodePath = p
	})
	return nodePath, nodeErr
}

func materializeBundle() (string, error) {
	bundleOnce.Do(func() {
		if p := strings.TrimSpace(os.Getenv("APPROVING_MERMAID_VALIDATE_BUNDLE")); p != "" {
			bundleFile = p
			return
		}
		dir, err := os.MkdirTemp("", "approving-mermaid-validate-*")
		if err != nil {
			bundleErr = err
			return
		}
		path := filepath.Join(dir, "bundle.mjs")
		if err := os.WriteFile(path, bundleMJS, 0o644); err != nil {
			bundleErr = err
			return
		}
		bundleFile = path
	})
	return bundleFile, bundleErr
}
