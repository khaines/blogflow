package gitops

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

const testSecret = "SUPERSECRET-token-123"

// failingGitServer returns 500 for every request and records the basic-auth
// credentials it saw.
func failingGitServer(t *testing.T) (*httptest.Server, func() (string, string, bool)) {
	t.Helper()
	var mu sync.Mutex
	var user, pass string
	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if u, p, ok := r.BasicAuth(); ok {
			user, pass, sawAuth = u, p, true
		}
		mu.Unlock()
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	return srv, func() (string, string, bool) {
		mu.Lock()
		defer mu.Unlock()
		return user, pass, sawAuth
	}
}

func TestCloneOrPull_EmbeddedCredentialsNotInError(t *testing.T) {
	srv, seen := failingGitServer(t)
	repoURL := strings.Replace(srv.URL, "http://", "http://x-access-token:"+testSecret+"@", 1) + "/o/r.git"

	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.CloneOrPull(context.Background(), repoURL, "main", filepath.Join(t.TempDir(), "dst"))
	if err == nil {
		t.Fatal("expected clone error")
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("error leaks credential: %v", err)
	}
	u, pw, ok := seen()
	if !ok || u != "x-access-token" || pw != testSecret {
		t.Fatalf("expected embedded credentials sent as basic auth, got ok=%v user=%q", ok, u)
	}
}

func TestCloneOrPull_ScrubsPersistedRemoteCredentials(t *testing.T) {
	bare := newBareRepoWithCommit(t)
	dest := filepath.Join(t.TempDir(), "dst")

	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.CloneOrPull(context.Background(), bare, "master", dest); err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Simulate a clone made by an older version that persisted a token.
	repo, err := git.PlainOpen(dest)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := failingGitServer(t)
	leaky := strings.Replace(srv.URL, "http://", "http://u:"+testSecret+"@", 1) + "/o/r.git"
	cfg.Remotes["origin"] = &gitconfig.RemoteConfig{Name: "origin", URLs: []string{leaky}, Fetch: cfg.Remotes["origin"].Fetch}
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	// Pull fails against the 500 server and falls back to re-clone, which
	// also fails; neither error may contain the secret.
	_, err = p.CloneOrPull(context.Background(), leaky, "master", dest)
	if err == nil {
		t.Fatal("expected error from failing remote")
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("error leaks credential: %v", err)
	}
}

func TestScrubRemoteCredentials(t *testing.T) {
	bare := newBareRepoWithCommit(t)
	dest := filepath.Join(t.TempDir(), "dst")
	repo, err := git.PlainClone(dest, false, &git.CloneOptions{URL: bare})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := repo.Config()
	cfg.Remotes["origin"].URLs = []string{"https://u:" + testSecret + "@example.com/o/r.git"}
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	scrubbed, err := scrubRemoteCredentials(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !scrubbed {
		t.Fatal("expected scrubRemoteCredentials to report a change")
	}
	cfg, _ = repo.Config()
	if got := cfg.Remotes["origin"].URLs[0]; got != "https://example.com/o/r.git" {
		t.Fatalf("remote URL not scrubbed: %q", got)
	}
}

func TestRedactError(t *testing.T) {
	base := errors.New(`Get "https://user:` + testSecret + `@host/o/r.git/info/refs": 500`)
	err := redactError(base)
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("not redacted: %v", err)
	}
	if !strings.Contains(err.Error(), "https://[REDACTED]@host/") {
		t.Fatalf("unexpected message: %v", err)
	}
	if !errors.Is(err, base) {
		t.Fatal("redacted error must unwrap to the original")
	}
	if redactError(nil) != nil {
		t.Fatal("redactError(nil) must be nil")
	}
}

func TestSplitHTTPCredentials(t *testing.T) {
	tests := []struct {
		in, wantURL, wantUser, wantPass string
		wantAuth                        bool
	}{
		{"https://github.com/o/r.git", "https://github.com/o/r.git", "", "", false},
		{"https://tok@github.com/o/r.git", "https://github.com/o/r.git", "tok", "", true},
		{"https://u:p@github.com/o/r.git", "https://github.com/o/r.git", "u", "p", true},
		{"https://u:p%40ss@github.com/o/r.git", "https://github.com/o/r.git", "u", "p@ss", true},
		{"HTTP://u:p@example.com/r.git", "http://example.com/r.git", "u", "p", true},
		{"ssh://git@github.com/o/r.git", "ssh://git@github.com/o/r.git", "", "", false},
		{"git@github.com:o/r.git", "git@github.com:o/r.git", "", "", false},
	}
	for _, tt := range tests {
		gotURL, auth := splitHTTPCredentials(tt.in)
		if gotURL != tt.wantURL {
			t.Errorf("splitHTTPCredentials(%q) url = %q, want %q", tt.in, gotURL, tt.wantURL)
		}
		if !tt.wantAuth {
			if auth != nil {
				t.Errorf("splitHTTPCredentials(%q) expected nil auth", tt.in)
			}
			continue
		}
		ba, ok := auth.(*githttp.BasicAuth)
		if !ok {
			t.Fatalf("splitHTTPCredentials(%q) auth = %T, want *http.BasicAuth", tt.in, auth)
		}
		if ba.Username != tt.wantUser || ba.Password != tt.wantPass {
			t.Errorf("splitHTTPCredentials(%q) auth = %q/%q, want %q/%q",
				tt.in, ba.Username, ba.Password, tt.wantUser, tt.wantPass)
		}
	}
}

