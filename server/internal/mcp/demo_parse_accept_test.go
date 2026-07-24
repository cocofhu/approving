package mcp

import "testing"

func TestDemoScenes_ParseQuestionsRecommendedCardinality(t *testing.T) {
	t.Run("multi_keeps_all_recommended", func(t *testing.T) {
		qs := parseQuestions([]any{
			map[string]any{
				"prompt":        "需要同步改哪些触点？",
				"allowMultiple": true,
				"options": []any{
					map[string]any{"id": "a", "label": "后端选答 API", "recommended": true},
					map[string]any{"id": "b", "label": "MCP 解析约束", "recommended": true},
					map[string]any{"id": "c", "label": "ClarifyChat 预选", "recommended": false},
					map[string]any{"id": "d", "label": "无关 UI 改版"},
				},
			},
		})
		if len(qs) != 1 || !qs[0].AllowMultiple {
			t.Fatalf("shape %+v", qs)
		}
		rec := 0
		for _, o := range qs[0].Options {
			if o.Recommended {
				rec++
			}
		}
		if rec != 2 {
			t.Fatalf("expected 2 recommended kept, got %d options=%+v", rec, qs[0].Options)
		}
	})
	t.Run("single_clamps_to_one", func(t *testing.T) {
		qs := parseQuestions([]any{
			map[string]any{
				"prompt": "无推荐时的平台策略？",
				"options": []any{
					map[string]any{"label": "回退第一个选项", "recommended": true},
					map[string]any{"label": "硬驳回提问", "recommended": true},
					map[string]any{"label": "卡住等待人工"},
				},
			},
		})
		if len(qs) != 1 {
			t.Fatalf("shape %+v", qs)
		}
		rec := 0
		for _, o := range qs[0].Options {
			if o.Recommended {
				rec++
			}
		}
		if rec != 1 || !qs[0].Options[0].Recommended || qs[0].Options[1].Recommended {
			t.Fatalf("expected first-only recommended, got %+v", qs[0].Options)
		}
	})
	t.Run("multi_zero_recommended_not_rejected", func(t *testing.T) {
		qs := parseQuestions([]any{
			map[string]any{
				"prompt":        "默认验收环境选哪个？",
				"allowMultiple": true,
				"options": []any{
					map[string]any{"label": "CI 单测"},
					map[string]any{"label": "本地联调"},
				},
			},
		})
		if len(qs) != 1 || len(qs[0].Options) != 2 {
			t.Fatalf("0-rec multi should parse successfully, got %+v", qs)
		}
		for _, o := range qs[0].Options {
			if o.Recommended {
				t.Fatalf("unexpected recommended: %+v", qs[0].Options)
			}
		}
	})
}
