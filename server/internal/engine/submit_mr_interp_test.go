package engine

import (
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestStrictInterpolateFailOnMissing(t *testing.T) {
	e, _ := setupEngine(t)
	c := &execCtx{
		run:  &models.Run{ID: "r1"},
		vars: map[string]any{"target_repo": "approving"},
	}

	r := e.strictInterpolate(c, "")
	if !r.ok || !r.emptyField || r.value != "" {
		t.Fatalf("empty field: %+v", r)
	}

	r = e.strictInterpolate(c, "{{vars.missing}}")
	if r.ok || !strings.Contains(r.err, "vars.missing") {
		t.Fatalf("missing var should fail: %+v", r)
	}

	r = e.strictInterpolate(c, "{{vars.target_repo}}")
	if !r.ok || r.value != "approving" || r.usedRepos {
		t.Fatalf("single var: %+v", r)
	}

	// Empty-string variable → ok interpolate but empty value (caller fails).
	c.vars["empty"] = ""
	r = e.strictInterpolate(c, "{{vars.empty}}")
	if !r.ok || r.value != "" {
		t.Fatalf("empty var value: %+v", r)
	}
}

func TestStrictInterpolateReposListBypass(t *testing.T) {
	e, _ := setupEngine(t)
	c := &execCtx{
		run: &models.Run{ID: "r1"},
		vars: map[string]any{
			"repos": []any{
				map[string]any{"name": "a", "url": "https://h/a.git"},
				map[string]any{"name": "b", "url": "https://h/b.git"},
			},
		},
	}

	r := e.strictInterpolate(c, "{{vars.repos}}")
	if !r.ok || !r.usedRepos || r.value != reposListSentinel {
		t.Fatalf("pure repos: %+v", r)
	}
	if detectSubmitMRRepoMode("{{vars.repos}}", r) != submitMRModeList {
		t.Fatalf("mode want list, got %s", detectSubmitMRRepoMode("{{vars.repos}}", r))
	}

	r = e.strictInterpolate(c, "x-{{vars.repos}}")
	if !r.ok || !r.usedRepos || !strings.Contains(r.value, reposListSentinel) {
		t.Fatalf("mixed repos: %+v", r)
	}
	if detectSubmitMRRepoMode("x-{{vars.repos}}", r) != submitMRModeList {
		t.Fatal("mixed should be list mode")
	}

	// Missing repos key while referencing it.
	c2 := &execCtx{run: &models.Run{ID: "r2"}, vars: map[string]any{}}
	r = e.strictInterpolate(c2, "{{vars.repos}}")
	if r.ok || !strings.Contains(r.err, "vars.repos") {
		t.Fatalf("missing repos key: %+v", r)
	}
}

func TestDetectSubmitMRRepoModeBlankSingle(t *testing.T) {
	blank := strictInterpResult{ok: true, emptyField: true}
	if detectSubmitMRRepoMode("", blank) != submitMRModeBlank {
		t.Fatal("empty → blank")
	}
	single := strictInterpResult{ok: true, value: "web"}
	if detectSubmitMRRepoMode("web", single) != submitMRModeSingle {
		t.Fatal("literal → single")
	}
	if detectSubmitMRRepoMode("{{vars.target_repo}}", strictInterpResult{ok: true, value: "web"}) != submitMRModeSingle {
		t.Fatal("interpolated single → single")
	}
}

func TestResolveStrictBranchField(t *testing.T) {
	e, _ := setupEngine(t)
	c := &execCtx{
		run:  &models.Run{ID: "r1"},
		vars: map[string]any{"feature_branch": "feat/x", "repos": []any{}},
	}
	got, err := e.resolveStrictBranchField(c, "", "源分支")
	if err != nil || got != "" {
		t.Fatalf("blank branch: %q %v", got, err)
	}
	got, err = e.resolveStrictBranchField(c, "{{vars.feature_branch}}", "源分支")
	if err != nil || got != "feat/x" {
		t.Fatalf("ok branch: %q %v", got, err)
	}
	_, err = e.resolveStrictBranchField(c, "{{vars.repos}}", "源分支")
	if err == nil {
		t.Fatal("repos as branch should fail")
	}
	_, err = e.resolveStrictBranchField(c, "{{vars.nope}}", "源分支")
	if err == nil {
		t.Fatal("missing branch var should fail")
	}
	_, err = e.resolveStrictBranchField(c, "{{vars.branches}}", "源分支")
	if err == nil || !strings.Contains(err.Error(), "branches") {
		t.Fatalf("branches whole table should fail: %v", err)
	}
	// Indirect: a string var holding JSON object dump must also fail.
	c.vars["bad_branch"] = `{"app":"feat/x"}`
	_, err = e.resolveStrictBranchField(c, "{{vars.bad_branch}}", "目标分支")
	if err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("JSON object as branch should fail: %v", err)
	}
}

func TestRepoNameInVars(t *testing.T) {
	vars := map[string]any{
		"repos": `[{"name":"web","url":"https://h/w.git"},{"name":"api","url":"https://h/a.git"}]`,
	}
	if !repoNameInVars("web", vars) || !repoNameInVars("api", vars) {
		t.Fatal("expected membership")
	}
	if repoNameInVars("missing", vars) {
		t.Fatal("missing should not match")
	}
}
