// Package overlayfs provides a layered filesystem where higher-priority
// layers shadow lower ones. It implements io/fs.FS, fs.ReadFileFS,
// fs.ReadDirFS, and fs.StatFS.
//
// BlogFlow uses this to enable progressive customization: the binary
// ships with embedded defaults, and users can override any file by
// placing it in an external directory.
//
// Layer resolution order (highest priority first):
//  1. Theme layer   — /data/theme/   (custom theme templates, CSS)
//  2. Content layer — /data/content/ (markdown posts, pages, media)
//  3. Config layer  — /data/config/  (site.yaml, navigation.yaml)
//  4. Defaults layer — embed.FS      (compiled-in defaults)
package overlayfs

import (
	"embed"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// layerMeta stores resolved root paths for disk-backed layers.
// Used for symlink escape detection on non-Linux platforms.
type layerMeta struct {
	rootPath string // empty for non-disk layers (embed.FS, MapFS)
	isDisk   bool
}

// OverlayFS is a layered filesystem where higher-priority layers
// shadow lower ones. It implements fs.FS, fs.StatFS, fs.ReadFileFS,
// and fs.ReadDirFS.
type OverlayFS struct {
	layers     []fs.FS
	layerNames []string
	layerMeta  []layerMeta

	mu sync.RWMutex // protects layers slice during hot-reload

	// Negative cache: tracks paths confirmed absent from upper layers.
	negCache      sync.Map // map[string]negCacheEntry
	negCacheCount atomic.Int64

	// maxNegCacheEntries bounds the negative cache size. When exceeded,
	// new entries are not cached (graceful degradation).
	maxNegCacheEntries int
}

type negCacheEntry struct {
	// firstCandidateLayer is the index of the first layer that may
	// contain this path. Layers [0, firstCandidateLayer) are known
	// misses and skipped on subsequent lookups.
	firstCandidateLayer int
}

// resolution describes which layer served a file.
type resolution struct {
	Path       string
	LayerIndex int
	LayerName  string
}

// NewOverlayFS creates a new overlay with the given layers.
// Layers are in priority order: layers[0] is checked first.
// Nil layers are silently skipped.
func NewOverlayFS(layers []fs.FS, names []string) *OverlayFS {
	var filtered []fs.FS
	var filteredNames []string
	for i, l := range layers {
		if l != nil {
			filtered = append(filtered, l)
			if i < len(names) {
				filteredNames = append(filteredNames, names[i])
			} else {
				filteredNames = append(filteredNames, fmt.Sprintf("layer-%d", i))
			}
		}
	}
	return &OverlayFS{
		layers:             filtered,
		layerNames:         filteredNames,
		layerMeta:          make([]layerMeta, len(filtered)),
		maxNegCacheEntries: 100_000,
	}
}

// NewFromPaths constructs the standard 4-layer BlogFlow overlay.
// Empty path strings cause that layer to be omitted.
// The defaults embed.FS is always included as the lowest layer.
// The defaults parameter should already have its prefix stripped via fs.Sub.
func NewFromPaths(themePath, contentPath, configPath string, defaults fs.FS) (*OverlayFS, error) {
	if !goVersionAtLeast(1, 22) {
		panic(fmt.Sprintf("blogflow: Go 1.22+ required for os.DirFS symlink hardening, got %s", runtime.Version()))
	}

	var layers []fs.FS
	var names []string
	var resolvedPaths []string

	paths := []struct {
		path string
		name string
	}{
		{themePath, "theme"},
		{contentPath, "content"},
		{configPath, "config"},
	}

	for _, p := range paths {
		if p.path == "" {
			continue
		}
		resolved, err := filepath.EvalSymlinks(p.path)
		if err != nil {
			return nil, fmt.Errorf("overlayfs: resolve %s path %q: %w", p.name, p.path, err)
		}
		info, err := os.Stat(resolved)
		if err != nil {
			return nil, fmt.Errorf("overlayfs: stat %s path %q: %w", p.name, resolved, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("overlayfs: %s path %q is not a directory", p.name, resolved)
		}
		layers = append(layers, os.DirFS(resolved))
		names = append(names, p.name)
		resolvedPaths = append(resolvedPaths, resolved)
	}

	if defaults != nil {
		layers = append(layers, defaults)
		names = append(names, "defaults")
	}

	ofs := NewOverlayFS(layers, names)
	for i, rp := range resolvedPaths {
		if i < len(ofs.layerMeta) {
			ofs.layerMeta[i] = layerMeta{rootPath: rp, isDisk: true}
		}
	}
	return ofs, nil
}

// NewFromEmbed is a convenience constructor for embedding defaults from an embed.FS.
// It strips the given prefix using fs.Sub.
func NewFromEmbed(defaults embed.FS, prefix string) (*OverlayFS, error) {
	sub, err := fs.Sub(defaults, prefix)
	if err != nil {
		return nil, fmt.Errorf("overlayfs: fs.Sub(%q): %w", prefix, err)
	}
	return NewOverlayFS([]fs.FS{sub}, []string{"defaults"}), nil
}

// Open implements fs.FS. Returns the file from the highest-priority
// layer that contains it, or fs.ErrNotExist if no layer has it.
// Only fs.ErrNotExist triggers fallthrough; other errors (EACCES, EIO)
// are returned immediately.
func (o *OverlayFS) Open(name string) (fs.File, error) {
	// TODO(security): ContextOverlayFS will log WARN with request_id and remote_addr
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}

	o.mu.RLock()
	layers := make([]fs.FS, len(o.layers))
	copy(layers, o.layers)
	o.mu.RUnlock()

	startLayer := 0
	if cached, ok := o.negCache.Load(name); ok {
		entry := cached.(negCacheEntry)
		if entry.firstCandidateLayer < len(layers) {
			startLayer = entry.firstCandidateLayer
		}
	}

	for i := startLayer; i < len(layers); i++ {
		f, err := layers[i].Open(name)
		if err == nil {
			// S1: Check symlink escape for disk layers
			if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
				if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
					_ = f.Close()
					return nil, symlinkErr
				}
			}
			// Cache: record that layers [0, i) don't have this path
			if i > 0 && o.negCacheCount.Load() < int64(o.maxNegCacheEntries) {
				if _, loaded := o.negCache.LoadOrStore(name, negCacheEntry{
					firstCandidateLayer: i,
				}); !loaded {
					o.negCacheCount.Add(1)
				}
			}
			return f, nil
		}
		if !isNotExist(err) {
			return nil, err // EACCES, EIO — propagate immediately
		}
	}

	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadFile implements fs.ReadFileFS. Reads the entire file from the
