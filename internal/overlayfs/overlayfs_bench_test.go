package overlayfs

import (
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"
)

// benchOverlay builds a 4-layer overlay with distinct files per layer
// and shared files that exist in every layer (for shadowing tests).
func benchOverlay(filesPerLayer int) *OverlayFS {
	layers := make([]fs.FS, 4)
	names := []string{"theme", "content", "config", "defaults"}

	for li := range layers {
		m := make(fstest.MapFS, filesPerLayer+1)
		for fi := range filesPerLayer {
			key := fmt.Sprintf("layer%d/file%d.txt", li, fi)
			m[key] = &fstest.MapFile{Data: []byte("data")}
		}
		// shared file present in every layer (top layer wins)
		m["shared.txt"] = &fstest.MapFile{Data: []byte(fmt.Sprintf("layer%d", li))}
		layers[li] = m
	}

	return NewOverlayFS(layers...).WithLayerNames(names)
}

// benchReadDirOverlay builds an overlay where each layer contributes
// unique directory entries so ReadDir must merge across all layers.
func benchReadDirOverlay(entriesPerLayer int) *OverlayFS {
	layers := make([]fs.FS, 4)
	names := []string{"theme", "content", "config", "defaults"}

	for li := range layers {
		m := make(fstest.MapFS, entriesPerLayer)
		for fi := range entriesPerLayer {
			key := fmt.Sprintf("dir/layer%d_file%d.txt", li, fi)
			m[key] = &fstest.MapFile{Data: []byte("x")}
		}
		layers[li] = m
	}

	return NewOverlayFS(layers...).WithLayerNames(names)
}

func BenchmarkOpen_TopLayer(b *testing.B) {
	ofs := benchOverlay(50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f, err := ofs.Open("layer0/file0.txt")
		if err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}

func BenchmarkOpen_BottomLayer(b *testing.B) {
	ofs := benchOverlay(50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f, err := ofs.Open("layer3/file0.txt")
		if err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}

func BenchmarkOpen_NotExist(b *testing.B) {
	ofs := benchOverlay(50)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := ofs.Open("does/not/exist.txt")
		if err == nil {
			b.Fatal("expected error")
		}
	}
}

func BenchmarkOpen_NegCacheHit(b *testing.B) {
	ofs := benchOverlay(50)

	// Warm the negative cache: first Open falls through all layers,
	// subsequent opens skip upper layers via the cached entry.
	f, err := ofs.Open("layer3/file0.txt")
	if err != nil {
		b.Fatal(err)
	}
	_ = f.Close()

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		f, err := ofs.Open("layer3/file0.txt")
		if err != nil {
			b.Fatal(err)
		}
		_ = f.Close()
	}
}

func BenchmarkReadDir_Union(b *testing.B) {
	ofs := benchReadDirOverlay(25)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		_, err := fs.ReadDir(ofs, "dir")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpen_Parallel(b *testing.B) {
	ofs := benchOverlay(50)
	b.SetParallelism(8)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			f, err := ofs.Open("shared.txt")
			if err != nil {
				b.Error(err)
				return
			}
			_ = f.Close()
		}
	})
}
