package gitops

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/cgi" //nolint:gosec // test-only git-http-backend; Go >= 1.6.3 is not affected by httpoxy
	"net/http/httptest"
	"os"
	"os/exec"
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
// credentials of the first request it saw.
func failingGitServer(t *testing.T) (*httptest.Server, func() (string, string, bool)) {
	t.Helper()
	var mu sync.Mutex
	var user, pass string
	var sawAuth, sawRequest bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		// Record only the first request: a failed pull falls back to a
		// re-clone, which must not mask a fetch that sent no auth.
		if !sawRequest {
			sawRequest = true
			user, pass, sawAuth = r.BasicAuth()
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

	if !hasPersistedCredentials(repo) {
		t.Fatal("hasPersistedCredentials = false before scrub")
	}
	scrubbed, err := scrubRemoteCredentials(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !scrubbed {
		t.Fatal("expected scrubRemoteCredentials to report a change")
	}
	if hasPersistedCredentials(repo) {
		t.Fatal("hasPersistedCredentials = true after scrub")
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

func TestCloneOrPull_PullFetchUsesURLCredentials(t *testing.T) {
	bare := newBareRepoWithCommit(t)
	dest := filepath.Join(t.TempDir(), "dst")

	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := p.CloneOrPull(context.Background(), bare, "master", dest); err != nil {
		t.Fatalf("clone: %v", err)
	}

	srv, seen := failingGitServer(t)
	repo, err := git.PlainOpen(dest)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Remotes["origin"].URLs = []string{srv.URL + "/o/r.git"}
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	repoURL := strings.Replace(srv.URL, "http://", "http://x-access-token:"+testSecret+"@", 1) + "/o/r.git"
	_, err = p.CloneOrPull(context.Background(), repoURL, "master", dest)
	if err == nil {
		t.Fatal("expected fetch error from failing server")
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("error leaks credential: %v", err)
	}
	u, pw, ok := seen()
	if !ok || u != "x-access-token" || pw != testSecret {
		t.Fatalf("fetch did not carry URL credentials as basic auth: ok=%v user=%q", ok, u)
	}
}

// A scheme URL that does not parse must be rejected before go-git, whose
// parse error would echo it.
func TestCloneOrPull_UnparseableURLNotEchoed(t *testing.T) {
	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, in := range ambiguousCredentialURLs {
		_, err := p.CloneOrPull(context.Background(), in, "main", filepath.Join(t.TempDir(), "dst"))
		if !errors.Is(err, errUnparseableRepoURL) {
			t.Errorf("CloneOrPull(%q) err = %v, want errUnparseableRepoURL", in, err)
		}
	}
}

// ambiguousCredentialURLs hold a credential with an unescaped delimiter.
// Some fail to parse; others parse with the secret in the path, query or
// fragment and no userinfo.
var ambiguousCredentialURLs = []string{
	"https://u:pa/ss" + testSecret + "@github.com/o/r.git",
	"https://u:pa ss" + testSecret + "@github.com/o/r.git",
	"https://u:/" + testSecret + "@github.com/o/r.git",
	"https://u:1234/" + testSecret + "@github.com/o/r.git",
	"https://ghp_ab/" + testSecret + "@github.com/o/r.git",
	"https://u:12?" + testSecret + "@github.com/o/r.git",
	"https://u:12#" + testSecret + "@github.com/o/r.git",
	"https://u:p@ss/" + testSecret + "@github.com/o/r.git",
	"https://u:p@ss?" + testSecret + "@github.com/o/r.git",
	"https://u:p@ss#" + testSecret + "@github.com/o/r.git",
	"https://u:p@ss://" + testSecret + "@github.com/o/r.git",
	"https://u:p@ss:/" + testSecret + "@github.com/o/r.git",
	" https://u:1234/" + testSecret + "@github.com/o/r.git",
}

func TestSanitizeURL_UnparseableIsRedacted(t *testing.T) {
	for _, in := range append([]string{
		"https://u:bad%zz" + testSecret + "@github.com/o/r.git",
		"https://u:p@ss%zz" + testSecret + "@github.com/o/r.git",
	}, ambiguousCredentialURLs...) {
		if got := SanitizeURL(in); strings.Contains(got, testSecret) {
			t.Errorf("SanitizeURL(%q) leaked credential: %q", in, got)
		}
	}
	if got, want := SanitizeURL("https://u:"+testSecret+"@github.com/o/r.git"), "https://github.com/o/r.git"; got != want {
		t.Errorf("SanitizeURL(userinfo URL) = %q, want %q", got, want)
	}
	if got, want := SanitizeURL("git@github.com:o/r.git"), "git@github.com:o/r.git"; got != want {
		t.Errorf("scp-style URL changed: got %q, want %q", got, want)
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

func TestRedactCredentials_WhitespaceInPassword(t *testing.T) {
	got := redactCredentials(`Get "https://u:pa ss` + testSecret + `@host/o/r.git/info/refs": 500`)
	if strings.Contains(got, testSecret) {
		t.Fatalf("not redacted: %q", got)
	}
}

// authGitServer serves the bare repo at bare over smart HTTP (via
// git-http-backend) and requires basic auth u/testSecret. It returns the
// repo URL without credentials and a func counting unauthenticated requests.
func authGitServer(t *testing.T, bare string) (string, func() int) {
	t.Helper()
	backend := filepath.Join(gitExecPath(t), "git-http-backend")
	if _, err := os.Stat(backend); err != nil {
		t.Skipf("git-http-backend not available: %v", err)
	}
	var mu sync.Mutex
	rejected := 0
	h := &cgi.Handler{
		Path: backend,
		Env:  []string{"GIT_PROJECT_ROOT=" + filepath.Dir(bare), "GIT_HTTP_EXPORT_ALL=1"},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if u, p, ok := r.BasicAuth(); !ok || u != "u" || p != testSecret {
			mu.Lock()
			rejected++
			mu.Unlock()
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/" + filepath.Base(bare), func() int {
		mu.Lock()
		defer mu.Unlock()
		return rejected
	}
}

func gitExecPath(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "--exec-path").Output()
	if err != nil {
		t.Skipf("git not available: %v", err)
	}
	return strings.TrimSpace(string(out))
}

func withCreds(u string) string {
	return strings.Replace(u, "http://", "http://u:"+testSecret+"@", 1)
}

// A clone whose .git/config holds the only credentials (made by another
// process or an older version) must keep syncing: they are not scrubbed
// when nothing else is configured, and the checkout survives repeated pulls.
func TestCloneOrPull_KeepsOnlyPersistedCredentials(t *testing.T) {
	repoURL, rejected := authGitServer(t, newBareRepoWithCommit(t))
	dest := filepath.Join(t.TempDir(), "dst")
	if _, err := git.PlainClone(dest, false, &git.CloneOptions{URL: withCreds(repoURL)}); err != nil {
		t.Fatalf("seed clone: %v", err)
	}

	var logs strings.Builder
	p, err := NewPuller(nil, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := p.CloneOrPull(context.Background(), repoURL, "master", dest); err != nil {
			t.Fatalf("pull %d: %v", i, err)
		}
	}
	if n := rejected(); n != 0 {
		t.Fatalf("%d unauthenticated requests; persisted credentials were not used", n)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Fatalf("checkout lost: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dest, ".git", "config")) //nolint:gosec // test reads its own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), testSecret) {
		t.Fatal("the only credentials were scrubbed from .git/config")
	}
	if n := strings.Count(logs.String(), "git credentials are stored in .git/config"); n != 1 {
		t.Fatalf("persisted-credentials warning logged %d times, want 1\n%s", n, logs.String())
	}
}

// With credentials configured (here, in the repo URL), credentials persisted
// in any remote are scrubbed, and pulls keep working.
func TestCloneOrPull_PullScrubsPersistedCredentials(t *testing.T) {
	repoURL, rejected := authGitServer(t, newBareRepoWithCommit(t))
	dest := filepath.Join(t.TempDir(), "dst")
	if _, err := git.PlainClone(dest, false, &git.CloneOptions{URL: withCreds(repoURL)}); err != nil {
		t.Fatalf("seed clone: %v", err)
	}

	p, err := NewPuller(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := p.CloneOrPull(context.Background(), withCreds(repoURL), "master", dest); err != nil {
			t.Fatalf("pull %d: %v", i, err)
		}
	}
	if n := rejected(); n != 0 {
		t.Fatalf("%d unauthenticated requests", n)
	}
	raw, err := os.ReadFile(filepath.Join(dest, ".git", "config")) //nolint:gosec // test reads its own temp dir
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), testSecret) {
		t.Fatalf(".git/config still contains the credential:\n%s", raw)
	}
	if !strings.Contains(string(raw), repoURL) {
		t.Fatalf(".git/config lost the scrubbed remote URL:\n%s", raw)
	}
}

// A scrub failure (e.g. read-only .git/config) must not stop the pull, and
// is warned about once, not on every pull.
func TestCloneOrPull_ScrubFailureIsNotFatal(t *testing.T) {
	orig := scrubRemote
	scrubRemote = func(*git.Repository) (bool, error) {
		return false, errors.New("read-only file system")
	}
	t.Cleanup(func() { scrubRemote = orig })

	repoURL, _ := authGitServer(t, newBareRepoWithCommit(t))
	dest := filepath.Join(t.TempDir(), "dst")
	var logs strings.Builder
	p, err := NewPuller(nil, slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if _, err := p.CloneOrPull(context.Background(), withCreds(repoURL), "master", dest); err != nil {
			t.Fatalf("sync %d: %v", i, err)
		}
	}
	if n := strings.Count(logs.String(), "could not remove credentials persisted"); n != 1 {
		t.Fatalf("scrub warning logged %d times, want 1\n%s", n, logs.String())
	}
}

// When persisted credentials are kept (no other auth), go-git errors that
// echo the remote URL must still be redacted.
func TestCloneOrPull_KeptCredentialsNotInError(t *testing.T) {
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
	cfg, err := repo.Config()
	if err != nil {
		t.Fatal(err)
	}
	srv, _ := failingGitServer(t)
	cfg.Remotes["origin"].URLs = []string{withCreds(srv.URL) + "/o/r.git"}
	if err := repo.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}

	_, err = p.CloneOrPull(context.Background(), srv.URL+"/o/r.git", "master", dest)
	if err == nil {
		t.Fatal("expected error from failing remote")
	}
	if strings.Contains(err.Error(), testSecret) {
		t.Fatalf("error leaks credential: %v", err)
	}
}