// highest-priority layer that contains it.
func (o *OverlayFS) ReadFile(name string) ([]byte, error) {
	// TODO(security): ContextOverlayFS will log WARN with request_id and remote_addr
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrInvalid}
	}

	o.mu.RLock()
	layers := make([]fs.FS, len(o.layers))
	copy(layers, o.layers)
	o.mu.RUnlock()

	startLayer := 0
	if cached, ok := o.negCache.Load(name); ok {
		entry := cached.(negCacheEntry)
		if entry.firstCandidateLayer < len(layers) {
			startLayer = entry.firstCandidateLayer
		}
	}

	for i := startLayer; i < len(layers); i++ {
		if rfs, ok := layers[i].(fs.ReadFileFS); ok {
			// S1: Check symlink escape for disk layers before reading
			if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
				if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
					return nil, symlinkErr
				}
			}
			data, err := rfs.ReadFile(name)
			if err == nil {
				if len(data) > maxReadSize {
					return nil, fmt.Errorf("overlayfs: file %q exceeds maximum read size of %d bytes", name, maxReadSize)
				}
				if i > 0 && o.negCacheCount.Load() < int64(o.maxNegCacheEntries) {
					if _, loaded := o.negCache.LoadOrStore(name, negCacheEntry{
						firstCandidateLayer: i,
					}); !loaded {
						o.negCacheCount.Add(1)
					}
				}
				return data, nil
			}
			if !isNotExist(err) {
				return nil, err
			}
		} else {
			// Fallback: open and read
			f, err := layers[i].Open(name)
			if err == nil {
				// S1: Check symlink escape for disk layers
				if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
					if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
						_ = f.Close()
						return nil, symlinkErr
					}
				}
				data, readErr := readAll(f)
				_ = f.Close()
				if readErr != nil {
					return nil, readErr
				}
				if i > 0 && o.negCacheCount.Load() < int64(o.maxNegCacheEntries) {
					if _, loaded := o.negCache.LoadOrStore(name, negCacheEntry{
						firstCandidateLayer: i,
					}); !loaded {
						o.negCacheCount.Add(1)
					}
				}
				return data, nil
			}
			if !isNotExist(err) {
				return nil, err
			}
		}
	}

	return nil, &fs.PathError{Op: "readfile", Path: name, Err: fs.ErrNotExist}
}

