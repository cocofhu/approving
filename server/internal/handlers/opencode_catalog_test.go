package handlers_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/cocofhu/grasp/internal/opencodecatalog"
)

const catalogBody = `{
  "deepseek": {"id":"deepseek","name":"DeepSeek","api":"https://api.deepseek.com","env":["DEEPSEEK_API_KEY"],
    "models":{"deepseek-v4-pro":{"id":"deepseek-v4-pro","name":"DeepSeek V4 Pro"}}},
  "anthropic": {"id":"anthropic","name":"Anthropic","env":["ANTHROPIC_API_KEY"],
    "models":{"claude-opus-5":{"id":"claude-opus-5","name":"Claude Opus 5"}}}
}`

func catalogHarness(t *testing.T, body string, status int) *harness {
	t.Helper()
	hn := newHarness(t)
	enableAdmin(t)
	hn.cookie = hn.login(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	hn.h.OpenCodeCatalog = opencodecatalog.New(srv.URL)
	return hn
}

func TestListOpenCodeProviders(t *testing.T) {
	hn := catalogHarness(t, catalogBody, http.StatusOK)
	w := hn.do(http.MethodGet, "/api/opencode/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Providers []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			API    string `json:"api"`
			KeyEnv string `json:"keyEnv"`
			Models int    `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Providers) != 2 || got.Providers[0].ID != "anthropic" {
		t.Fatalf("providers: %+v", got.Providers)
	}
	ds := got.Providers[1]
	if ds.ID != "deepseek" || ds.Name != "DeepSeek" || ds.KeyEnv != "DEEPSEEK_API_KEY" || ds.Models != 1 {
		t.Fatalf("provider fields: %+v", ds)
	}
	if ds.API != "https://api.deepseek.com" {
		t.Fatalf("default base url lost: %+v", ds)
	}
}

func TestListOpenCodeProviderModels(t *testing.T) {
	hn := catalogHarness(t, catalogBody, http.StatusOK)
	w := hn.do(http.MethodGet, "/api/opencode/providers/deepseek/models", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Models) != 1 || got.Models[0].ID != "deepseek-v4-pro" || got.Models[0].Name != "DeepSeek V4 Pro" {
		t.Fatalf("models: %+v", got.Models)
	}
}

// A company gateway is absent from the catalog; its ids are typed by hand, so an
// empty list is the right answer rather than 404.
func TestListOpenCodeProviderModelsUnknownProvider(t *testing.T) {
	hn := catalogHarness(t, catalogBody, http.StatusOK)
	w := hn.do(http.MethodGet, "/api/opencode/providers/my-gateway/models", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if body := w.Body.String(); body != `{"models":[]}` {
		t.Fatalf("body = %s", body)
	}
}

// An unreachable catalog must not fail the form: the picker degrades to typing.
func TestListOpenCodeProvidersUpstreamFailure(t *testing.T) {
	hn := catalogHarness(t, "", http.StatusInternalServerError)
	w := hn.do(http.MethodGet, "/api/opencode/providers", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	var got struct {
		Providers []any  `json:"providers"`
		Error     string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Providers) != 0 || got.Error == "" {
		t.Fatalf("want empty list plus a reason, got %+v", got)
	}
}

func TestOpenCodeCatalogUnconfigured(t *testing.T) {
	hn := newHarness(t)
	enableAdmin(t)
	hn.cookie = hn.login(t)
	hn.h.OpenCodeCatalog = nil
	if w := hn.do(http.MethodGet, "/api/opencode/providers", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("providers status %d", w.Code)
	}
	if w := hn.do(http.MethodGet, "/api/opencode/providers/deepseek/models", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("models status %d", w.Code)
	}
}
