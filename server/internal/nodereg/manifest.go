package nodereg

import "sort"

// ProductEntry describes one structured product mapping for UI / API consumers.
type ProductEntry struct {
	Type         string `json:"type"`
	Label        string `json:"label"`
	Category     string `json:"category"`
	OutputKey    string `json:"outputKey,omitempty"`
	ArtifactName string `json:"artifactName,omitempty"`
	OutputJSON   string `json:"outputJsonKey,omitempty"`
	SetTool      string `json:"setTool,omitempty"`
	RuleFile     string `json:"ruleFile,omitempty"`
	Gate         string `json:"gate,omitempty"`
	// ReviewVar is the control variable that gates this node's post-run ReAct
	// review phase (empty ⇒ not review-capable). See engine.reviewEnabled.
	ReviewVar string `json:"reviewVar,omitempty"`
	// Artifacts lists every reserved deliverable when a node writes more than
	// one (Approve). Single-product types omit this and keep ArtifactName.
	Artifacts []ProductArtifact `json:"artifacts,omitempty"`
}

// ProductArtifact is one reserved deliverable on a multi-product node.
type ProductArtifact struct {
	ArtifactName string `json:"artifactName"`
	OutputKey    string `json:"outputKey"`
	OutputJSON   string `json:"outputJsonKey,omitempty"`
	SetTool      string `json:"setTool,omitempty"`
	Required     bool   `json:"required"`
}

// Manifest is the cross-platform contract summary derived from the registry.
type Manifest struct {
	Products             []ProductEntry    `json:"products"`
	OutputKeyToArtifact  map[string]string `json:"outputKeyToArtifact"`
	ArtifactToOutputJSON map[string]string `json:"artifactToOutputJSON"`
}

// BuildManifest exports the structured-product contract for API / codegen.
func BuildManifest() Manifest {
	m := Manifest{
		OutputKeyToArtifact:  map[string]string{},
		ArtifactToOutputJSON: map[string]string{},
	}
	for _, s := range registry {
		if len(s.Products) > 0 {
			entry := ProductEntry{
				Type:     s.Type,
				Label:    s.Label,
				Category: s.Category,
			}
			if len(s.EmbeddedRules) > 0 {
				entry.RuleFile = s.EmbeddedRules[0]
			}
			entry.ReviewVar = s.ReviewVar
			for _, p := range s.Products {
				art := ProductArtifact{
					ArtifactName: p.ArtifactName,
					OutputKey:    p.OutputKey,
					SetTool:      p.SetTool,
					Required:     p.Required,
				}
				if p.ArtifactName != "" && p.ArtifactName != visualPageName {
					art.OutputJSON = p.OutputKey + "_json"
					if _, exists := m.ArtifactToOutputJSON[p.ArtifactName]; !exists {
						m.ArtifactToOutputJSON[p.ArtifactName] = art.OutputJSON
					}
				}
				entry.Artifacts = append(entry.Artifacts, art)
				if _, exists := m.OutputKeyToArtifact[p.OutputKey]; !exists {
					m.OutputKeyToArtifact[p.OutputKey] = p.ArtifactName
				}
			}
			m.Products = append(m.Products, entry)
			continue
		}
		if s.OutputKey == "" || s.ArtifactName == "" {
			continue
		}
		entry := ProductEntry{
			Type:         s.Type,
			Label:        s.Label,
			Category:     s.Category,
			OutputKey:    s.OutputKey,
			ArtifactName: s.ArtifactName,
			SetTool:      s.SetTool,
		}
		if s.ArtifactName != "" && s.ArtifactName != "page.html" {
			entry.OutputJSON = s.OutputKey + "_json"
			m.ArtifactToOutputJSON[s.ArtifactName] = entry.OutputJSON
		}
		if len(s.EmbeddedRules) > 0 {
			entry.RuleFile = s.EmbeddedRules[0]
		}
		entry.ReviewVar = s.ReviewVar
		switch s.Gate {
		case GateTest:
			entry.Gate = "test"
		case GateReview:
			entry.Gate = "review"
		}
		m.Products = append(m.Products, entry)
		m.OutputKeyToArtifact[s.OutputKey] = s.ArtifactName
	}
	sort.Slice(m.Products, func(i, j int) bool {
		return m.Products[i].Type < m.Products[j].Type
	})
	return m
}