// ReadDir implements fs.ReadDirFS. Returns the UNION of directory
// entries across all layers. For duplicate names, the entry from the
// highest-priority layer wins. Entries are sorted by name.
func (o *OverlayFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// TODO(security): ContextOverlayFS will log WARN with request_id and remote_addr
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}

	o.mu.RLock()
	layers := make([]fs.FS, len(o.layers))
	copy(layers, o.layers)
	o.mu.RUnlock()

	merged := make(map[string]fs.DirEntry)
	found := false

	for i := len(layers) - 1; i >= 0; i-- {
		entries, err := fs.ReadDir(layers[i], name)
		if err != nil {
			if isNotExist(err) {
				continue
			}
			return nil, err // EACCES, EIO — propagate
		}
		found = true
		for _, e := range entries {
			merged[e.Name()] = e // higher-priority layers overwrite
		}
	}

	if !found {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}

	result := make([]fs.DirEntry, 0, len(merged))
	for _, e := range merged {
		result = append(result, e)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result, nil
}

// Stat implements fs.StatFS. Returns file info from the highest-priority
// layer that contains the path.
func (o *OverlayFS) Stat(name string) (fs.FileInfo, error) {
	// TODO(security): ContextOverlayFS will log WARN with request_id and remote_addr
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}

	o.mu.RLock()
	layers := make([]fs.FS, len(o.layers))
	copy(layers, o.layers)
	o.mu.RUnlock()

	startLayer := 0
	if cached, ok := o.negCache.Load(name); ok {
		entry := cached.(negCacheEntry)
		if entry.firstCandidateLayer < len(layers) {
			startLayer = entry.firstCandidateLayer
		}
	}

	for i := startLayer; i < len(layers); i++ {
		if sfs, ok := layers[i].(fs.StatFS); ok {
			// S1: Check symlink escape for disk layers before stat
			if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
				if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
					return nil, symlinkErr
				}
			}
			info, err := sfs.Stat(name)
			if err == nil {
				return info, nil
			}
			if !isNotExist(err) {
				return nil, err
			}
		} else {
			// Fallback: open and stat
			f, err := layers[i].Open(name)
			if err == nil {
				// S1: Check symlink escape for disk layers
				if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
					if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
						_ = f.Close()
						return nil, symlinkErr
					}
				}
				info, statErr := f.Stat()
				_ = f.Close()
				if statErr == nil {
					return info, nil
				}
				return nil, statErr
			}
			if !isNotExist(err) {
				return nil, err
			}
		}
	}

	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// OpenFile opens a file and returns both the handle and its FileInfo
// from the same layer resolution. Prevents TOCTOU races between
// separate Stat() and Open() calls during concurrent layer updates.
func (o *OverlayFS) OpenFile(name string) (fs.File, fs.FileInfo, error) {
	f, err := o.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, info, nil
}

// InvalidateLayer clears negative cache entries for a specific layer.
// Called when files within a layer change (e.g., file edit, new file).
func (o *OverlayFS) InvalidateLayer(layerIndex int) {
	o.negCache.Range(func(key, value any) bool {
		entry := value.(negCacheEntry)
		if entry.firstCandidateLayer >= layerIndex {
			if _, deleted := o.negCache.LoadAndDelete(key); deleted {
				o.negCacheCount.Add(-1)
			}
		}
		return true
	})
}

// InvalidateAll clears the entire negative cache.
func (o *OverlayFS) InvalidateAll() {
	o.negCache.Range(func(key, _ any) bool {
		if _, deleted := o.negCache.LoadAndDelete(key); deleted {
			o.negCacheCount.Add(-1)
		}
		return true
	})
}

