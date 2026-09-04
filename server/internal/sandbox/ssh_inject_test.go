package sandbox

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

func TestPackSSHInjectTarGz(t *testing.T) {
	data, err := PackSSHInjectTarGz("-----BEGIN KEY-----\nABC\n-----END KEY-----", "host ssh-ed25519 AAAA")
	if err != nil {
		t.Fatal(err)
	}
	if data == nil {
		t.Fatal("expected bytes")
	}
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	seen := map[string]string{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		seen[hdr.Name] = string(b)
		if hdr.Name == "id_rsa" && hdr.Mode&0o777 != 0o600 {
			t.Fatalf("id_rsa mode=%o", hdr.Mode)
		}
	}
	if !strings.Contains(seen["id_rsa"], "BEGIN KEY") {
		t.Fatalf("id_rsa=%q", seen["id_rsa"])
	}
	if !strings.Contains(seen["known_hosts"], "AAAA") {
		t.Fatalf("known_hosts=%q", seen["known_hosts"])
	}
}

func TestPackSSHInjectTarGzEmpty(t *testing.T) {
	data, err := PackSSHInjectTarGz("  ", "")
	if err != nil {
		t.Fatal(err)
	}
	if data != nil {
		t.Fatal("expected nil for empty")
	}
}

func TestApplySSHCredentialsStripsEnv(t *testing.T) {
	spec := &Spec{Env: map[string]string{
		"GIT_SSH_PRIVATE_KEY": "k",
		"GIT_SSH_KNOWN_HOSTS": "h",
		"GITHUB_TOKEN":        "t",
	}}
	ApplySSHCredentials(spec, "meta-key", "meta-hosts")
	if spec.SSHPrivateKey != "meta-key" || spec.SSHKnownHosts != "meta-hosts" {
		t.Fatalf("ssh fields %+v", spec)
	}
	if _, ok := spec.Env["GIT_SSH_PRIVATE_KEY"]; ok {
		t.Fatal("private key env not stripped")
	}
	if _, ok := spec.Env["GIT_SSH_KNOWN_HOSTS"]; ok {
		t.Fatal("known_hosts env not stripped")
	}
	if spec.Env["GITHUB_TOKEN"] != "t" {
		t.Fatal("token should remain")
	}
}

func TestBundleStorePutWithTokenShared(t *testing.T) {
	s := NewBundleStore()
	token := "shared-token-xyz"
	id1 := s.PutWithToken([]byte("aaa"), DefaultInjectBundleTTL, token)
	id2 := s.PutWithToken([]byte("bbb"), DefaultInjectBundleTTL, token)
	if id1 == "" || id2 == "" || id1 == id2 {
		t.Fatalf("ids %q %q", id1, id2)
	}
	b1, ok := s.Get(id1, token)
	if !ok || string(b1) != "aaa" {
		t.Fatalf("get1 %v %q", ok, b1)
	}
	b2, ok := s.Get(id2, token)
	if !ok || string(b2) != "bbb" {
		t.Fatalf("get2 %v %q", ok, b2)
	}
}
