package gateshare

import (
	"fmt"
	"testing"
	"time"
)

func TestIPLimiterGCExpired(t *testing.T) {
	l := NewIPLimiter()
	l.mu.Lock()
	for i := 0; i < ipLimiterGCAt; i++ {
		l.byIP[fmt.Sprintf("10.2.0.%d", i)] = &ipWindow{start: time.Now().Add(-2 * time.Minute), count: 1}
	}
	l.mu.Unlock()
	if !l.Allow("10.9.9.9") {
		t.Fatal("fresh IP should allow")
	}
	if n := l.LenForTest(); n > 2 {
		t.Fatalf("expected GC to drop expired IPs, len=%d", n)
	}
}
