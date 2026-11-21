package pluginpackage

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
)

// Archive indexes regular files without extracting or executing a package.
// Git hosting providers typically wrap the repository under a single directory.
type Archive struct {
	files map[string]*zip.File
}

// Bound the central directory before archive/zip allocates one File per entry.
// The supported size and entry limits fit the single-disk classic ZIP format.
func validateDirectory(data []byte) error {
	const endSize = 22
	for end := len(data) - endSize; end >= max(0, len(data)-endSize-65535); end-- {
		record := data[end:]
		if string(record[:4]) != "PK\x05\x06" || int(binary.LittleEndian.Uint16(record[20:])) != len(record)-endSize {
			continue
		}
		// archive/zip also accepts EOCD candidates with a shorter comment.
		// An embedded signature must never select a different directory.
		if bytes.Contains(record[4:], []byte("PK\x05\x06")) {
			return fs.ErrInvalid
		}
		count := int(binary.LittleEndian.Uint16(record[10:]))
		if count > 16384 || binary.LittleEndian.Uint32(record[4:]) != 0 || int(binary.LittleEndian.Uint16(record[8:])) != count {
			return fs.ErrInvalid
		}
		offset := int64(binary.LittleEndian.Uint32(record[16:]))
		size := int64(binary.LittleEndian.Uint32(record[12:]))
		if offset+size != int64(end) {
			return fs.ErrInvalid
		}
		dir := data[offset:end]
		for n := 0; n < count; n++ {
			if len(dir) < 46 || string(dir[:4]) != "PK\x01\x02" {
				return fs.ErrInvalid
			}
			length := 46
			for _, start := range []int{28, 30, 32} {
				length += int(binary.LittleEndian.Uint16(dir[start:]))
			}
			if length > len(dir) {
				return fs.ErrInvalid
			}
			dir = dir[length:]
		}
		if len(dir) != 0 {
			return fs.ErrInvalid
		}
		return nil
	}
	return fs.ErrInvalid
}

func OpenArchive(data []byte) (*Archive, error) {
	if err := validateDirectory(data); err != nil {
		return nil, err
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(reader.File) > 16384 {
		return nil, errors.New("PLUGIN_ARCHIVE_LIMIT")
	}
	names := make(map[string]bool)
	directories := make(map[string]string)
	files := make(map[string]*zip.File)
	var total uint64
	for _, file := range reader.File {
		name := strings.TrimSuffix(file.Name, "/")
		key := strings.ToLower(name)
		if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\:\x00") || strings.Count(name, "/") > maxDepth+2 || names[key] {
			return nil, fs.ErrInvalid
		}
		names[key] = true
		if previous, ok := directories[key]; ok && (!file.FileInfo().IsDir() || previous != name) {
			return nil, fs.ErrInvalid
		}
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			lower := strings.ToLower(dir)
			if previous, ok := directories[lower]; ok && previous != dir {
				return nil, fs.ErrInvalid
			}
			if names[lower] && directories[lower] == "" {
				return nil, fs.ErrInvalid
			}
			directories[lower] = dir
		}
		if file.FileInfo().IsDir() {
			directories[key] = name
			continue
		}
		if !file.Mode().IsRegular() || file.UncompressedSize64 > 32<<20 {
			return nil, fs.ErrInvalid
		}
		total += file.UncompressedSize64
		if total > 128<<20 {
			return nil, errors.New("PLUGIN_ARCHIVE_LIMIT")
		}
		files[name] = file
	}
	if len(files) == 0 {
		return nil, fs.ErrInvalid
	}
	return &Archive{files: files}, nil
}

func (a *Archive) Read(name string, limit int64) ([]byte, error) {
	file := a.files[name]
	if file == nil {
		return nil, fs.ErrNotExist
	}
	if file.UncompressedSize64 > uint64(limit) {
		return nil, errors.New("PLUGIN_ARCHIVE_LIMIT")
	}
	r, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errors.New("PLUGIN_ARCHIVE_LIMIT")
	}
	return data, nil
}

func (a *Archive) Wrapper() string {
	var prefix string
	for name := range a.files {
		dir, _, ok := strings.Cut(name, "/")
		if !ok || strings.HasPrefix(dir, ".") {
			return ""
		}
		if prefix != "" && prefix != dir+"/" {
			return ""
		}
		prefix = dir + "/"
	}
	return prefix
}

func (a *Archive) Package(origin, dir string) (*LocalSnapshot, error) {
	snap := &LocalSnapshot{Origin: origin, Files: make(map[string][]byte)}
	prefix := strings.TrimSuffix(dir, "/") + "/"
	if dir == "." {
		prefix = ""
	}
	var total int64
	for name := range a.files {
		rel, ok := strings.CutPrefix(name, prefix)
		if !ok {
			continue
		}
		if len(snap.Files) >= maxEntries || strings.Count(rel, "/") > maxDepth {
			return nil, fs.ErrInvalid
		}
		for _, part := range strings.Split(rel, "/") {
			if strings.EqualFold(part, ".git") {
				return nil, fs.ErrInvalid
			}
		}
		data, err := a.Read(path.Join(dir, rel), min(maxFileBytes, maxPackageBytes-total))
		if err != nil {
			return nil, fmt.Errorf("package archive: %w", err)
		}
		total += int64(len(data))
		snap.Files[rel] = data
	}
	return finishSnapshot(snap)
}
