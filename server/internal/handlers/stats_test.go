package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cocofhu/approving/internal/services"
)

func TestGetGlobalTokenStatsOmitsWindowDefaultsAll(t *testing.T) {
	hn := newHarness(t)

	w := hn.do("GET", "/api/stats/token?timezone=UTC", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("omit window: %d %s", w.Code, w.Body.String())
	}
	var got services.GlobalTokenStatsResult
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v body=%s", err, w.Body.String())
	}
	if got.Window != services.TokenStatsWindowAll {
		t.Fatalf("GET /stats/token omitted window want all, got %q", got.Window)
	}
	if got.BucketWidth != services.TokenStatsBucketWeek {
		t.Fatalf("all should use week buckets, got %q", got.BucketWidth)
	}

	w = hn.do("GET", "/api/stats/token?window=30d&timezone=UTC", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("explicit 30d: %d %s", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode 30d: %v", err)
	}
	if got.Window != services.TokenStatsWindow30d {
		t.Fatalf("explicit window=30d want 30d, got %q", got.Window)
	}
}
