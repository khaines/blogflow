package gitops

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
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

	if err := scrubRemoteCredentials(repo); err != nil {
		t.Fatal(err)
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
	}{
		{"https://github.com/o/r.git", "https://github.com/o/r.git", "", ""},
		{"https://tok@github.com/o/r.git", "https://github.com/o/r.git", "tok", ""},
		{"https://u:p@github.com/o/r.git", "https://github.com/o/r.git", "u", "p"},
		{"ssh://git@github.com/o/r.git", "ssh://git@github.com/o/r.git", "", ""},
		{"git@github.com:o/r.git", "git@github.com:o/r.git", "", ""},
	}
	for _, tt := range tests {
		gotURL, auth := splitHTTPCredentials(tt.in)
		if gotURL != tt.wantURL {
			t.Errorf("splitHTTPCredentials(%q) url = %q, want %q", tt.in, gotURL, tt.wantURL)
		}
		if tt.wantUser == "" {
			if auth != nil {
				t.Errorf("splitHTTPCredentials(%q) expected nil auth", tt.in)
			}
			continue
		}
		if auth == nil || auth.String() == "" {
			t.Fatalf("splitHTTPCredentials(%q) expected auth", tt.in)
		}
	}
}

func TestSanitizeURL_UnparseableIsRedacted(t *testing.T) {
	got := SanitizeURL("https://u:bad%zz" + testSecret + "@github.com/o/r.git")
	if strings.Contains(got, testSecret) {
		t.Fatalf("SanitizeURL leaked credential: %q", got)
	}
}
