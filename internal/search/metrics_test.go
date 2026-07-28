package search

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// TestRecordQuery_IncrementsByStatus verifies the query counter and latency
// histogram record per-status outcomes.
func TestRecordQuery_IncrementsByStatus(t *testing.T) {
	for _, status := range []string{StatusOK, StatusInvalid, StatusError} {
		before := testutil.ToFloat64(queriesTotal.WithLabelValues(status))
		RecordQuery(status, 3*time.Millisecond)
		after := testutil.ToFloat64(queriesTotal.WithLabelValues(status))
		if after-before != 1 {
			t.Errorf("queriesTotal{status=%s} delta = %v, want 1", status, after-before)
		}
	}
}

// TestRecordResults_ZeroResultCounter verifies the zero-result counter fires
// only on empty result sets.
func TestRecordResults_ZeroResultCounter(t *testing.T) {
	before := testutil.ToFloat64(zeroResultsTotal)
	RecordResults(0)
	if got := testutil.ToFloat64(zeroResultsTotal) - before; got != 1 {
		t.Errorf("zeroResultsTotal delta on 0 results = %v, want 1", got)
	}
	steady := testutil.ToFloat64(zeroResultsTotal)
	RecordResults(7)
	if got := testutil.ToFloat64(zeroResultsTotal); got != steady {
		t.Errorf("zeroResultsTotal changed on non-zero results: %v", got-steady)
	}
}

// TestObserveRebuild_SetsGauges verifies rebuild observation updates the index
// gauges and the snapshot-generation gauge.
func TestObserveRebuild_SetsGauges(t *testing.T) {
	si := Build(context.Background(), baseCorpus(), testCfg(), 42)
	ObserveRebuild(si, 2*time.Millisecond, si.LogicalBytes())

	if got := testutil.ToFloat64(indexDocuments); got != float64(si.DocCount()) {
		t.Errorf("indexDocuments = %v, want %d", got, si.DocCount())
	}
	if got := testutil.ToFloat64(indexTokens); got != float64(si.TokenCount()) {
		t.Errorf("indexTokens = %v, want %d", got, si.TokenCount())
	}
	if got := testutil.ToFloat64(indexMemoryBytes); got != float64(si.LogicalBytes()) {
		t.Errorf("indexMemoryBytes = %v, want %d", got, si.LogicalBytes())
	}
	if got := testutil.ToFloat64(snapshotGeneration); got != 42 {
		t.Errorf("snapshotGeneration = %v, want 42", got)
	}
}

// TestObserveRebuild_TruncationCounter verifies a truncated build increments the
// truncation counter labelled by reason.
func TestObserveRebuild_TruncationCounter(t *testing.T) {
	cfg := testCfg()
	cfg.MaxDocs = 1 // force truncation on the 3-doc corpus
	si := Build(context.Background(), baseCorpus(), cfg, 1)
	if !si.Truncated() {
		t.Fatal("expected truncated index for the test")
	}
	label := string(TruncationMaxDocs)
	before := testutil.ToFloat64(indexTruncatedTotal.WithLabelValues(label))
	ObserveRebuild(si, time.Millisecond, si.LogicalBytes())
	if got := testutil.ToFloat64(indexTruncatedTotal.WithLabelValues(label)) - before; got != 1 {
		t.Errorf("indexTruncatedTotal{reason=%s} delta = %v, want 1", label, got)
	}
}

// TestRecordRebuildFailure_UpdatesState verifies the failure path advances the
// snapshot-generation gauge and zeroes the active-index gauges so dashboards do
// not show a stale healthy index while /search serves 503.
func TestRecordRebuildFailure_UpdatesState(t *testing.T) {
	// Seed gauges with a successful build first.
	si := Build(context.Background(), baseCorpus(), testCfg(), 10)
	ObserveRebuild(si, time.Millisecond, si.LogicalBytes())

	RecordRebuildFailure(11, 2*time.Millisecond)

	if got := testutil.ToFloat64(indexDocuments); got != 0 {
		t.Errorf("indexDocuments after failure = %v, want 0", got)
	}
	if got := testutil.ToFloat64(indexTokens); got != 0 {
		t.Errorf("indexTokens after failure = %v, want 0", got)
	}
	if got := testutil.ToFloat64(indexMemoryBytes); got != 0 {
		t.Errorf("indexMemoryBytes after failure = %v, want 0", got)
	}
	if got := testutil.ToFloat64(indexRebuildPeakBytes); got != 0 {
		t.Errorf("indexRebuildPeakBytes after failure = %v, want 0", got)
	}
	if got := testutil.ToFloat64(snapshotGeneration); got != 11 {
		t.Errorf("snapshotGeneration after failure = %v, want 11", got)
	}
}
