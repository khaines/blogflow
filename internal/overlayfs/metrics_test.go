package overlayfs

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newMetricsOverlay(reg *prometheus.Registry, layers ...fs.FS) *OverlayFS {
	names := make([]string, len(layers))
	for i := range layers {
		names[i] = layerName(i)
	}
	ofs, err := NewOverlayFS(layers...).WithLayerNames(names).WithOptions(WithMetrics(reg))
	if err != nil {
		panic(err)
	}
	return ofs
}

func counterValue(reg *prometheus.Registry, name string, labels map[string]string) float64 {
	families, _ := reg.Gather()
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func histogramCount(reg *prometheus.Registry, name string, labels map[string]string) uint64 {
	families, _ := reg.Gather()
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetHistogram().GetSampleCount()
			}
		}
	}
	return 0
}

func gaugeValue(reg *prometheus.Registry, name string) float64 {
	families, _ := reg.Gather()
	for _, f := range families {
		if f.GetName() != name {
			continue
		}
		for _, m := range f.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return -1
}

func matchLabels(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	m := make(map[string]string, len(pairs))
	for _, p := range pairs {
		m[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if m[k] != v {
			return false
		}
	}
	return true
}

func TestMetrics_Registered(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = newMetricsOverlay(reg, fstest.MapFS{"a.txt": {Data: []byte("a")}})

	families, err := reg.Gather()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]bool{
		"blogflow_overlay_resolve_duration_seconds": false,
		"blogflow_overlay_layer_hit_total":          false,
		"blogflow_overlay_miss_total":               false,
		"blogflow_overlay_negcache_hit_total":       false,
		"blogflow_overlay_negcache_size":            false,
		"blogflow_overlay_path_rejected_total":      false,
	}

	for _, f := range families {
		if _, ok := want[f.GetName()]; ok {
			want[f.GetName()] = true
		}
	}

	// Trigger at least one operation so lazy metrics appear
	ofs := newMetricsOverlay(prometheus.NewRegistry(), fstest.MapFS{"b.txt": {Data: []byte("b")}})
	_, _ = ofs.Open("b.txt")
	_, _ = ofs.Open("../bad")

	// Re-check with a fresh registry that has activity
	reg2 := prometheus.NewRegistry()
	ofs2 := newMetricsOverlay(reg2, fstest.MapFS{"c.txt": {Data: []byte("c")}})
	_, _ = ofs2.Open("c.txt")
	_, _ = ofs2.Open("../bad")
	families2, _ := reg2.Gather()
	found := make(map[string]bool)
	for _, f := range families2 {
		found[f.GetName()] = true
	}
	for name := range want {
		if !found[name] {
			t.Errorf("metric %q not registered", name)
		}
	}
}

func TestMetrics_LayerHitCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	theme := fstest.MapFS{"style.css": {Data: []byte("theme-css")}}
	defaults := fstest.MapFS{"base.css": {Data: []byte("default-css")}}
	ofs, err := NewOverlayFS(theme, defaults).
		WithLayerNames([]string{"theme", "defaults"}).
		WithOptions(WithMetrics(reg))
	if err != nil {
		t.Fatal(err)
	}

	// Hit theme layer
	f, err := ofs.Open("style.css")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	v := counterValue(reg, "blogflow_overlay_layer_hit_total", map[string]string{"layer": "theme"})
	if v != 1 {
		t.Errorf("theme layer hit = %v, want 1", v)
	}

	// Hit defaults layer
	_, err = fs.ReadFile(ofs, "base.css")
	if err != nil {
		t.Fatal(err)
	}

	v = counterValue(reg, "blogflow_overlay_layer_hit_total", map[string]string{"layer": "defaults"})
	if v != 1 {
		t.Errorf("defaults layer hit = %v, want 1", v)
	}
}

func TestMetrics_MissCounter(t *testing.T) {
	reg := prometheus.NewRegistry()
	ofs := newMetricsOverlay(reg, fstest.MapFS{})

	_, _ = ofs.Open("nonexistent.txt")

	v := counterValue(reg, "blogflow_overlay_miss_total", nil)
	if v != 1 {
		t.Errorf("miss total = %v, want 1", v)
	}
}

func TestMetrics_HistogramRecordsDurations(t *testing.T) {
	reg := prometheus.NewRegistry()
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	ofs := newMetricsOverlay(reg, layer)

	f, _ := ofs.Open("file.txt")
	_ = f.Close()
	_, _ = fs.ReadFile(ofs, "file.txt")

	openCount := histogramCount(reg, "blogflow_overlay_resolve_duration_seconds", map[string]string{"op": "open"})
	if openCount < 1 {
		t.Errorf("open histogram count = %d, want >= 1", openCount)
	}

	readfileCount := histogramCount(reg, "blogflow_overlay_resolve_duration_seconds", map[string]string{"op": "readfile"})
	if readfileCount < 1 {
		t.Errorf("readfile histogram count = %d, want >= 1", readfileCount)
	}
}

