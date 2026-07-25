package sandbox

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestQuoteShellPathRejectsMetacharacters(t *testing.T) {
	t.Parallel()
	bad := []string{
		"",
		"foo;id",
		"foo`id`",
		"foo$(id)",
		"foo && id",
		"foo|id",
		"foo\nid",
		"../etc/passwd",
		"/tmp/foo bar",
		"foo'bar",
	}
	for _, p := range bad {
		if _, err := quoteShellPath(p); err == nil {
			t.Fatalf("quoteShellPath(%q) want error", p)
		}
	}
	ok, err := quoteShellPath("/root/workspace/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if ok != `'/root/workspace/a.txt'` {
		t.Fatalf("got %q", ok)
	}
}

func TestHostKeyTOFUPinAndMismatch(t *testing.T) {
	t.Parallel()
	cache := newHostKeyCache()
	c := sshCreds{host: "127.0.0.1", port: 2222, hostKeys: cache}

	key1 := mustSSHPublicKey(t)
	key2 := mustSSHPublicKey(t)

	cb1 := c.hostKeyCallback()
	if err := cb1("127.0.0.1", dummyAddr{}, key1); err != nil {
		t.Fatalf("first trust: %v", err)
	}
	if got := cache.get(c.addrKey()); got == nil || string(got.Marshal()) != string(key1.Marshal()) {
		t.Fatal("host key not pinned after first trust")
	}

	cb2 := c.hostKeyCallback()
	if err := cb2("127.0.0.1", dummyAddr{}, key1); err != nil {
		t.Fatalf("same key should pass FixedHostKey: %v", err)
	}
	err := cb2("127.0.0.1", dummyAddr{}, key2)
	if err == nil {
		t.Fatal("mismatch should be rejected")
	}
	if !strings.Contains(err.Error(), "mismatch") && !errors.Is(err, err) {
		// ssh.FixedHostKey returns a clear error; accept any non-nil.
		t.Logf("mismatch error: %v", err)
	}
}

func mustSSHPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

type dummyAddr struct{}

func (dummyAddr) Network() string { return "tcp" }
func (dummyAddr) String() string  { return "127.0.0.1:2222" }

var _ net.Addr = dummyAddr{}
