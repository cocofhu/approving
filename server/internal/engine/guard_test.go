package engine

import "testing"

func ctxFixture() EvalContext {
	arts := map[string]string{
		"ci_result.json":     `{"success": true, "status": "success"}`,
		"design_review.json": `{"complete": false}`,
	}
	return EvalContext{
		Vars:  map[string]any{"attempt": float64(2), "approved": true, "name": "alice", "max_ci_fix": float64(3)},
		Nodes: map[string]map[string]any{"design": {"outputs": map[string]any{"content": "doc"}}},
		Extra: map[string]any{"action": "approve"},
		Artifact: func(name string) (string, bool) {
			c, ok := arts[name]
			return c, ok
		},
	}
}

func TestGuardPasses(t *testing.T) {
	ctx := ctxFixture()
	cases := []struct {
		expr string
		want bool
	}{
		{"", true},
		{"default", true},
		{"action == 'approve'", true},
		{"action == 'revise'", false},
		{"vars.attempt < vars.max_ci_fix", true},
		{"vars.attempt > vars.max_ci_fix", false},
		{"vars.approved == true", true},
		{"vars.name == 'alice'", true},
		{"exists(\"ci_result.json\")", true},
		{"exists(\"missing.json\")", false},
		{"json(\"ci_result.json\").success == true", true},
		{"json(\"design_review.json\").complete == true", false},
		{"vars.attempt < vars.max_ci_fix && vars.approved == true", true},
		{"vars.attempt > 9 || action == 'approve'", true},
	}
	for _, c := range cases {
		if got := guardPasses(c.expr, ctx); got != c.want {
			t.Errorf("guardPasses(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestArithmeticExpr(t *testing.T) {
	ctx := ctxFixture()
	v, err := evalExpr("vars.attempt + 1", ctx)
	if err != nil {
		t.Fatalf("eval err: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 3 {
		t.Errorf("attempt+1 = %v, want 3", v)
	}
}

func TestGuardExpressionBranches(t *testing.T) {
	ctx := ctxFixture()
	// nodeOutputs are flat (nodes.<id>.outputs.<field> resolves against the
	// outputs map stored directly under the node id).
	ctx.Nodes["impl"] = map[string]any{"content": "doc"}
	cases := []struct {
		expr string
		want bool
	}{
		{"vars.attempt - 1 == 1", true},
		{"vars.attempt >= 2 && vars.approved", true},
		{"vars.name != 'bob'", true},
		{"json(\"ci_result.json\").status == 'success'", true},
		{"nodes.impl.outputs.content == 'doc'", true},
		{"nodes.missing.outputs.x == 'y'", false},
		{"artifact(\"ci_result.json\") != ''", true},
		{"true", true},
		{"false", false},
		{"vars.attempt <= 2", true},
	}
	for _, c := range cases {
		if got := guardPasses(c.expr, ctx); got != c.want {
			t.Errorf("guardPasses(%q) = %v, want %v", c.expr, got, c.want)
		}
	}
}

func TestEvalExprErrors(t *testing.T) {
	ctx := ctxFixture()
	if _, err := evalExpr("1 +", ctx); err == nil {
		t.Error("expected parse error for dangling operator")
	}
	if _, err := evalExpr("unknownfn(1)", ctx); err == nil {
		t.Error("expected unknown function error")
	}
	if _, err := evalExpr("(1", ctx); err == nil {
		t.Error("expected unbalanced paren error")
	}
	if _, err := evalExpr("'a' + 1", ctx); err == nil {
		t.Error("expected non-numeric arithmetic error")
	}
}

func TestParseCallEdgeCases(t *testing.T) {
	arts := map[string]string{
		"ok.json":  `{"a":{"b":42},"flag":true}`,
		"bad.json": `not json`,
	}
	ctx := EvalContext{
		Vars: map[string]any{},
		Artifact: func(n string) (string, bool) {
			c, ok := arts[n]
			return c, ok
		},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{`exists("ok.json")`, true},
		{`exists("ok.json","extra") == false`, true}, // wrong arg count -> false
		{`json("ok.json").a.b == 42`, true},          // nested digPath
		{`json("ok.json").flag == true`, true},
		{`json("bad.json") == ''`, false},     // bad json -> nil, nil != ''
		{`json("missing.json") == ''`, false}, // absent -> nil
		{`artifact("ok.json") != ''`, true},
		{`artifact("missing.json") == ''`, true}, // absent -> ""
	}
	for _, c := range cases {
		if got := guardPasses(c.expr, ctx); got != c.want {
			t.Errorf("guardPasses(%q) = %v want %v", c.expr, got, c.want)
		}
	}

	// Artifact-less context: exists/artifact/json degrade to false/empty/nil.
	noArt := EvalContext{Vars: map[string]any{}}
	if guardPasses(`exists("x")`, noArt) {
		t.Error("exists with nil Artifact should be false")
	}
	if guardPasses(`artifact("x") != ''`, noArt) {
		t.Error("artifact with nil Artifact should be empty")
	}
	if guardPasses(`json("x").a == 1`, noArt) {
		t.Error("json with nil Artifact should be nil")
	}
}

func TestDigPathNonMap(t *testing.T) {
	m := map[string]any{"a": map[string]any{"b": "leaf"}}
	if digPath(m, "a.b") != "leaf" {
		t.Error("nested path")
	}
	if digPath(m, "a.b.c") != nil {
		t.Error("descending into a non-map should be nil")
	}
	if digPath(m, "missing") != nil {
		t.Error("missing key should be nil")
	}
}

func TestTruthyAndToFloat(t *testing.T) {
	truthyCases := []struct {
		in   any
		want bool
	}{
		{true, true}, {false, false}, {float64(0), false}, {float64(2), true},
		{int(0), false}, {int(3), true}, {"", false}, {"false", false}, {"x", true},
		{nil, false}, {[]int{1}, true}, // non-nil struct/slice -> default true
	}
	for _, c := range truthyCases {
		if got := truthy(c.in); got != c.want {
			t.Errorf("truthy(%v) = %v want %v", c.in, got, c.want)
		}
	}
	if f, ok := toFloat("3.5"); !ok || f != 3.5 {
		t.Errorf("toFloat string = %v,%v", f, ok)
	}
	if f, ok := toFloat(true); !ok || f != 1 {
		t.Errorf("toFloat bool = %v,%v", f, ok)
	}
	if _, ok := toFloat([]int{}); ok {
		t.Error("toFloat slice should fail")
	}
}