func TestResolveRemote(t *testing.T) {
	var logs strings.Builder
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	// No explicit auth: URL credentials become BasicAuth, URL is clean.
	p, err := NewPuller(nil, logger)
	if err != nil {
		t.Fatal(err)
	}
	u, auth := p.resolveRemote("https://u:" + testSecret + "@example.com/o/r.git")
	if u != "https://example.com/o/r.git" {
		t.Fatalf("clone URL = %q, want credentials stripped", u)
	}
	if ba, ok := auth.(*githttp.BasicAuth); !ok || ba.Password != testSecret {
		t.Fatalf("auth = %#v, want BasicAuth carrying the URL password", auth)
	}

	// Explicit auth wins; the conflict is logged once, not per call.
	p, err = NewPuller(&AuthConfig{Method: AuthToken, Token: "explicit"}, logger)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		u, auth = p.resolveRemote("https://u:" + testSecret + "@example.com/o/r.git")
	}
	if ba, ok := auth.(*githttp.BasicAuth); !ok || ba.Password != "explicit" {
		t.Fatalf("auth = %#v, want explicit token auth", auth)
	}
	if u != "https://example.com/o/r.git" {
		t.Fatalf("clone URL = %q, want credentials stripped", u)
	}
	if n := strings.Count(logs.String(), "ignoring credentials embedded in repo URL"); n != 1 {
		t.Fatalf("conflict warning logged %d times, want 1", n)
	}
	if strings.Contains(logs.String(), testSecret) {
		t.Fatal("logs contain the URL credential")
	}
}

// A successful pull must scrub credentials persisted in .git/config by
// older versions. origin stays a local path so the fetch succeeds; a second
// remote carries the persisted credential.
func TestCloneOrPull_PullScrubsPersistedCredentials(t *testing.T) {
	bare := newBareRepoWithCommit(t)
	dest := filepath.Join(t.TempDir(), "dst")

	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.CloneOrPull(context.Background(), bare, "master", dest); err != nil {
		t.Fatalf("clone: %v", err)
	}

	repo, err := git.PlainOpen(dest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateRemote(&gitconfig.RemoteConfig{
		Name: "legacy",
		URLs: []string{"https://u:" + testSecret + "@example.com/o/r.git"},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := p.CloneOrPull(context.Background(), bare, "master", dest); err != nil {
		t.Fatalf("pull: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dest, ".git", "config")) //nolint:gosec // test reads its own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testSecret) {
		t.Fatalf(".git/config still contains the credential:\n%s", raw)
	}
	if !strings.Contains(string(raw), "https://example.com/o/r.git") {
		t.Fatalf(".git/config lost the scrubbed remote URL:\n%s", raw)
	}
}

func TestSanitizeURL_UnparseableIsRedacted(t *testing.T) {
	for _, in := range []string{
		"https://u:bad%zz" + testSecret + "@github.com/o/r.git",
		"https://u:p@ss%zz" + testSecret + "@github.com/o/r.git",
	} {
		if got := SanitizeURL(in); strings.Contains(got, testSecret) {
			t.Errorf("SanitizeURL(%q) leaked credential: %q", in, got)
		}
	}
}

func TestRedactCredentials_AtInPassword(t *testing.T) {
	msg := `Get "https://u:p@ss` + testSecret + `@host/o/r.git/info/refs": 500; see https://docs.example.com/x`
	got := redactCredentials(msg)
	if strings.Contains(got, testSecret) || strings.Contains(got, "ss"+testSecret) {
		t.Fatalf("partial redaction: %q", got)
	}
	if !strings.Contains(got, "https://[REDACTED]@host/o/r.git") || !strings.Contains(got, "https://docs.example.com/x") {
		t.Fatalf("unexpected redaction result: %q", got)
	}
}