// ReplaceLayer atomically replaces a layer's backing fs.FS and clears
// its negative cache entries. Used after git-sync symlink swaps where
// the base directory path has changed.
func (o *OverlayFS) ReplaceLayer(layerIndex int, newFS fs.FS) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if layerIndex < 0 || layerIndex >= len(o.layers) {
		return fmt.Errorf("overlayfs: layer index %d out of range [0, %d)", layerIndex, len(o.layers))
	}
	o.layers[layerIndex] = newFS
	// Clear neg-cache entries that reference this or higher layers
	o.negCache.Range(func(key, value any) bool {
		entry := value.(negCacheEntry)
		if entry.firstCandidateLayer >= layerIndex {
			if _, deleted := o.negCache.LoadAndDelete(key); deleted {
				o.negCacheCount.Add(-1)
			}
		}
		return true
	})
	return nil
}

// LayerCount returns the number of active layers.
func (o *OverlayFS) LayerCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return len(o.layers)
}

// resolveInfo returns metadata about where a path resolves to.
// Internal only — must NOT appear in HTTP responses.
func (o *OverlayFS) resolveInfo(name string) (*resolution, error) {
	// TODO(security): ContextOverlayFS will log WARN with request_id and remote_addr
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "resolve", Path: name, Err: fs.ErrInvalid}
	}

	o.mu.RLock()
	layers := make([]fs.FS, len(o.layers))
	copy(layers, o.layers)
	names := make([]string, len(o.layerNames))
	copy(names, o.layerNames)
	o.mu.RUnlock()

	for i := 0; i < len(layers); i++ {
		f, err := layers[i].Open(name)
		if err == nil {
			// S1: Check symlink escape for disk layers
			if i < len(o.layerMeta) && o.layerMeta[i].isDisk {
				if symlinkErr := checkSymlinkSafe(o.layerMeta[i].rootPath, name); symlinkErr != nil {
					_ = f.Close()
					return nil, symlinkErr
				}
			}
			_ = f.Close()
			layerName := fmt.Sprintf("layer-%d", i)
			if i < len(names) {
				layerName = names[i]
			}
			return &resolution{
				Path:       name,
				LayerIndex: i,
				LayerName:  layerName,
			}, nil
		}
		if !isNotExist(err) {
			return nil, err
		}
	}
	return nil, &fs.PathError{Op: "resolve", Path: name, Err: fs.ErrNotExist}
}

// isNotExist checks if an error indicates the file does not exist.
func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}

// maxReadSize is the maximum file size readAll will read.
// Files exceeding this limit return an error.
const maxReadSize = 64 * 1024 * 1024 // 64 MiB

// readAll reads all bytes from an fs.File, bounded by maxReadSize.
func readAll(f fs.File) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(f, maxReadSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxReadSize {
		return nil, fmt.Errorf("overlayfs: file exceeds maximum read size of %d bytes", maxReadSize)
	}
	return data, nil
}

// checkSymlinkSafe verifies the opened path hasn't escaped the layer root
// via symlink. This is defense-in-depth for platforms without openat2/RESOLVE_BENEATH.
func checkSymlinkSafe(root, name string) error {
	if root == "" {
		return nil // non-disk layer, skip check
	}
	fullPath := filepath.Join(root, filepath.FromSlash(name))
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return err
	}
	// Verify resolved path is still under root
	if !strings.HasPrefix(resolved, root+string(filepath.Separator)) && resolved != root {
		return &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	return nil
}

// goVersionAtLeast checks if the runtime Go version is at least major.minor.
func goVersionAtLeast(major, minor int) bool {
	v := runtime.Version()
	v = strings.TrimPrefix(v, "go")
	// Handle versions like "1.22.1" or "1.22"
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return false
	}
	var maj, min int
	if _, err := fmt.Sscan(parts[0], &maj); err != nil {
		return false
	}
	if _, err := fmt.Sscan(parts[1], &min); err != nil {
		return false
	}
	return maj > major || (maj == major && min >= minor)
}

// Compile-time interface checks.
var (
	_ fs.FS         = (*OverlayFS)(nil)
	_ fs.ReadFileFS = (*OverlayFS)(nil)
	_ fs.ReadDirFS  = (*OverlayFS)(nil)
	_ fs.StatFS     = (*OverlayFS)(nil)
)
