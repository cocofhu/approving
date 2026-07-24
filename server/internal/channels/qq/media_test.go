package qq

import (
	"net"
	"testing"
)

func TestIsBlockedIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"::1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, c := range cases {
		got := isBlockedIP(net.ParseIP(c.ip))
		if got != c.want {
			t.Errorf("isBlockedIP(%s) = %v want %v", c.ip, got, c.want)
		}
	}
}

func TestValidatePublicHTTPURL(t *testing.T) {
	if err := validatePublicHTTPURL("ftp://example.com/a.png"); err == nil {
		t.Fatal("expected scheme reject")
	}
	if err := validatePublicHTTPURL("https://127.0.0.1/a.png"); err == nil {
		t.Fatal("expected loopback reject")
	}
	if err := validatePublicHTTPURL("https://10.1.2.3/a.png"); err == nil {
		t.Fatal("expected private reject")
	}
	if err := validatePublicHTTPURL("https://8.8.8.8/a.png"); err != nil {
		t.Fatalf("public IP should pass: %v", err)
	}
}
