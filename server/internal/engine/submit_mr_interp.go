package engine

import (
	"fmt"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/runtime"
)

// reposListSentinel replaces {{vars.repos}} during strict interpolation so the
// expanded value is never mistaken for a repo name (VarDisplayText of a list
// would be a JSON dump / []any string).
const reposListSentinel = "__REPOS_LIST__"

// strictInterpResult is the fail-sensitive interpolation outcome for submit_mr
// config fields (repo / source_branch / target_branch).
type strictInterpResult struct {
	ok         bool
	value      string
	emptyField bool
	usedRepos  bool
	err        string
}

// strictInterpolate resolves {{vars.x}} / {{nodes.x.outputs.y}} with fail-on-
// missing semantics for submit_mr fields. Empty tmpl is allowed (emptyField).
// {{vars.repos}} is never expanded via VarDisplayText — it sets usedRepos and
// substitutes a sentinel. Global prompt interpolate (unknown→empty) is unchanged.
func (e *Engine) strictInterpolate(c *execCtx, tmpl string) strictInterpResult {
	if strings.TrimSpace(tmpl) == "" {
		return strictInterpResult{ok: true, value: "", emptyField: true}
	}
	out := tmpl
	usedRepos := false
	ec := e.evalContext(c, nil)
	for {
		i := strings.Index(out, "{{")
		if i < 0 {
			break
		}
		j := strings.Index(out[i:], "}}")
		if j < 0 {
			break
		}
		expr := strings.TrimSpace(out[i+2 : i+j])
		repl := ""
		if strings.HasPrefix(expr, "#") || strings.HasPrefix(expr, "/") {
			out = out[:i] + out[i+j+2:]
			continue
		}
		if isVarsReposRef(expr) {
			if _, ok := c.vars["repos"]; !ok {
				return strictInterpResult{ok: false, err: "变量不存在: vars.repos"}
			}
			usedRepos = true
			repl = reposListSentinel
		} else if name, ok := simpleVarsRef(expr); ok {
			if _, exists := c.vars[name]; !exists {
				return strictInterpResult{ok: false, err: "变量不存在: vars." + name}
			}
			if v, err := evalExpr(expr, ec); err == nil && v != nil {
				repl = models.VarDisplayText(v)
			}
		} else if v, err := evalExpr(expr, ec); err == nil && v != nil {
			repl = models.VarDisplayText(v)
		}
		out = out[:i] + repl + out[i+j+2:]
	}
	return strictInterpResult{ok: true, value: out, usedRepos: usedRepos}
}

func isVarsReposRef(expr string) bool {
	return expr == "vars.repos"
}

func simpleVarsRef(expr string) (name string, ok bool) {
	if !strings.HasPrefix(expr, "vars.") {
		return "", false
	}
	rest := expr[len("vars."):]
	if rest == "" || strings.Contains(rest, ".") {
		return "", false
	}
	for _, r := range rest {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_') {
			return "", false
		}
	}
	return rest, true
}

// submitMRRepoMode is the three-way config.repo interpretation.
type submitMRRepoMode string

const (
	submitMRModeBlank  submitMRRepoMode = "blank"
	submitMRModeSingle submitMRRepoMode = "single"
	submitMRModeList   submitMRRepoMode = "multi"
)

func detectSubmitMRRepoMode(raw string, r strictInterpResult) submitMRRepoMode {
	if r.emptyField || strings.TrimSpace(raw) == "" {
		return submitMRModeBlank
	}
	if r.usedRepos || r.value == reposListSentinel || rawRefsVarsRepos(raw) {
		return submitMRModeList
	}
	return submitMRModeSingle
}

func rawRefsVarsRepos(raw string) bool {
	for _, m := range varsRefRE.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 && m[1] == "repos" {
			return true
		}
	}
	return false
}

// resolveStrictBranchField applies strict interpolation to a branch field.
// Empty raw stays empty (provider mrBranches / per-repo defaults). Non-empty
// must expand to a non-empty, non-list single branch string.
func (e *Engine) resolveStrictBranchField(c *execCtx, raw, fieldLabel string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if rawRefsVarsBranches(raw) {
		return "", fmt.Errorf("%s 插值非法:不要将 {{vars.branches}} 整表用作分支名，请改用单分支字符串变量或留空走各仓默认", fieldLabel)
	}
	r := e.strictInterpolate(c, raw)
	if !r.ok {
		return "", fmt.Errorf("%s: %s", fieldLabel, r.err)
	}
	if r.usedRepos || r.value == reposListSentinel {
		return "", fmt.Errorf("%s 插值非法:不要将 {{vars.repos}} 用作分支名", fieldLabel)
	}
	val := strings.TrimSpace(r.value)
	if val == "" {
		return "", fmt.Errorf("%s:原配置非空但插值结果为空", fieldLabel)
	}
	if looksLikeJSONObjectOrMap(val) {
		return "", fmt.Errorf("%s 插值非法:结果疑似 JSON/整表（勿填 {{vars.branches}}），请改用单分支字符串变量或留空走各仓默认", fieldLabel)
	}
	return val, nil
}

func rawRefsVarsBranches(raw string) bool {
	for _, m := range varsRefRE.FindAllStringSubmatch(raw, -1) {
		if len(m) >= 2 && m[1] == "branches" {
			return true
		}
	}
	return false
}

// looksLikeJSONObjectOrMap detects map/JSON-object dumps that must not be used
// as a git branch name (e.g. VarDisplayText of vars.branches).
func looksLikeJSONObjectOrMap(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	if (strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}")) ||
		(strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]")) {
		return true
	}
	// fmt.Sprint(map) style: map[key:value ...]
	return strings.HasPrefix(s, "map[")
}

func repoNameInVars(name string, vars map[string]any) bool {
	name = strings.TrimSpace(name)
	for _, n := range runtime.RepoNamesFromVars(vars) {
		if n == name {
			return true
		}
	}
	return false
}
