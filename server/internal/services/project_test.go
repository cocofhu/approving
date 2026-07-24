package services

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"
)

func TestProjectCRUDAndDeleteConstraint(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "proj.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)

	p, err := s.Create("Alpha", "desc", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.Name != "Alpha" || p.Description != "desc" {
		t.Fatalf("create = %+v", p)
	}
	if _, err := s.Create("Alpha", "", nil, nil); err != ErrProjectNameExists {
		t.Fatalf("dup name: %v", err)
	}

	name := "Alpha2"
	desc := "d2"
	p, err = s.Update(p.ID, &name, &desc, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Alpha2" || p.Description != "d2" {
		t.Fatalf("update = %+v", p)
	}

	if err := db.Create(&models.WorkflowDef{
		ID: "wf-1", ProjectID: p.ID, Name: "w", Status: "draft", Version: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(p.ID); err != ErrProjectHasWorkflows {
		t.Fatalf("delete with wf: %v", err)
	}
	if err := db.Delete(&models.WorkflowDef{}, "id = ?", "wf-1").Error; err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(p.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get(p.ID); ok {
		t.Fatal("expected deleted")
	}
}

func TestProjectSecretMergeAndMask(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "secret.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	p, err := s.Create("S", "", []models.EnvEntry{
		{Key: "TOKEN", Value: "plain-secret", Secret: true},
		{Key: "PUBLIC", Value: "visible", Secret: false},
	}, []models.ProjectVariable{
		{Name: "api_key", Type: "string", Value: "sk-123", Secret: true},
		{Name: "region", Type: "string", Value: "cn", Secret: false},
	})
	if err != nil {
		t.Fatal(err)
	}

	maskedEnv := MaskedSandboxEnv(p.SandboxEnv)
	if maskedEnv[0].Value != SecretMask || maskedEnv[1].Value != "visible" {
		t.Fatalf("mask env = %+v", maskedEnv)
	}
	maskedVars := MaskedProjectVars(p.Variables)
	if maskedVars[0].Value != SecretMask || maskedVars[1].Value != "cn" {
		t.Fatalf("mask vars = %+v", maskedVars)
	}

	// Empty/mask keeps plaintext; new value overwrites; delete key by omission; toggle secret.
	env := []models.EnvEntry{
		{Key: "TOKEN", Value: SecretMask, Secret: true},
		{Key: "PUBLIC", Value: "visible2", Secret: false},
		{Key: "NEW", Value: "n", Secret: false},
	}
	vars := []models.ProjectVariable{
		{Name: "api_key", Type: "string", Value: "", Secret: true},
		{Name: "region", Type: "string", Value: "us", Secret: true}, // become secret with new value
	}
	p, err = s.Update(p.ID, nil, nil, &env, &vars)
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["TOKEN"]; got != "plain-secret" {
		t.Fatalf("TOKEN preserved = %q", got)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["PUBLIC"]; got != "visible2" {
		t.Fatalf("PUBLIC = %q", got)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["NEW"]; got != "n" {
		t.Fatalf("NEW = %q", got)
	}
	var apiKey, region models.ProjectVariable
	for _, v := range p.Variables {
		switch v.Name {
		case "api_key":
			apiKey = v
		case "region":
			region = v
		}
	}
	if apiKey.Value != "sk-123" {
		t.Fatalf("api_key preserved = %v", apiKey.Value)
	}
	if region.Value != "us" || !region.Secret {
		t.Fatalf("region = %+v", region)
	}

	// Explicit new secret value.
	env2 := []models.EnvEntry{{Key: "TOKEN", Value: "rotated", Secret: true}}
	p, err = s.Update(p.ID, nil, nil, &env2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["TOKEN"]; got != "rotated" {
		t.Fatalf("TOKEN rotated = %q", got)
	}
}

func TestProjectSecretTogglePreservesPlaintext(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "toggle.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	p, err := s.Create("T", "", []models.EnvEntry{
		{Key: "TOKEN", Value: "real-secret", Secret: true},
	}, []models.ProjectVariable{
		{Name: "api_key", Type: "string", Value: "sk-real", Secret: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// secret → non-secret with masked value must keep plaintext (not store ****).
	env := []models.EnvEntry{{Key: "TOKEN", Value: SecretMask, Secret: false}}
	vars := []models.ProjectVariable{{Name: "api_key", Type: "string", Value: SecretMask, Secret: false}}
	p, err = s.Update(p.ID, nil, nil, &env, &vars)
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["TOKEN"]; got != "real-secret" {
		t.Fatalf("TOKEN after un-secret = %q", got)
	}
	tokSecret := false
	for _, e := range p.SandboxEnv {
		if e.Key == "TOKEN" {
			tokSecret = e.Secret
		}
	}
	if tokSecret {
		t.Fatal("TOKEN should no longer be secret")
	}
	var apiKey models.ProjectVariable
	for _, v := range p.Variables {
		if v.Name == "api_key" {
			apiKey = v
		}
	}
	if apiKey.Value != "sk-real" || apiKey.Secret {
		t.Fatalf("api_key after un-secret = %+v", apiKey)
	}

	// non-secret → secret with mask/empty keeps plaintext and flips flag.
	env2 := []models.EnvEntry{{Key: "TOKEN", Value: "", Secret: true}}
	vars2 := []models.ProjectVariable{{Name: "api_key", Type: "string", Value: SecretMask, Secret: true}}
	p, err = s.Update(p.ID, nil, nil, &env2, &vars2)
	if err != nil {
		t.Fatal(err)
	}
	if got := ProjectEnvMap(p.SandboxEnv)["TOKEN"]; got != "real-secret" {
		t.Fatalf("TOKEN after re-secret = %q", got)
	}
	for _, e := range p.SandboxEnv {
		if e.Key == "TOKEN" && !e.Secret {
			t.Fatal("TOKEN should be secret again")
		}
	}
	for _, v := range p.Variables {
		if v.Name == "api_key" && (v.Value != "sk-real" || !v.Secret) {
			t.Fatalf("api_key after re-secret = %+v", v)
		}
	}
}

func TestProjectRejectsPlatformAuthEnvKey(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "authenv.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	_, err = s.Create("A", "", []models.EnvEntry{
		{Key: "CURSOR_API_KEY", Value: "x", Secret: true},
	}, nil)
	if !errors.Is(err, ErrPlatformAuthEnvKey) {
		t.Fatalf("create auth key: %v", err)
	}
	p, err := s.Create("A", "", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	env := []models.EnvEntry{{Key: "ANTHROPIC_API_KEY", Value: "y", Secret: true}}
	if _, err := s.Update(p.ID, nil, nil, &env, nil); !errors.Is(err, ErrPlatformAuthEnvKey) {
		t.Fatalf("update auth key: %v", err)
	}
}

func TestProjectRejectsMaskOnRenamedKey(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "rename.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	p, err := s.Create("R", "", []models.EnvEntry{
		{Key: "TOKEN", Value: "real", Secret: true},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	env := []models.EnvEntry{{Key: "TOKEN2", Value: SecretMask, Secret: true}}
	if _, err := s.Update(p.ID, nil, nil, &env, nil); !errors.Is(err, ErrSecretPlaceholderOnNewKey) {
		t.Fatalf("rename with mask: %v", err)
	}
}

func TestProjectRejectsMaskOnCreateVars(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "create-mask.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)
	_, err = s.Create("C", "", nil, []models.ProjectVariable{
		{Name: "api_key", Type: "string", Value: SecretMask, Secret: true},
	})
	if !errors.Is(err, ErrSecretPlaceholderOnNewKey) {
		t.Fatalf("create vars with mask: %v", err)
	}
	_, err = s.Create("C", "", []models.EnvEntry{
		{Key: "TOKEN", Value: SecretMask, Secret: true},
	}, nil)
	if !errors.Is(err, ErrSecretPlaceholderOnNewKey) {
		t.Fatalf("create env with mask: %v", err)
	}
}

func TestDefaultProjectBackfill(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "bf.db"))
	if err != nil {
		t.Fatal(err)
	}
	var p models.Project
	if err := db.Where("id = ?", models.DefaultProjectID).First(&p).Error; err != nil {
		t.Fatalf("default project missing: %v", err)
	}
	if p.Name != models.DefaultProjectName {
		t.Fatalf("name = %q", p.Name)
	}
}

func TestProjectListAndWorkflowLookups(t *testing.T) {
	db, err := database.OpenSQLiteTest(filepath.Join(t.TempDir(), "proj-list.db"))
	if err != nil {
		t.Fatal(err)
	}
	s := NewProjectService(db)

	list := s.List()
	if len(list) == 0 {
		t.Fatal("expected default project in list")
	}
	if id := s.DefaultProjectID(); id != models.DefaultProjectID {
		t.Fatalf("DefaultProjectID=%q", id)
	}
	if got := FormatProjectHasWorkflowsError(3); !strings.Contains(got, "3") {
		t.Fatalf("format: %q", got)
	}

	p, err := s.Create("LookMe", "", []models.EnvEntry{{Key: "K", Value: "v"}}, []models.ProjectVariable{{Name: "v1", Type: "string", Value: "1"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.WorkflowDef{
		ID: "wf-look", ProjectID: p.ID, Name: "w", Status: "draft", Version: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	env := s.SandboxEnvForWorkflow("wf-look")
	if len(env) != 1 || env[0].Key != "K" {
		t.Fatalf("env=%+v", env)
	}
	vars := s.VariablesForWorkflow("wf-look")
	if len(vars) != 1 || vars[0].Name != "v1" {
		t.Fatalf("vars=%+v", vars)
	}
	if s.SandboxEnvForWorkflow("missing") != nil || s.VariablesForWorkflow("missing") != nil {
		t.Fatal("missing workflow should yield nil")
	}
	// Workflow with empty project_id
	if err := db.Create(&models.WorkflowDef{
		ID: "wf-empty-pid", ProjectID: "", Name: "e", Status: "draft", Version: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if s.SandboxEnvForWorkflow("wf-empty-pid") != nil {
		t.Fatal("empty project_id")
	}
	// Workflow pointing at deleted project
	if err := db.Create(&models.WorkflowDef{
		ID: "wf-orphan", ProjectID: "no-such-proj", Name: "o", Status: "draft", Version: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if s.SandboxEnvForWorkflow("wf-orphan") != nil || s.VariablesForWorkflow("wf-orphan") != nil {
		t.Fatal("orphan project")
	}

	// DefaultProjectID falls back to oldest when default row is gone.
	db.Exec("DELETE FROM projects WHERE id = ?", models.DefaultProjectID)
	if id := s.DefaultProjectID(); id == "" || id == models.DefaultProjectID {
		t.Fatalf("fallback DefaultProjectID=%q", id)
	}
	db.Exec("DELETE FROM projects")
	if id := s.DefaultProjectID(); id != "" {
		t.Fatalf("empty projects DefaultProjectID=%q", id)
	}
}
