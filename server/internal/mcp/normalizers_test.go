package mcp

import "testing"

func TestPlanNormalizers(t *testing.T) {
	if normPlanStatus("done") != "done" || normPlanStatus("garbage") != planStatusPending {
		t.Error("normPlanStatus wrong")
	}
}

func TestAsBool(t *testing.T) {
	if !asBool(true) || !asBool("true") || !asBool("1") || asBool("no") || asBool(3) {
		t.Error("asBool wrong")
	}
}
