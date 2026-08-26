package services

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Guards against reintroducing parentAgent reporting-line business code.
func TestNoParentAgentReportingDeadCode(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := []string{
		"IsInReportingClosure",
		"detectReportingCycles",
		"ParentAgent string",
		`json:"parentAgent`,
		"wouldCreateReportingCycle",
		"reportingClosure(",
	}
	var offenders []string
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "vendor" || info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := string(b)
		for _, needle := range patterns {
			if strings.Contains(s, needle) {
				offenders = append(offenders, path+": contains "+needle)
			}
		}
		return nil
	})
	if len(offenders) > 0 {
		t.Fatalf("reporting-line residue:\n%s", strings.Join(offenders, "\n"))
	}
}
