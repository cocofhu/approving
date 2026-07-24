package engine

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cocofhu/approving/internal/models"
	"github.com/cocofhu/approving/internal/nodereg"
)

const outputCompleteMd = "工作流完成。"

var (
	nodeOutRefRE  = regexp.MustCompile(`^\{\{nodes\.([^.]+)\.outputs\.([^}]+)\}\}$`)
	artifactRefRE = regexp.MustCompile(`^\{\{artifact\("([^"]+)"\)\}\}$`)
)

type outputSourceRef struct {
	kind      string // node | artifact | unknown
	nodeID    string
	outputKey string
	artifact  string
	raw       string
}

func parseOutputSourceRef(tmpl string) outputSourceRef {
	t := strings.TrimSpace(tmpl)
	if m := nodeOutRefRE.FindStringSubmatch(t); len(m) == 3 {
		return outputSourceRef{kind: "node", nodeID: m[1], outputKey: m[2], raw: t}
	}
	if m := artifactRefRE.FindStringSubmatch(t); len(m) == 2 {
		return outputSourceRef{kind: "artifact", artifact: m[1], raw: t}
	}
	return outputSourceRef{kind: "unknown", raw: t}
}

func resolveOutputResults(cfg map[string]any) []string {
	if raw, ok := cfg["results"].([]any); ok && len(raw) > 0 {
		out := make([]string, 0, len(raw))
		for _, item := range raw {
			if s := strings.TrimSpace(fmt.Sprint(item)); s != "" {
				out = append(out, s)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if s := strings.TrimSpace(str(cfg["result"])); s != "" {
		return []string{s}
	}
	return nil
}

func (e *Engine) nodeLatestStatus(c *execCtx, nodeID string) string {
	var sr models.StateRun
	err := e.db.Where("run_id = ? AND node_id = ?", c.run.ID, nodeID).
		Order("iteration desc, id desc").First(&sr).Error
	if err != nil {
		return ""
	}
	return sr.Status
}

func structuredArtifactForKey(key string) string {
	m := nodereg.BuildManifest()
	if name, ok := m.OutputKeyToArtifact[key]; ok {
		return name
	}
	return ""
}

func cardTitleForNodeRef(c *execCtx, ref outputSourceRef) string {
	label := eNodeLabel(c, ref.nodeID)
	key := ref.outputKey
	switch key {
	case "plan":
		return "计划 · " + label
	case "clarified_requirement":
		return "需求澄清 · " + label
	case "research":
		return "调研 · " + label
	case "proposals":
		return "候选方案 · " + label
	case "proposal":
		return "已选方案 · " + label
	case "test_result":
		return "测试结果 · " + label
	case "review":
		return "评审结论 · " + label
	case "implementation_result":
		return "实现结果 · " + label
	case "page":
		return "网页预览 · " + label
	case "content":
		return label + " · 文本输出"
	default:
		return label + " · " + key
	}
}

func eNodeLabel(c *execCtx, nodeID string) string {
	for _, n := range c.graph.Nodes {
		if n.ID == nodeID {
			if lbl := strings.TrimSpace(n.Label); lbl != "" {
				return lbl
			}
		}
	}
	return nodeID
}

func (e *Engine) buildOutputCard(c *execCtx, idx int, tmpl string) map[string]any {
	ref := parseOutputSourceRef(tmpl)
	card := map[string]any{
		"index":    idx,
		"template": tmpl,
		"status":   "ok",
	}

	switch ref.kind {
	case "node":
		card["nodeId"] = ref.nodeID
		card["outputKey"] = ref.outputKey
		card["title"] = cardTitleForNodeRef(c, ref)
		st := e.nodeLatestStatus(c, ref.nodeID)
		if st == "failed" || st == "skipped" || st == "cancelled" {
			card["typeTag"] = "来源失败"
			card["status"] = "failed"
			card["errorReason"] = fmt.Sprintf("上游节点状态：%s", st)
			return card
		}
		outs := c.nodeOutputs[ref.nodeID]
		if outs == nil {
			card["typeTag"] = "来源失败"
			card["status"] = "failed"
			card["errorReason"] = "上游节点无输出"
			return card
		}
		val, ok := outs[ref.outputKey]
		if !ok {
			card["typeTag"] = "来源失败"
			card["status"] = "failed"
			card["errorReason"] = fmt.Sprintf("上游输出键 %q 不存在", ref.outputKey)
			return card
		}
		if artName := structuredArtifactForKey(ref.outputKey); artName != "" {
			card["typeTag"] = "结构化产物"
			card["structuredArtifactName"] = artName
			m := nodereg.BuildManifest()
			if jsonKey, ok := m.ArtifactToOutputJSON[artName]; ok {
				if snap, ok := outs[jsonKey]; ok {
					if s, ok := snap.(string); ok && strings.TrimSpace(s) != "" {
						card["jsonSnapshot"] = s
					} else if b, err := json.Marshal(snap); err == nil {
						card["jsonSnapshot"] = string(b)
					}
				}
			}
			if md, ok := val.(string); ok && strings.TrimSpace(md) != "" {
				card["markdown"] = md
			}
			if card["jsonSnapshot"] == nil && (card["markdown"] == nil || card["markdown"] == "") {
				card["typeTag"] = "来源失败"
				card["status"] = "failed"
				card["errorReason"] = "结构化产物缺失或为空"
			}
			return card
		}
		if ref.outputKey == "content" || ref.outputKey == "page" {
			md := models.VarDisplayText(val)
			if strings.TrimSpace(md) == "" {
				card["typeTag"] = "来源失败"
				card["status"] = "failed"
				card["errorReason"] = "文本输出为空"
				return card
			}
			if ref.outputKey == "page" {
				card["typeTag"] = "自定义产物"
				card["artifactName"] = "page.html"
				card["markdown"] = md
			} else {
				card["typeTag"] = "Markdown"
				card["markdown"] = md
			}
			return card
		}
		md := models.VarDisplayText(val)
		if strings.TrimSpace(md) == "" {
			card["typeTag"] = "来源失败"
			card["status"] = "failed"
			card["errorReason"] = "来源内容为空"
			return card
		}
		card["typeTag"] = "Markdown"
		card["markdown"] = md
		return card

	case "artifact":
		card["title"] = "产物 · " + ref.artifact
		card["artifactName"] = ref.artifact
		content, ok := e.store.Get(c.run.ID, ref.artifact)
		if !ok || strings.TrimSpace(content) == "" {
			card["typeTag"] = "来源失败"
			card["status"] = "failed"
			card["errorReason"] = fmt.Sprintf("产物 %q 不存在或为空", ref.artifact)
			return card
		}
		if isKnownStructuredArtifact(ref.artifact) {
			card["typeTag"] = "结构化产物"
			card["structuredArtifactName"] = ref.artifact
			card["jsonSnapshot"] = content
			return card
		}
		card["typeTag"] = "自定义产物"
		return card

	default:
		card["title"] = "自定义 · " + tmpl
		card["typeTag"] = "来源失败"
		card["status"] = "failed"
		card["errorReason"] = "无法解析的来源模板"
		return card
	}
}

func isKnownStructuredArtifact(name string) bool {
	m := nodereg.BuildManifest()
	_, ok := m.ArtifactToOutputJSON[name]
	return ok
}

func (e *Engine) execOutput(c *execCtx, node *models.Node) nodeOutcome {
	templates := resolveOutputResults(node.Config)
	cards := make([]map[string]any, 0, len(templates))
	for i, tmpl := range templates {
		cards = append(cards, e.buildOutputCard(c, i+1, tmpl))
	}
	outputs := map[string]any{
		"outputCards": cards,
		"results":     templates,
	}
	return nodeOutcome{status: "completed", outputMd: outputCompleteMd, outputs: outputs}
}
