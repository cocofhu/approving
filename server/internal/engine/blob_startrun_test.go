package engine

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/models"
)

func TestStartRunExternalizesCompositeImages(t *testing.T) {
	g := models.Graph{
		Variables: []models.Variable{{Name: "feature", Type: "string", Ask: true}},
		Nodes: []models.Node{
			{ID: "input", Type: "input"},
			{ID: "output", Type: "output"},
		},
		Edges: []models.Edge{{ID: "e1", Source: "input", Target: "output"}},
	}
	eng, db := setupEngineGraph(t, g)
	raw := base64.StdEncoding.EncodeToString([]byte("hello-bytes"))
	run, err := eng.StartRun("wf", map[string]any{
		"feature": map[string]any{
			"text": "hi",
			"images": []any{
				map[string]any{"data": raw, "mimeType": "image/png", "name": "a.png"},
			},
		},
	}, "test")
	if err != nil {
		t.Fatalf("start: %v", err)
	}

	var stored models.Run
	if err := db.First(&stored, "id = ?", run.ID).Error; err != nil {
		t.Fatal(err)
	}
	inJSON, _ := json.Marshal(stored.Inputs)
	if strings.Contains(string(inJSON), `"data"`) && strings.Contains(string(inJSON), raw) {
		t.Fatalf("runs.inputs still contains base64 data: %s", inJSON)
	}
	ct := models.AsCompositeText(stored.Inputs["feature"])
	if ct == nil || len(ct.Images) != 1 || ct.Images[0].Ref == "" || ct.Images[0].Data != "" {
		t.Fatalf("inputs feature = %#v", stored.Inputs["feature"])
	}
	if !strings.HasPrefix(ct.Images[0].Ref, "blob:") {
		t.Fatalf("ref = %q", ct.Images[0].Ref)
	}

	var rv models.RunVariable
	if err := db.Where("run_id = ? AND name = ?", run.ID, "feature").First(&rv).Error; err != nil {
		t.Fatal(err)
	}
	vct := models.AsCompositeText(rv.Value)
	if vct == nil || len(vct.Images) != 1 || vct.Images[0].Ref == "" || vct.Images[0].Data != "" {
		t.Fatalf("run_variables.feature = %#v", rv.Value)
	}
}
