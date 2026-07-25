package services

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cocofhu/approving/internal/models"
)

const AgentExportSchemaVersion = 1

// agentExportJSON is the root agent.json inside an export ZIP.
type agentExportJSON struct {
	Name              string               `json:"name"`
	GitCredentialType string               `json:"gitCredentialType,omitempty"`
	MCP               []MCPServer          `json:"mcp,omitempty"`
	Env               map[string]string    `json:"env,omitempty"`
	Layout            *AgentLayout         `json:"layout,omitempty"`
	Prompts           *models.AgentPrompts `json:"prompts,omitempty"`
	SchemaVersion     int                  `json:"schemaVersion"`
	ExportedAt        string               `json:"exportedAt"`
}

// ExportZIP builds a portable ZIP for one agent from on-disk state.
func (s *SkillService) ExportZIP(name string) ([]byte, error) {
	a, ok := s.Get(name)
	if !ok {
		return nil, fmt.Errorf("agent %q not found", name)
	}
	layout := a.Layout.withDefaults()
	export := agentExportJSON{
		Name:              name,
		GitCredentialType: a.GitCredentialType,
		MCP:               a.MCP,
		Env:               a.Env,
		Layout:            &layout,
		Prompts:           a.Prompts,
		SchemaVersion:     AgentExportSchemaVersion,
		ExportedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	meta, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	// Store agent.json uncompressed so browser peek can read it without
	// DecompressionStream (which hangs on Go deflate-raw in Chromium).
	metaHdr := &zip.FileHeader{Name: "agent.json", Method: zip.Store}
	w, err := zw.CreateHeader(metaHdr)
	if err != nil {
		return nil, err
	}
	if _, err := w.Write(meta); err != nil {
		return nil, err
	}

	work := filepath.Join(s.root, sanitize(name), WorkDirName)
	if fi, err := os.Stat(work); err == nil && fi.IsDir() {
		err = filepath.WalkDir(work, func(p string, d os.DirEntry, walkErr error) error {
			if walkErr != nil || d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(work, p)
			if err != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if rel == "" || strings.Contains(rel, "..") {
				return nil
			}
			body, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			fw, err := zw.Create(rel)
			if err != nil {
				return err
			}
			_, err = fw.Write(body)
			return err
		})
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
	}

	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ImportZIPMode selects create vs overwrite semantics.
type ImportZIPMode string

const (
	ImportZIPCreate    ImportZIPMode = "create"
	ImportZIPOverwrite ImportZIPMode = "overwrite"
)

// ImportZIP parses a ZIP export and writes the agent to disk.
func (s *SkillService) ImportZIP(raw []byte, targetName string, mode ImportZIPMode) (Agent, error) {
	targetName = strings.TrimSpace(targetName)
	if targetName == "" {
		return Agent{}, fmt.Errorf("target name is required")
	}
	switch mode {
	case ImportZIPCreate, ImportZIPOverwrite:
	default:
		return Agent{}, fmt.Errorf("invalid import mode %q", mode)
	}
	exists := s.Exists(targetName)
	if mode == ImportZIPCreate && exists {
		return Agent{}, fmt.Errorf("agent %q already exists", targetName)
	}
	if mode == ImportZIPOverwrite && !exists {
		return Agent{}, fmt.Errorf("agent %q not found", targetName)
	}

	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Agent{}, fmt.Errorf("ZIP 格式非法：%v", err)
	}

	safeName := sanitize(targetName)
	if safeName == "" {
		return Agent{}, fmt.Errorf("invalid agent name")
	}
	workRoot := filepath.Join(s.root, safeName, WorkDirName)

	var export agentExportJSON
	var files []AgentFile
	foundMeta := false

	for _, f := range zr.File {
		name := filepath.ToSlash(f.Name)
		name = strings.TrimPrefix(name, "./")
		if strings.HasSuffix(name, "/") {
			continue
		}
		if name == "agent.json" {
			if foundMeta {
				return Agent{}, fmt.Errorf("ZIP 包含多个 agent.json")
			}
			rc, err := f.Open()
			if err != nil {
				return Agent{}, fmt.Errorf("无法读取 agent.json：%v", err)
			}
			b, err := io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return Agent{}, fmt.Errorf("无法读取 agent.json：%v", err)
			}
			if err := json.Unmarshal(b, &export); err != nil {
				return Agent{}, fmt.Errorf("agent.json 格式无效：%v", err)
			}
			if export.SchemaVersion != AgentExportSchemaVersion {
				return Agent{}, fmt.Errorf("不支持的 schemaVersion：%d。当前仅支持 schemaVersion=%d。", export.SchemaVersion, AgentExportSchemaVersion)
			}
			foundMeta = true
			continue
		}

		rel := safeRel(name)
		if rel == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || filepath.IsAbs(name) {
			return Agent{}, fmt.Errorf("ZIP 包含非法路径：%s", f.Name)
		}
		// Zip Slip under-root barrier (CodeQL #19): reject before accepting content.
		if _, err := underRoot(workRoot, rel); err != nil {
			return Agent{}, fmt.Errorf("ZIP 包含非法路径：%s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return Agent{}, fmt.Errorf("无法读取 %s：%v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return Agent{}, fmt.Errorf("无法读取 %s：%v", f.Name, err)
		}
		files = append(files, AgentFile{Path: rel, Content: string(body)})
	}

	if !foundMeta {
		return Agent{}, fmt.Errorf("ZIP 缺少根目录 agent.json")
	}

	var layout AgentLayout
	if export.Layout != nil {
		layout = export.Layout.withDefaults()
	} else {
		layout = AgentLayout{}.withDefaults()
	}

	// Home project is environment-specific and never travels in the ZIP.
	// Overwrite keeps the existing binding; create stays unbound.
	projectID := ""
	if mode == ImportZIPOverwrite {
		if prev, ok := s.Get(targetName); ok {
			projectID = strings.TrimSpace(prev.ProjectID)
		}
	}
	agent := Agent{
		Name:              targetName,
		ProjectID:         projectID,
		GitCredentialType: export.GitCredentialType,
		Files:             files,
		MCP:               export.MCP,
		Env:               export.Env,
		Layout:            layout,
		Prompts:           export.Prompts,
	}
	if mode == ImportZIPCreate && len(agent.MCP) == 0 {
		agent.MCP = DefaultPlatformMCP()
	}
	if projectID == "" {
		agent.MCP = StripProjectPlatformMCP(agent.MCP)
	}
	if err := s.Save(agent); err != nil {
		return Agent{}, err
	}
	out, ok := s.Get(targetName)
	if !ok {
		return Agent{}, fmt.Errorf("import succeeded but agent %q missing", targetName)
	}
	return out, nil
}
