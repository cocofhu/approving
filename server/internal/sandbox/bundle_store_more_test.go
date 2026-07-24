package sandbox

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBundleStoreServeHTTPAndSweep(t *testing.T) {
	store := NewBundleStore()
	id, token := store.Put([]byte("abc"), time.Millisecond)
	time.Sleep(5 * time.Millisecond)
	if _, ok := store.Get(id, token); ok {
		t.Fatal("expired should miss")
	}
	id, token = store.Put([]byte("xyz"), time.Minute)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	store.ServeHTTP(w, req, id)
	if w.Code == http.StatusOK {
		t.Fatal("wrong token")
	}
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodHead, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	store.ServeHTTP(w, req, id+".tgz")
	if w.Code != http.StatusOK {
		t.Fatalf("head: %d", w.Code)
	}
	store.Delete(id)
	if _, ok := store.Get(id, token); ok {
		t.Fatal("deleted")
	}
	var nilStore *BundleStore
	if id2, tok := nilStore.Put(nil, 0); id2 != "" || tok != "" {
		t.Fatal("nil put")
	}
	if _, ok := nilStore.Get("a", "b"); ok {
		t.Fatal("nil get")
	}
	nilStore.Delete("a")
}
