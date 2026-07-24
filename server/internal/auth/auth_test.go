package auth_test

import (
	"testing"
	"time"

	"github.com/cocofhu/approving/internal/auth"
	"github.com/cocofhu/approving/internal/config"
	"github.com/cocofhu/approving/internal/database"
	"github.com/cocofhu/approving/internal/models"

	"golang.org/x/crypto/bcrypt"
)

func testConfig(users []config.AuthUser) func() *config.Config {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Users:        users,
			MaxFailures:  5,
			LockDuration: "5m",
			SessionTTL:   "168h",
		},
	}
	config.StoreConfig(cfg)
	return config.GetConfig
}

func TestValidateRedirect(t *testing.T) {
	cases := map[string]string{
		"":            "/",
		"/workflows":  "/workflows",
		"//evil.com":  "/",
		"http://x":    "/",
		"/sandbox/1/": "/sandbox/1/",
	}
	for in, want := range cases {
		if got := auth.ValidateRedirect(in); got != want {
			t.Fatalf("%q => %q want %q", in, got, want)
		}
	}
}

func TestLoginLogoutSession(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	db, err := database.OpenSQLiteTest(t.TempDir() + "/auth.db")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Session{}); err != nil {
		t.Fatal(err)
	}
	svc := auth.NewService(db, testConfig([]config.AuthUser{
		{Username: "admin", PasswordHash: string(hash)},
	}))

	sess, err := svc.Login("admin", "secret")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.Username != "admin" {
		t.Fatalf("username: %s", sess.Username)
	}
	if sess.ExpiresAt.Before(time.Now().Add(167 * time.Hour)) {
		t.Fatalf("expires too soon: %v", sess.ExpiresAt)
	}

	got, err := svc.ValidateSession(sess.ID)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if got.ID != sess.ID {
		t.Fatalf("token mismatch")
	}

	if _, err := svc.Login("admin", "wrong"); err == nil {
		t.Fatal("expected login failure")
	}

	if err := svc.DeleteSession(sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ValidateSession(sess.ID); err == nil {
		t.Fatal("expected expired session")
	}
}

func TestRateLimiter(t *testing.T) {
	cfgFn := testConfig(nil)
	rl := auth.NewRateLimiter(cfgFn)
	ip := "1.2.3.4"
	for i := 0; i < 4; i++ {
		if rl.RecordFailure(ip) {
			t.Fatalf("locked early at %d", i)
		}
	}
	if !rl.RecordFailure(ip) {
		t.Fatal("expected lock on 5th failure")
	}
	if _, locked := rl.Check(ip); !locked {
		t.Fatal("expected locked")
	}
	rl.Reset(ip)
	if _, locked := rl.Check(ip); locked {
		t.Fatal("expected unlocked after reset")
	}
}
