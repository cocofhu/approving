package mcp

import (
	"strings"
	"testing"
)

func TestExpectedKindForReservedName(t *testing.T) {
	cases := []struct {
		name string
		kind string
		ok   bool
	}{
		{"page.html", "html", true},
		{PlanArtifactName, "json", true},
		{ClarifiedRequirementArtifactName, "json", true},
		{ResearchArtifactName, "json", true},
		{ProposalsArtifactName, "json", true},
		{ProposalArtifactName, "json", true},
		{TestResultArtifactName, "json", true},
		{ReviewArtifactName, "json", true},
		{ImplementationResultArtifactName, "json", true},
		{NodeOutcomeArtifactName, "json", true},
		{FeedbackIndexArtifactName, "json", true},
		{"feedback.clarify.approve_1.i1.json", "json", true},
		{"note.md", "", false},
		{"brand-row-preview.html", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := ExpectedKindForReservedName(tc.name)
		if ok != tc.ok || got != tc.kind {
			t.Fatalf("%q: got (%q,%v) want (%q,%v)", tc.name, got, ok, tc.kind, tc.ok)
		}
	}
}

func TestInferWriteArtifactKind(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"a.json", "json"},
		{"a.YAML", "yaml"},
		{"b.yml", "yaml"},
		{"page.html", "html"},
		{"x.HTM", "html"},
		{"note.md", "markdown"},
		{"README.markdown", "markdown"},
		{"plain.txt", "text"},
		{"shot.png", "image"},
		{"photo.JPG", "image"},
		{"noext", "markdown"},
	}
	for _, tc := range cases {
		if got := InferWriteArtifactKind(tc.name); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.name, got, tc.want)
		}
	}
}

func TestResolveWriteArtifactKind(t *testing.T) {
	cases := []struct {
		name    string
		kind    string
		want    string
		wantErr string
	}{
		// g1.2 / g2.1: empty kind → reserved or extension
		{"page.html", "", "html", ""},
		{PlanArtifactName, "", "json", ""},
		{"note.md", "", "markdown", ""},
		{"cfg.yaml", "", "yaml", ""},
		{"freeform", "", "markdown", ""},

		// g1.4 / g2.1 / g2.3: reserved mismatch
		{"page.html", "image", "", "期望 kind=\"html\""},
		{"page.html", "markdown", "", "期望 kind=\"html\""},
		{PlanArtifactName, "markdown", "", "期望 kind=\"json\""},
		{PlanArtifactName, "html", "", "期望 kind=\"json\""},

		// g1.3 / g2.2: any name + image
		{"shot-1.png", "image", "", "artifact-upload"},
		{"note.md", "image", "", "artifact-upload"},
		{"shot-1.png", "", "", "artifact-upload"}, // empty infers image then reject

		// success paths (f4)
		{"page.html", "html", "html", ""},
		{PlanArtifactName, "json", "json", ""},
		{"note.md", "markdown", "markdown", ""},
		{"foo.txt", "html", "html", ""}, // non-reserved: no extension/kind force match
	}
	for _, tc := range cases {
		got, err := ResolveWriteArtifactKind(tc.name, tc.kind)
		if tc.wantErr != "" {
			if err == nil {
				t.Fatalf("%s kind=%q: want error containing %q, got nil", tc.name, tc.kind, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("%s kind=%q: error %q missing %q", tc.name, tc.kind, err.Error(), tc.wantErr)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s kind=%q: unexpected err %v", tc.name, tc.kind, err)
		}
		if got != tc.want {
			t.Fatalf("%s kind=%q: got %q want %q", tc.name, tc.kind, got, tc.want)
		}
	}
}

// TestWriteArtifactKindValidation covers the MCP write path (g2.1–g2.4):
// page.html image/html/empty; arbitrary name + image; reserved json mismatch;
// error text includes expected kind / artifact-upload guidance.
func TestWriteArtifactKindValidation(t *testing.T) {
	store := &memStore{}
	h := NewHost(store)
	runID := "kind-run"
	tok := h.RegisterRun(runID)

	assertErrContains := func(t *testing.T, body, substr string) {
		t.Helper()
		resp := call(t, h, runID, tok, body)
		result := resp["result"].(map[string]any)
		if result["isError"] != true {
			t.Fatalf("want isError, got: %v", resp)
		}
		text := result["content"].([]any)[0].(map[string]any)["text"].(string)
		if !strings.Contains(text, substr) {
			t.Fatalf("error text %q missing %q", text, substr)
		}
	}

	assertOK := func(t *testing.T, body, name, wantKind string) {
		t.Helper()
		resp := call(t, h, runID, tok, body)
		result := resp["result"].(map[string]any)
		if result["isError"] == true {
			t.Fatalf("want success, got: %v", resp)
		}
		if got := store.kindOf(runID, name); got != wantKind {
			t.Fatalf("stored kind for %q = %q, want %q", name, got, wantKind)
		}
	}

	// g2.1 page.html + image fails (g2.4: expected kind + artifact-upload)
	assertErrContains(t,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"page.html","content":"<html/>","kind":"image"}}}`,
		"html",
	)
	assertErrContains(t,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"page.html","content":"<html/>","kind":"image"}}}`,
		"artifact-upload",
	)
	if _, ok := store.Get(runID, "page.html"); ok {
		t.Fatal("page.html must not be stored after kind=image failure")
	}

	// g2.1 page.html + html succeeds
	assertOK(t,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"page.html","content":"<html>ok</html>","kind":"html"}}}`,
		"page.html", "html",
	)

	// g2.1 page.html empty kind → html
	assertOK(t,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"page.html","content":"<html>inferred</html>"}}}`,
		"page.html", "html",
	)

	// g2.2 any name + image fails
	assertErrContains(t,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"shot-1.png","content":"PNG","kind":"image"}}}`,
		"artifact-upload",
	)
	assertErrContains(t,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"note.md","content":"x","kind":"image"}}}`,
		"artifact-upload",
	)
	if _, ok := store.Get(runID, "shot-1.png"); ok {
		t.Fatal("shot-1.png must not be stored via write_artifact")
	}

	// g2.3 plan.json + markdown fails
	assertErrContains(t,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"plan.json","content":"{}","kind":"markdown"}}}`,
		"json",
	)
	if _, ok := store.Get(runID, PlanArtifactName); ok {
		t.Fatal("plan.json must not be stored with wrong kind")
	}

	// non-reserved + legal text kind still works
	assertOK(t,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"write_artifact","arguments":{"name":"note.md","content":"hi","kind":"markdown"}}}`,
		"note.md", "markdown",
	)
}
