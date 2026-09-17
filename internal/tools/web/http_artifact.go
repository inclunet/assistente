package web

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	httpArtifactDirName = ".assistente-http-artifacts"
	httpArtifactTTL     = 30 * time.Minute
)

var errHTTPArtifactTooLarge = errors.New("resposta excede o limite seguro de download")

type httpArtifactStore struct {
	mu  sync.Mutex
	dir string
}

type httpArtifact struct {
	Path      string
	Size      int64
	SHA256    string
	Truncated bool
}

func newHTTPArtifactStore() *httpArtifactStore {
	return &httpArtifactStore{dir: filepath.Join(os.TempDir(), httpArtifactDirName)}
}

// SetDir configura a raiz exclusiva dos artefatos HTTP. output_path nunca
// pode apontar para fora desta raiz nem para uma subpasta.
func (s *httpArtifactStore) SetDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return fmt.Errorf("diretório de artefatos HTTP vazio")
	}
	abs, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return fmt.Errorf("diretório de artefatos HTTP inválido: %w", err)
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return fmt.Errorf("não foi possível criar o diretório de artefatos HTTP: %w", err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return fmt.Errorf("não foi possível resolver o diretório de artefatos HTTP: %w", err)
	}
	s.mu.Lock()
	s.dir = real
	s.mu.Unlock()
	return s.cleanupExpired(time.Now())
}

func (s *httpArtifactStore) resolveOutputPath(requested string) (string, error) {
	s.mu.Lock()
	dir := s.dir
	s.mu.Unlock()
	if dir == "" {
		return "", fmt.Errorf("diretório de artefatos HTTP não configurado")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("não foi possível criar o diretório de artefatos HTTP: %w", err)
	}

	requested = strings.TrimSpace(requested)
	if requested == "" {
		return filepath.Join(dir, "http-response-"+uuid.NewString()+".bin"), nil
	}

	cleaned := filepath.Clean(requested)
	var path string
	if filepath.IsAbs(cleaned) {
		path = cleaned
	} else {
		// Somente um nome simples é aceito. Isso rejeita ../, .\ e subpastas
		// antes de qualquer criação de diretório controlada pelo modelo.
		if filepath.Base(cleaned) != cleaned || cleaned == "." || cleaned == ".." {
			return "", fmt.Errorf("output_path deve ser um nome de arquivo dentro do diretório de artefatos HTTP")
		}
		path = filepath.Join(dir, cleaned)
	}
	path = filepath.Clean(path)
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("não foi possível validar output_path: %w", err)
	}
	if parent != filepath.Clean(dir) {
		return "", fmt.Errorf("output_path deve permanecer dentro do diretório de artefatos HTTP")
	}
	// Canonicaliza a representação da raiz (inclusive a forma 8.3 do Windows)
	// antes de abrir o arquivo, sem aceitar uma pasta real fora da raiz.
	path = filepath.Join(dir, filepath.Base(path))
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output_path não pode ser um link simbólico")
		}
		return "", fmt.Errorf("o arquivo indicado por output_path já existe")
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("não foi possível validar output_path: %w", err)
	}
	return path, nil
}

func (s *httpArtifactStore) writeResponse(ctx context.Context, body io.Reader, requested string, maxBytes int64) (httpArtifact, error) {
	path, err := s.resolveOutputPath(requested)
	if err != nil {
		return httpArtifact{}, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".http-response-*.tmp")
	if err != nil {
		return httpArtifact{}, fmt.Errorf("não foi possível criar o arquivo temporário do artefato HTTP: %w", err)
	}
	tmpName := tmp.Name()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			_ = os.Remove(tmpName)
		}
	}()

	hash := sha256.New()
	writer := io.MultiWriter(tmp, hash)
	bytes, copyErr := io.CopyBuffer(writer, io.LimitReader(body, maxBytes+1), make([]byte, 32*1024))
	if copyErr != nil {
		return httpArtifact{}, fmt.Errorf("falha ao baixar resposta HTTP: %w", copyErr)
	}
	if err := ctx.Err(); err != nil {
		return httpArtifact{}, err
	}
	if bytes > maxBytes {
		return httpArtifact{Size: bytes, Truncated: true}, errHTTPArtifactTooLarge
	}
	if err := tmp.Close(); err != nil {
		return httpArtifact{}, fmt.Errorf("não foi possível finalizar o artefato HTTP: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return httpArtifact{}, fmt.Errorf("não foi possível materializar o artefato HTTP: %w", err)
	}
	removeTemp = false
	return httpArtifact{Path: path, Size: bytes, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *httpArtifactStore) cleanupExpired(now time.Time) error {
	s.mu.Lock()
	dir := s.dir
	s.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	cutoff := now.Add(-httpArtifactTTL)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
	return nil
}

// Cleanup remove o conteúdo da pasta exclusiva de artefatos. O diretório em
// si permanece para que uma próxima execução não precise recriá-lo.
func (s *httpArtifactStore) Cleanup() error {
	s.mu.Lock()
	dir := s.dir
	s.mu.Unlock()
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var firstErr error
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dir, entry.Name())); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func artifactSummaryContent(status int, statusText, contentType string, headers map[string]string, artifact httpArtifact) string {
	value := map[string]any{
		"status":       status,
		"status_text":  statusText,
		"content_type": contentType,
		"headers":      headers,
		"size_bytes":   artifact.Size,
		"path":         artifact.Path,
		"sha256":       artifact.SHA256,
		"truncated":    artifact.Truncated,
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("Artefato HTTP materializado em %s (%d bytes)", artifact.Path, artifact.Size)
	}
	return string(encoded)
}

func relevantArtifactHeaders(header http.Header) map[string]string {
	result := make(map[string]string)
	for _, name := range []string{"Content-Type", "Content-Encoding", "Content-Language", "ETag", "Last-Modified"} {
		if value := header.Get(name); value != "" {
			result[name] = value
		}
	}
	return result
}
