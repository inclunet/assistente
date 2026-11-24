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

	"sort"
	"strings"
)

// LocalSnapshot contains exactly the bytes inspected for this revision.
type LocalSnapshot struct {
	DiscoveryAuthentication     string
	DiscoveryRepositoryRevision string
	DiscoveryRepositoryID       string
	Origin                      string
	Manifest                    Manifest
	Warnings                    []string
	Digest                      string
	Files                       map[string][]byte
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
	root, err := os.OpenRoot(absolute)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return inspectOpened(root, absolute, entryLimit, depthLimit, fileLimit, byteLimit)
}

// InspectRepositoryPackage anchors both the package root and its members in
// the selected marketplace. Reopening an unchecked joined path would allow a
// concurrent directory replacement between preview and installation.
func InspectRepositoryPackage(directory, expectedRoot, relative string) (*LocalSnapshot, error) {
	if !fs.ValidPath(relative) || relative == "." {
		return nil, fs.ErrInvalid
	}
	local, err := filepath.Localize(relative)
	if err != nil {
		return nil, err
	}
	repo, err := os.OpenRoot(directory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = repo.Close() }()
	identity, err := RootIdentity(repo)
	if err != nil {
		return nil, err
	}
	if identity != expectedRoot {
		return nil, fs.ErrInvalid
	}
	root, err := repo.OpenRoot(local)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return inspectOpened(root, filepath.Join(directory, local), maxEntries, maxDepth, maxFileBytes, maxPackageBytes)
}

func inspectOpened(root *os.Root, absolute string, entryLimit, depthLimit int, fileLimit, byteLimit int64) (*LocalSnapshot, error) {
	snapshot := &LocalSnapshot{Files: make(map[string][]byte)}
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
		if depth == 0 {
			snapshot.Origin, err = packageOrigin(opened, absolute)
			if err != nil {
				return err
			}
			current, err := opened.Stat()
			if err != nil {
				return err
			}
			location, err := os.Stat(snapshot.Origin)
			if err != nil {
				return err
			}
			if !os.SameFile(current, location) {
				return fmt.Errorf("package directory changed during inspection")
			}
		}
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

func RootIdentity(root *os.Root) (string, error) {
	file, err := root.Open(".")
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	return directoryID(file)
}
