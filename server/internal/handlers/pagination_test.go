package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/cocofhu/approving/internal/models"

	"github.com/gin-gonic/gin"
)

func TestParsePaginationValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		query string
		ok    bool
	}{
		{"", true},
		{"page=2&pageSize=10", true},
		{"page=0", false},
		{"pageSize=101", false},
		{"pageSize=abc", false},
	}
	for _, tc := range cases {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("GET", "/?"+tc.query, nil)
		_, ok := parsePagination(c)
		if ok != tc.ok {
			t.Fatalf("query %q ok=%v want %v code=%d", tc.query, ok, tc.ok, w.Code)
		}
	}
}

func TestParseCursorPaginationValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/?limit=200", nil)
	_, ok := parseCursorPagination(c)
	if ok {
		t.Fatal("limit>100 should fail")
	}
}

func TestPaginatedResponseNilSlice(t *testing.T) {
	out := paginatedResponse(nil, 0, 1, 20)
	if out["items"] == nil {
		t.Fatal("items should be empty slice")
	}
	var runs []models.Run
	out2 := paginatedResponse(runs, 0, 1, 20)
	if out2["hasMore"] != false {
		t.Fatal("hasMore")
	}
}
