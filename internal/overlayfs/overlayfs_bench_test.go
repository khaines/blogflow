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

func BenchmarkNegCacheHit_Parallel(b *testing.B) {
	ofs := NewOverlayFS(
		fstest.MapFS{},
		fstest.MapFS{"hot/path.txt": {Data: []byte("data")}},
	).WithLayerNames([]string{"theme", "defaults"})

	if _, err := ofs.ReadFile("hot/path.txt"); err != nil {
		b.Fatal(err)
	}

	b.Run("ReadFile", func(b *testing.B) {
		b.SetParallelism(8)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				data, err := ofs.ReadFile("hot/path.txt")
				if err != nil {
					b.Error(err)
					return
				}
				if len(data) == 0 {
					b.Error("ReadFile returned empty data")
					return
				}
			}
		})
	})

	b.Run("Stat", func(b *testing.B) {
		b.SetParallelism(8)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				if _, err := ofs.Stat("hot/path.txt"); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
}

func BenchmarkNegCacheHitCrossShard_Parallel(b *testing.B) {
	const keysPerShard = 16

	names := benchCrossShardNames(keysPerShard)
	upper := fstest.MapFS{}
	lower := make(fstest.MapFS, len(names))
	for _, name := range names {
		lower[name] = &fstest.MapFile{Data: []byte("data")}
	}
	ofs := NewOverlayFS(upper, lower).WithLayerNames([]string{"theme", "defaults"})

	for _, name := range names {
		if _, err := ofs.ReadFile(name); err != nil {
			b.Fatalf("warm ReadFile(%q): %v", name, err)
		}
	}

	b.Run("ReadFile", func(b *testing.B) {
		offsets := benchWorkerOffsets(len(names))
		b.SetParallelism(8)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := <-offsets
			for pb.Next() {
				name := names[i%len(names)]
				i++
				data, err := ofs.ReadFile(name)
				if err != nil {
					b.Error(err)
					return
				}
				if len(data) == 0 {
					b.Error("ReadFile returned empty data")
					return
				}
			}
		})
	})

	b.Run("Stat", func(b *testing.B) {
		offsets := benchWorkerOffsets(len(names))
		b.SetParallelism(8)
		b.ReportAllocs()
		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := <-offsets
			for pb.Next() {
				name := names[i%len(names)]
				i++
				if _, err := ofs.Stat(name); err != nil {
					b.Error(err)
					return
				}
			}
		})
	})
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

func benchCrossShardNames(keysPerShard int) []string {
	namesByShard := make([][]string, negCacheShardCount)
	for i := 0; i < 100_000; i++ {
		name := fmt.Sprintf("cross-shard/file%05d.txt", i)
		shard := negCacheShardIndex(name)
		if len(namesByShard[shard]) < keysPerShard {
			namesByShard[shard] = append(namesByShard[shard], name)
		}
	}

	names := make([]string, 0, negCacheShardCount*keysPerShard)
	for shard := range namesByShard {
		if len(namesByShard[shard]) != keysPerShard {
			panic("could not generate enough cross-shard benchmark names")
		}
	}
	for key := 0; key < keysPerShard; key++ {
		for shard := range namesByShard {
			names = append(names, namesByShard[shard][key])
		}
	}
	return names
}

func benchWorkerOffsets(nameCount int) <-chan int {
	const maxWorkers = 4096

	offsets := make(chan int, maxWorkers)
	for i := 0; i < maxWorkers; i++ {
		offsets <- i % nameCount
	}
	return offsets
}
