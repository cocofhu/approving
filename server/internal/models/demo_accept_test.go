package models

import "testing"

// Demo-aligned acceptance: mirrors approved page.html four scenes + all-recommended edge.
func TestDemoScenes_AutoClarifyMultiRecommended(t *testing.T) {
	t.Run("multi_2_recommended", func(t *testing.T) {
		q := ReactQuestion{
			Prompt: "需要同步改哪些触点？", AllowMultiple: true,
			Options: []ReactOption{
				{ID: "a", Label: "后端选答 API", Recommended: true},
				{ID: "b", Label: "MCP 解析约束", Recommended: true},
				{ID: "c", Label: "ClarifyChat 预选"},
				{ID: "d", Label: "无关 UI 改版"},
			},
		}
		got := FormatChoiceReply([]ReactQuestion{q})
		want := "我的选择:\n- 需要同步改哪些触点？ → 后端选答 API、MCP 解析约束"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("partial_recommended", func(t *testing.T) {
		q := ReactQuestion{
			Prompt: "本迭代文档要同步哪些？", AllowMultiple: true,
			Options: []ReactOption{
				{ID: "a", Label: "MCP schema 说明", Recommended: true},
				{ID: "b", Label: "react.md 规则", Recommended: true},
				{ID: "c", Label: "节点帮助文案", Recommended: true},
				{ID: "d", Label: "营销落地页"},
			},
		}
		got := FormatChoiceReply([]ReactQuestion{q})
		want := "我的选择:\n- 本迭代文档要同步哪些？ → MCP schema 说明、react.md 规则、节点帮助文案"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("none_fallback_first", func(t *testing.T) {
		q := ReactQuestion{
			Prompt: "默认验收环境选哪个？", AllowMultiple: true,
			Options: []ReactOption{
				{ID: "a", Label: "CI 单测"},
				{ID: "b", Label: "本地联调"},
				{ID: "c", Label: "预发回归"},
			},
		}
		got := FormatChoiceReply([]ReactQuestion{q})
		want := "我的选择:\n- 默认验收环境选哪个？ → CI 单测"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("single_multi_rec_picks_first", func(t *testing.T) {
		q := ReactQuestion{
			Prompt: "无推荐时的平台策略？",
			Options: []ReactOption{
				{ID: "a", Label: "回退第一个选项", Recommended: true},
				{ID: "b", Label: "硬驳回提问", Recommended: true},
				{ID: "c", Label: "卡住等待人工"},
			},
		}
		got := FormatChoiceReply([]ReactQuestion{q})
		want := "我的选择:\n- 无推荐时的平台策略？ → 回退第一个选项"
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	})
	t.Run("all_recommended", func(t *testing.T) {
		q := ReactQuestion{
			AllowMultiple: true,
			Options: []ReactOption{
				{ID: "a", Label: "A", Recommended: true},
				{ID: "b", Label: "B", Recommended: true},
				{ID: "c", Label: "C", Recommended: true},
			},
		}
		got, ok := SelectRecommendedOptions(q)
		if !ok || len(got) != 3 {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})
}