func TestMetrics_StatDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	layer := fstest.MapFS{"file.txt": {Data: []byte("data")}}
	ofs := newMetricsOverlay(reg, layer)

	_, _ = ofs.Stat("file.txt")

	count := histogramCount(reg, "blogflow_overlay_resolve_duration_seconds", map[string]string{"op": "stat"})
	if count != 1 {
		t.Errorf("stat histogram count = %d, want 1", count)
	}
}

func TestMetrics_ReadDirDuration(t *testing.T) {
	reg := prometheus.NewRegistry()
	layer := fstest.MapFS{"dir/a.txt": {Data: []byte("a")}}
	ofs := newMetricsOverlay(reg, layer)

	_, _ = ofs.ReadDir("dir")

	count := histogramCount(reg, "blogflow_overlay_resolve_duration_seconds", map[string]string{"op": "readdir"})
	if count != 1 {
		t.Errorf("readdir histogram count = %d, want 1", count)
	}
}

func TestMetrics_PathRejected(t *testing.T) {
	reg := prometheus.NewRegistry()
	ofs := newMetricsOverlay(reg, fstest.MapFS{})

	_, _ = ofs.Open("../etc/passwd")
	_, _ = ofs.Open("/absolute")
	_, _ = ofs.Open("")

	traversal := counterValue(reg, "blogflow_overlay_path_rejected_total", map[string]string{"reason": "traversal"})
	absolute := counterValue(reg, "blogflow_overlay_path_rejected_total", map[string]string{"reason": "absolute"})
	invalid := counterValue(reg, "blogflow_overlay_path_rejected_total", map[string]string{"reason": "invalid"})

	if traversal < 1 {
		t.Errorf("traversal rejections = %v, want >= 1", traversal)
	}
	if absolute < 1 {
		t.Errorf("absolute rejections = %v, want >= 1", absolute)
	}
	if invalid < 1 {
		t.Errorf("invalid rejections = %v, want >= 1", invalid)
	}
}

func TestMetrics_NegCacheHit(t *testing.T) {
	reg := prometheus.NewRegistry()
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"deep.txt": {Data: []byte("deep")}}
	ofs := newMetricsOverlay(reg, theme, defaults)

	// First read warms the negative cache
	_, _ = fs.ReadFile(ofs, "deep.txt")
	// Second read should hit negative cache
	_, _ = fs.ReadFile(ofs, "deep.txt")

	v := counterValue(reg, "blogflow_overlay_negcache_hit_total", nil)
	if v < 1 {
		t.Errorf("negcache hit = %v, want >= 1", v)
	}
}

func TestMetrics_NegCacheSize(t *testing.T) {
	reg := prometheus.NewRegistry()
	theme := fstest.MapFS{}
	defaults := fstest.MapFS{"f.txt": {Data: []byte("f")}}
	ofs := newMetricsOverlay(reg, theme, defaults)

	_, _ = fs.ReadFile(ofs, "f.txt")

	v := gaugeValue(reg, "blogflow_overlay_negcache_size")
	if v != 1 {
		t.Errorf("negcache size = %v, want 1", v)
	}

	ofs.InvalidateAll()
	v = gaugeValue(reg, "blogflow_overlay_negcache_size")
	if v != 0 {
		t.Errorf("negcache size after invalidate = %v, want 0", v)
	}
}

func TestMetrics_NoPanicWhenDisabled(t *testing.T) {
	// No WithMetrics option — metrics should be nil
	ofs := NewOverlayFS(fstest.MapFS{"a.txt": {Data: []byte("a")}}).
		WithLayerNames([]string{"layer0"})

	// None of these should panic
	f, err := ofs.Open("a.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()

	_, _ = ofs.Open("nonexistent")
	_, _ = ofs.Open("../bad")
	_, _ = fs.ReadFile(ofs, "a.txt")
	_, _ = ofs.ReadDir(".")
	_, _ = ofs.Stat("a.txt")
	ofs.InvalidateAll()
}

func TestMetrics_DuplicateRegistrationReturnsError(t *testing.T) {
	reg := prometheus.NewRegistry()
	layer := fstest.MapFS{"a.txt": {Data: []byte("a")}}

	// First registration succeeds.
	_, err := NewOverlayFS(layer).WithLayerNames([]string{"l0"}).WithOptions(WithMetrics(reg))
	if err != nil {
		t.Fatalf("first registration: %v", err)
	}

	// Second registration with the same registry must return an error, not panic.
	_, err = NewOverlayFS(layer).WithLayerNames([]string{"l1"}).WithOptions(WithMetrics(reg))
	if err == nil {
		t.Fatal("expected error on duplicate metric registration")
	}
}
