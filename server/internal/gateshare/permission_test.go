package gateshare

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

func TestParseAndNormalizePermissionPreset(t *testing.T) {
	p, ok := ParsePermissionPreset("")
	if !ok || p != models.SharePermissionFull {
		t.Fatalf("empty: %q ok=%v", p, ok)
	}
	p, ok = ParsePermissionPreset("react_only")
	if !ok || p != models.SharePermissionReactOnly {
		t.Fatalf("react_only: %q ok=%v", p, ok)
	}
	if _, ok := ParsePermissionPreset("comment_only"); ok {
		t.Fatal("unknown create preset should fail")
	}
	if NormalizePermissionPreset("") != models.SharePermissionFull {
		t.Fatal("empty normalize")
	}
	if NormalizePermissionPreset("bogus") != models.SharePermissionFull {
		t.Fatal("legacy unknown normalize → full")
	}
}

func TestAllowAndFilterActions(t *testing.T) {
	if !Allow(models.SharePermissionFull, ActionDecide) {
		t.Fatal("full should allow decide")
	}
	if Allow(models.SharePermissionReactOnly, ActionDecide) {
		t.Fatal("react_only must deny decide")
	}
	if !Allow(models.SharePermissionReactOnly, ActionReply) || !Allow(models.SharePermissionReactOnly, ActionCancel) {
		t.Fatal("react_only should allow reply/cancel")
	}
	actions := map[string]string{
		"approve": "approve",
		"confirm": "approve",
		"reject":  "revise",
		"reply":   "reply",
		"cancel":  "cancel",
	}
	filtered := FilterActionsByPreset(actions, models.SharePermissionReactOnly)
	if filtered["approve"] != "" || filtered["confirm"] != "" || filtered["reject"] != "" {
		t.Fatalf("decide keys remain: %+v", filtered)
	}
	if filtered["reply"] != "reply" || filtered["cancel"] != "cancel" {
		t.Fatalf("react keys missing: %+v", filtered)
	}
	full := FilterActionsByPreset(actions, "")
	if full["confirm"] != "approve" {
		t.Fatalf("empty preset should keep decide: %+v", full)
	}
}

func TestBuildPreviewDTOFiltersReactOnly(t *testing.T) {
	gateID := uint(9)
	lookup := &LookupResult{
		Link: models.GateShareLink{
			ID:               "gsl-1",
			PermissionPreset: models.SharePermissionReactOnly,
			ExpiresAt:        time.Now().Add(time.Hour),
			GateID:           &gateID,
		},
		Gate: models.Gate{
			Title: "审",
			Actions: []models.GateAction{
				{ID: "approve", Label: "批准"},
				{ID: "revise", Label: "驳回", RequireForm: true},
			},
		},
	}
	hot := BuildPreviewDTO(models.ShareLinkStateActive, lookup, "", "", "", "n1", PreviewExtras{ReactSessionAlive: true})
	if hot.PermissionPreset != models.SharePermissionReactOnly {
		t.Fatalf("preset=%q", hot.PermissionPreset)
	}
	if hot.Actions["reply"] != "reply" || hot.Actions["cancel"] != "cancel" {
		t.Fatalf("hot react_only actions: %+v", hot.Actions)
	}
	if hot.Actions["confirm"] != "" || hot.Actions["approve"] != "" || hot.Actions["reject"] != "" {
		t.Fatalf("hot react_only still has decide: %+v", hot.Actions)
	}
	cold := BuildPreviewDTO(models.ShareLinkStateActive, lookup, "", "", "", "n2", PreviewExtras{ReactSessionAlive: false})
	if len(cold.Actions) != 0 {
		t.Fatalf("cold react_only must have no actions: %+v", cold.Actions)
	}

	lookup.Link.PermissionPreset = ""
	legacy := BuildPreviewDTO(models.ShareLinkStateActive, lookup, "", "", "", "n3", PreviewExtras{})
	if legacy.PermissionPreset != models.SharePermissionFull {
		t.Fatalf("legacy preset=%q", legacy.PermissionPreset)
	}
	if legacy.Actions["confirm"] == "" || legacy.Actions["reject"] == "" {
		t.Fatalf("legacy full decide missing: %+v", legacy.Actions)
	}
}

func TestBuildReviewPreviewDTOFiltersReactOnly(t *testing.T) {
	lookup := &LookupResult{
		Link: models.GateShareLink{
			ID:               "gsl-r1",
			PermissionPreset: models.SharePermissionReactOnly,
			ExpiresAt:        time.Now().Add(time.Hour),
			Kind:             models.ShareLinkKindReview,
		},
		Kind: models.ShareLinkKindReview,
		Node: &models.Node{ID: "research1", Type: "research", Label: "调研"},
	}
	dto := BuildReviewPreviewDTO(models.ShareLinkStateActive, lookup, "", "", "", "n", PreviewExtras{ReactSessionAlive: true})
	if dto.Actions["confirm"] != "" {
		t.Fatalf("review react_only still has confirm: %+v", dto.Actions)
	}
	if dto.Actions["reply"] != "reply" {
		t.Fatalf("review react_only missing reply: %+v", dto.Actions)
	}
}
