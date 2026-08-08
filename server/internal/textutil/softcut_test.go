package textutil

import (
	"strings"
	"testing"
)

func TestSoftTruncateRunesNeverCutsALatinWordInHalf(t *testing.T) {
	// The shape that shipped: a 24-rune budget landing inside "worker".
	const in = "调研 Approving 最近关于快模型和 worker 架构的精简空间"
	for limit := 8; limit < len([]rune(in)); limit++ {
		got := SoftTruncateRunes(in, limit)
		if !strings.HasSuffix(got, "…") {
			t.Fatalf("limit=%d: missing ellipsis: %q", limit, got)
		}
		stem := strings.TrimSuffix(got, "…")
		if !strings.HasPrefix(in, stem) {
			t.Fatalf("limit=%d: %q is not a prefix of the input", limit, got)
		}
		// A cut is mid-word when the input continues the same Latin token.
		next := []rune(in)[len([]rune(stem)):]
		last := []rune(stem)
		if len(next) > 0 && len(last) > 0 && isTokenChar(last[len(last)-1]) && isTokenChar(next[0]) {
			t.Fatalf("limit=%d: cut mid-token: %q", limit, got)
		}
	}
}

func TestSoftTruncateRunesKeepsShortTextIntact(t *testing.T) {
	const in = "已经推到 GitHub"
	if got := SoftTruncateRunes(in, 40); got != in {
		t.Fatalf("short text changed: %q", got)
	}
	if strings.Contains(SoftTruncateRunes(in, 0), "…") {
		t.Fatalf("limit 0 should not truncate")
	}
}

func TestSoftTruncateRunesCutsChineseAtAnyCharacter(t *testing.T) {
	const in = "两个检查还在跑，其余全部通过，等它们跑完就可以合并"
	got := SoftTruncateRunes(in, 10)
	if got != "两个检查还在跑，其余…" {
		t.Fatalf("got %q", got)
	}
}

func TestSoftTruncateRunesDropsDanglingPunctuation(t *testing.T) {
	got := SoftTruncateRunes("结论：两个检查还在跑", 3)
	if strings.Contains(got, "：…") {
		t.Fatalf("ellipsis attached to punctuation: %q", got)
	}
}

// Chinese is often written straight up against a Latin word with no space
// between them. The character before the word is still a boundary, so the cut
// lands there rather than inside the word.
func TestSoftTruncateRunesWalksBackToTheChineseBeforeAWord(t *testing.T) {
	if got := SoftTruncateRunes("调研快模型和worker架构", 9); got != "调研快模型和…" {
		t.Fatalf("got %q, want %q", got, "调研快模型和…")
	}
}

// Walking back is worth any number of characters, but not the whole string.
// When the budget falls inside a token that has no boundary behind it, the cut
// stays where it was: a shortened path still tells the reader more than an
// ellipsis on its own.
func TestSoftTruncateRunesCutsInPlaceWhenThereIsNoBoundaryToWalkBackTo(t *testing.T) {
	cases := map[string]string{
		"averylongunbrokentoken":  "averylon…", // no separator anywhere
		"/averylongunbrokentoken": "/averylo…", // only a leading one, nothing before it
	}
	for in, want := range cases {
		if got := SoftTruncateRunes(in, 8); got != want {
			t.Errorf("SoftTruncateRunes(%q, 8) = %q, want %q", in, got, want)
		}
	}
}

// splitsLatinToken is asked about a cut point, and a cut at either end of the
// text is not inside anything. Its caller screens those out before asking, and
// the guard is what keeps that from being load-bearing — indexing r[at] at the
// end would panic.
func TestSplitsLatinTokenSaysNoAtTheEdges(t *testing.T) {
	r := []rune("worker")
	for _, at := range []int{-1, 0, len(r), len(r) + 1} {
		if splitsLatinToken(r, at) {
			t.Errorf("splitsLatinToken(%q, %d) = true, want false", string(r), at)
		}
	}
	if !splitsLatinToken(r, 3) {
		t.Errorf("splitsLatinToken(%q, 3) = false, want true", string(r))
	}
}

func TestHealBrokenTailRepairsHardCutDebris(t *testing.T) {
	cases := map[string]string{
		"调研快模型和 wo":  "调研快模型和…",
		"调研快模型和 wo…": "调研快模型和…",
		"精简分析 arc":   "精简分析…",
	}
	for in, want := range cases {
		if got := HealBrokenTail(in); got != want {
			t.Errorf("HealBrokenTail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHealBrokenTailLeavesRealSentencesAlone(t *testing.T) {
	intact := []string{
		"两个检查还在跑，其余全部通过。",
		"已经合并到 main",       // a real word, not debris
		"等着看 CI",           // acronym
		"改完了 PR",           // acronym
		"这次用的是 fix",        // known short token
		"analysis is ok",   // no Chinese context
		"调研快模型和 worker",    // long enough to be a word
		"首屏从 3.2s 降到 1.1s", // a unit, attached to the number
		"超时阈值调到 30m",       // same
	}
	for _, in := range intact {
		if got := HealBrokenTail(in); got != in {
			t.Errorf("HealBrokenTail(%q) = %q, want unchanged", in, got)
		}
	}
}

// Healing runs over persisted titles, some of which are empty or were stored as
// nothing but spacing. Handing back an ellipsis for those would invent content.
func TestHealBrokenTailLeavesEmptyTextAlone(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t"} {
		if got := HealBrokenTail(in); got != in {
			t.Errorf("HealBrokenTail(%q) = %q, want unchanged", in, got)
		}
	}
	if HasBrokenLatinStub("") {
		t.Error("empty text has no stub to heal")
	}
}

// The stub test only fires inside Chinese text. English prose ends in short
// words constantly, and none of them are the debris of a cut — including the
// ones no dictionary of ours happens to list.
func TestHasBrokenLatinStubStaysOutOfEnglishProse(t *testing.T) {
	if HasBrokenLatinStub("rolled the canary back to hk") {
		t.Error("a short word ending an English sentence is not truncation debris")
	}
}
