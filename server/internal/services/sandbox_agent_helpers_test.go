package services

import (
	"testing"

	"github.com/cocofhu/approving/internal/sandbox"
)

func TestFilterAgentPlatformMCPAndDedupe(t *testing.T) {
	filtered := filterAgentPlatformMCP([]sandbox.MCPServerSpec{
		{Name: MemoryStoreMCP, URL: "http://mem"},
		{Name: "custom", URL: "http://x"},
	})
	if len(filtered) != 1 || filtered[0].Name != "custom" {
		t.Fatalf("filtered=%v", filtered)
	}
	deduped := dedupeMCPByName([]sandbox.MCPServerSpec{
		{Name: "a", URL: "1"},
		{Name: "a", URL: "2"},
		{Name: "b", URL: "3"},
	})
	if len(deduped) != 2 || deduped[0].URL != "1" {
		t.Fatalf("dedupe=%v", deduped)
	}
	if sanitizeID("Hello World!") != "HelloWorld!" {
		t.Fatalf("sanitize=%q", sanitizeID("Hello World!"))
	}
}

func TestIsAgentSandboxPurpose(t *testing.T) {
	if !isAgentSandboxPurpose(SandboxPurposeAgent) || !isAgentSandboxPurpose(SandboxPurposePM) {
		t.Fatal("agent purposes")
	}
	if isAgentSandboxPurpose("test") {
		t.Fatal("test is not agent purpose")
	}
}
