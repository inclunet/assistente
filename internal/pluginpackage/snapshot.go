package pluginpackage

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// LocalSnapshot contains exactly the bytes inspected for this revision.
type LocalSnapshot struct {
	Origin   string
	Manifest Manifest
	Warnings []string
	Digest   string
	Files    map[string][]byte
}

const (
	maxEntries      = 2048
	maxDepth        = 32
	maxFileBytes    = 8 << 20
	maxPackageBytes = 32 << 20
)

// InspectLocal captures package data without running it. Relative file names
// and lengths are part of the content hash, so moves change the revision.
func InspectLocal(dir string) (*LocalSnapshot, error) {
	return inspectLocal(dir, maxEntries, maxDepth, maxFileBytes, maxPackageBytes)
}

func inspectLocal(dir string, entryLimit, depthLimit int, fileLimit, byteLimit int64) (*LocalSnapshot, error) {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	origin, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS == "windows" {
		origin = ordinaryWindowsPath(origin)
		volume := filepath.VolumeName(origin)
		if strings.HasPrefix(volume, `\\`) && !strings.HasPrefix(volume, `\\?\`) {
			origin = strings.ToLower(volume) + origin[len(volume):]
		}
	}
	root, err := os.OpenRoot(origin)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	snapshot := &LocalSnapshot{Origin: origin, Files: make(map[string][]byte)}
	manifest, err := ReadFile(root, "plugin.json", min(int64(1<<20), byteLimit, fileLimit))
	if err != nil {
		return nil, err
	}
	snapshot.Manifest, snapshot.Warnings, err = decodeManifest(manifest)
	if err != nil {
		return nil, err
	}
	snapshot.Files["plugin.json"] = manifest
	remaining := byteLimit - int64(len(manifest))
	entriesLeft := entryLimit
	foundManifest := false
	var collect func(string, int) error
	collect = func(dir string, depth int) error {
		if depth > depthLimit {
			return fmt.Errorf("package directories exceed depth limit")
		}
		opened, err := root.OpenFile(filepath.FromSlash(dir), os.O_RDONLY|readFlags, 0)
		if err != nil {
			return err
		}
		defer func() { _ = opened.Close() }()
		for {
			entries, err := opened.ReadDir(64)
			if err != nil && err != io.EOF {
				return err
			}
			for _, entry := range entries {
				entriesLeft--
				if entriesLeft < 0 {
					return fmt.Errorf("package exceeds entry limit")
				}
				name := path.Join(dir, entry.Name())
				if strings.EqualFold(name, "plugin.json") {
					if foundManifest {
						return fmt.Errorf("ambiguous plugin.json entries")
					}
					foundManifest = true
					continue
				}
				if strings.EqualFold(entry.Name(), ".git") {
					continue
				}
				if !fs.ValidPath(name) {
					return fs.ErrInvalid
				}
				if entry.IsDir() {
					if err := collect(name, depth+1); err != nil {
						return err
					}
					continue
				}
				data, err := ReadFile(root, name, min(remaining, fileLimit))
				if err != nil {
					return fmt.Errorf("read package file %q: %w", name, err)
				}
				snapshot.Files[name] = data
				remaining -= int64(len(data))
			}
			if err == io.EOF {
				break
			}
		}
		return nil
	}
	if err := collect(".", 0); err != nil {
		return nil, err
	}
	if !foundManifest {
		return nil, fmt.Errorf("plugin.json disappeared during inspection: %w", fs.ErrNotExist)
	}
	names := make([]string, 0, len(snapshot.Files))
	for name := range snapshot.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	h := sha256.New()
	for _, name := range names {
		data := snapshot.Files[name]
		_, _ = h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(name))))
		_, _ = io.WriteString(h, name)
		_, _ = h.Write(binary.BigEndian.AppendUint64(nil, uint64(len(data))))
		_, _ = h.Write(data)
	}
	snapshot.Digest = hex.EncodeToString(h.Sum(nil))
	return snapshot, nil
}

// Prefer the ordinary spelling only when it names the same existing directory.
// Some extended Windows paths (for example names ending in a dot) need their prefix.
func ordinaryWindowsPath(origin string) string {
	const extended = `\\?\`
	if !strings.HasPrefix(origin, extended) {
		return origin
	}
	name := strings.TrimPrefix(origin, extended)
	switch {
	case len(name) >= 4 && strings.EqualFold(name[:4], `UNC\`):
		name = `\\` + name[4:]
	case len(name) >= 3 && name[1] == ':' && name[2] == '\\':
	default:
		return origin
	}
	ordinary, err := filepath.EvalSymlinks(name)
	if err != nil {
		return origin
	}
	current, err := os.Stat(origin)
	if err != nil {
		return origin
	}
	other, err := os.Stat(ordinary)
	if err != nil || !os.SameFile(current, other) {
		return origin
	}
	return ordinary
}
