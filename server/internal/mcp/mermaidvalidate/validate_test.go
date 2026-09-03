package mermaidvalidate_test

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/mcp/mermaidvalidate"
)

func TestCheckLegalAndIllegal(t *testing.T) {
	if err := mermaidvalidate.Check("flowchart LR\n  A-->B"); err != nil {
		t.Fatalf("legal flowchart: %v", err)
	}
	if err := mermaidvalidate.Check("erDiagram\n  A ||--o{ B : has"); err != nil {
		t.Fatalf("legal er: %v", err)
	}
	if err := mermaidvalidate.Check("sequenceDiagram\n  A->>B: hi"); err != nil {
		t.Fatalf("legal sequence: %v", err)
	}
	err := mermaidvalidate.Check("flowchart LR\n  A-->[")
	if err == nil {
		t.Fatal("want error for truncated bracket")
	}
	if !strings.Contains(err.Error(), "Parse error") && !strings.Contains(err.Error(), "Expecting") {
		t.Fatalf("unexpected error: %v", err)
	}
}
