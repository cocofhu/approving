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
		l.byKey[fmt.Sprintf("10.2.0.%d\x00%s", i, RateBucketPreview)] = &ipWindow{start: time.Now().Add(-2 * time.Minute), count: 1}
	}
	l.mu.Unlock()
	if !l.Allow("10.9.9.9") {
		t.Fatal("fresh IP should allow")
	}
	if n := l.LenForTest(); n > 2 {
		t.Fatalf("expected GC to drop expired IPs, len=%d", n)
	}
}

func TestIPLimiterBucketsIndependent(t *testing.T) {
	l := NewIPLimiter()
	ip := "10.1.2.3"
	for i := 0; i < publicRateMax; i++ {
		if !l.AllowBucket(ip, RateBucketPreview) {
			t.Fatalf("preview %d should allow", i)
		}
	}
	if l.AllowBucket(ip, RateBucketPreview) {
		t.Fatal("preview should be limited")
	}
	if !l.AllowBucket(ip, RateBucketDecide) {
		t.Fatal("decide must still have its own budget after preview is full")
	}
}

func TestIPLimiterDecideBucketTrips(t *testing.T) {
	l := NewIPLimiter()
	ip := "10.1.2.4"
	for i := 0; i < publicRateMax; i++ {
		if !l.AllowBucket(ip, RateBucketDecide) {
			t.Fatalf("decide %d should allow", i)
		}
	}
	if l.AllowBucket(ip, RateBucketDecide) {
		t.Fatal("decide should return 429 after its own budget")
	}
}
