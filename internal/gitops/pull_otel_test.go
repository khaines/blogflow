package gitops

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func setupGitopsTestTracer(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exp))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
	})
	return exp
}

func TestCloneOrPull_CreatesSpans(t *testing.T) {
	exp := setupGitopsTestTracer(t)

	bareDir := newBareRepoWithCommit(t)
	destDir := filepath.Join(t.TempDir(), "dest")

	puller, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("NewPuller: %v", err)
	}

	changed, err := puller.CloneOrPull(context.Background(), bareDir, "master", destDir)
	if err != nil {
		t.Fatalf("CloneOrPull: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for fresh clone")
	}

	spans := exp.GetSpans()
	spanNames := make(map[string]bool)
	for _, s := range spans {
		spanNames[s.Name] = true
	}

	if !spanNames["gitops.CloneOrPull"] {
		t.Errorf("missing 'gitops.CloneOrPull' span; got: %v", spanNames)
	}
	if !spanNames["gitops.clone"] {
		t.Errorf("missing 'gitops.clone' child span; got: %v", spanNames)
	}

	// Verify attributes on parent span.
	for _, s := range spans {
		if s.Name == "gitops.CloneOrPull" {
			attrs := make(map[string]any)
			for _, a := range s.Attributes {
				attrs[string(a.Key)] = a.Value.AsInterface()
			}
			if v, ok := attrs["gitops.repo_url"]; !ok {
				t.Error("missing gitops.repo_url attribute")
			} else if _, isStr := v.(string); !isStr {
				t.Errorf("gitops.repo_url is not a string: %T", v)
			}
			if v, ok := attrs["gitops.branch"]; !ok || v != "master" {
				t.Errorf("gitops.branch = %v, want 'master'", v)
			}
			if v, ok := attrs["gitops.operation"]; !ok || v != "clone" {
				t.Errorf("gitops.operation = %v, want 'clone'", v)
			}
		}
	}
}

func TestCloneOrPull_Pull_CreatesSpans(t *testing.T) {
	exp := setupGitopsTestTracer(t)

	bareDir := newBareRepoWithCommit(t)
	destDir := filepath.Join(t.TempDir(), "dest")

	puller, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("NewPuller: %v", err)
	}

	// Initial clone
	if _, err := puller.CloneOrPull(context.Background(), bareDir, "master", destDir); err != nil {
		t.Fatalf("initial clone: %v", err)
	}

	exp.Reset()

	// Pull (already up to date)
	_, err = puller.CloneOrPull(context.Background(), bareDir, "master", destDir)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}

	spans := exp.GetSpans()
	spanNames := make(map[string]bool)
	for _, s := range spans {
		spanNames[s.Name] = true
	}

	if !spanNames["gitops.CloneOrPull"] {
		t.Errorf("missing 'gitops.CloneOrPull' span; got: %v", spanNames)
	}
	if !spanNames["gitops.pull"] {
		t.Errorf("missing 'gitops.pull' child span; got: %v", spanNames)
	}

	// Check operation attribute is "pull"
	for _, s := range spans {
		if s.Name == "gitops.CloneOrPull" {
			for _, a := range s.Attributes {
				if string(a.Key) == "gitops.operation" && a.Value.AsInterface() != "pull" {
					t.Errorf("gitops.operation = %v, want 'pull'", a.Value.AsInterface())
				}
			}
		}
	}
}

func TestCloneOrPull_ErrorSetsSpanStatus(t *testing.T) {
	exp := setupGitopsTestTracer(t)

	puller, err := NewPuller(&AuthConfig{Method: AuthNone}, nil)
	if err != nil {
		t.Fatalf("NewPuller: %v", err)
	}

	// Clone from invalid path — will fail
	destDir := filepath.Join(t.TempDir(), "dest")
	_, err = puller.CloneOrPull(context.Background(), "/nonexistent/repo", "master", destDir)
	if err == nil {
		t.Fatal("expected error cloning from invalid path")
	}

	spans := exp.GetSpans()
	var found bool
	for _, s := range spans {
		if s.Name == "gitops.CloneOrPull" {
			found = true
			if s.Status.Code.String() != "Error" {
				t.Errorf("span status = %v, want Error", s.Status.Code)
			}
		}
	}
	if !found {
		names := make([]string, len(spans))
		for i, s := range spans {
			names[i] = s.Name
		}
		t.Errorf("span 'gitops.CloneOrPull' not found; got: %s", strings.Join(names, ", "))
	}
}
