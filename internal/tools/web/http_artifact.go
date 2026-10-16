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
	mu     sync.Mutex
	dir    string
	owned  map[string]os.FileInfo
	timers map[string]*time.Timer
	closed bool
	root   *os.Root
}

type httpArtifact struct {
	Path      string
	Size      int64
	SHA256    string
	Truncated bool
}

func newHTTPArtifactStore() *httpArtifactStore {
	return &httpArtifactStore{dir: filepath.Join(os.TempDir(), httpArtifactDirName, uuid.NewString()), owned: make(map[string]os.FileInfo), timers: make(map[string]*time.Timer)}
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
	defer s.mu.Unlock()
	if s.closed || s.root != nil {
		return fmt.Errorf("diretório de artefatos não pode ser reconfigurado após uso")
	}
	root, err := os.OpenRoot(real)
	if err != nil {
		return err
	}
	s.root = root
	s.dir = real
	return nil
}

// Cada operação recebe um handle independente; Cleanup pode encerrar o store
// enquanto um download cancelado ainda está removendo seu arquivo parcial.
func (s *httpArtifactStore) openRoot() (*os.Root, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, "", fmt.Errorf("armazenamento de artefatos encerrado")
	}
	if s.root == nil {
		if err := os.MkdirAll(s.dir, 0o700); err != nil {
			return nil, "", err
		}
		dir, err := filepath.EvalSymlinks(s.dir)
		if err != nil {
			return nil, "", err
		}
		root, err := os.OpenRoot(dir)
		if err != nil {
			return nil, "", err
		}
		s.root, s.dir = root, dir
	}
	root, err := s.root.OpenRoot(".")
	return root, s.dir, err
}

func (s *httpArtifactStore) resolveOutputPath(requested string) (string, error) {
	root, dir, err := s.openRoot()
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	if err := validateArtifactRoot(root, dir); err != nil {
		return "", err
	}

	requested = strings.TrimSpace(requested)
	if len(requested) > 1024 || strings.ContainsAny(requested, "\x00") {
		return "", fmt.Errorf("output_path inválido")
	}
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
		if strings.ContainsAny(requested, `/\:`) || filepath.Base(cleaned) != cleaned || cleaned == "." || cleaned == ".." {
			return "", fmt.Errorf("output_path deve ser um nome de arquivo dentro do diretório de artefatos HTTP")
		}
		path = filepath.Join(dir, cleaned)
	}
	path = filepath.Clean(path)
	if strings.ContainsAny(filepath.Base(path), `\:`) {
		return "", fmt.Errorf("output_path inválido")
	}
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
	if info, err := root.Lstat(filepath.Base(path)); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("output_path não pode ser um link simbólico")
		}
		return "", fmt.Errorf("o arquivo indicado por output_path já existe")
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("não foi possível validar output_path: %w", err)
	}
	return path, nil
}

func validateArtifactRoot(root *os.Root, dir string) error {
	original, err := root.Stat(".")
	if err != nil {
		return err
	}
	current, err := os.Stat(dir)
	if err != nil {
		return err
	}
	if !os.SameFile(original, current) {
		return fmt.Errorf("diretório de artefatos HTTP foi substituído")
	}
	return nil
}

func (s *httpArtifactStore) writeResponse(ctx context.Context, body io.Reader, requested string, maxBytes int64) (httpArtifact, error) {
	path, err := s.resolveOutputPath(requested)
	if err != nil {
		return httpArtifact{}, err
	}
	root, dir, err := s.openRoot()
	if err != nil {
		return httpArtifact{}, err
	}
	defer func() { _ = root.Close() }()
	// O_EXCL impede sobrescrita e corridas entre downloads para o mesmo nome.
	name := filepath.Base(path)
	tmp, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return httpArtifact{}, fmt.Errorf("não foi possível criar o arquivo temporário do artefato HTTP: %w", err)
	}
	original, statErr := tmp.Stat()
	removeTemp := true
	defer func() {
		_ = tmp.Close()
		if removeTemp {
			if original != nil {
				_ = removeArtifactFile(root, name, original)
			}
		}
	}()
	if statErr != nil {
		return httpArtifact{}, statErr
	}

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
	info, err := tmp.Stat()
	if err != nil {
		return httpArtifact{}, err
	}
	if err := tmp.Close(); err != nil {
		return httpArtifact{}, fmt.Errorf("não foi possível finalizar o artefato HTTP: %w", err)
	}
	current, err := root.Lstat(name)
	if err != nil || !os.SameFile(info, current) {
		return httpArtifact{}, fmt.Errorf("arquivo de artefato HTTP foi substituído durante o download")
	}
	if err := validateArtifactRoot(root, dir); err != nil {
		return httpArtifact{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return httpArtifact{}, fmt.Errorf("armazenamento de artefatos encerrado")
	}
	s.owned[path] = info
	s.timers[path] = time.AfterFunc(httpArtifactTTL, func() { _ = s.cleanupExpired(time.Now()) })
	removeTemp = false
	return httpArtifact{Path: path, Size: bytes, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *httpArtifactStore) cleanupExpired(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := now.Add(-httpArtifactTTL)
	for path, info := range s.owned {
		if !info.ModTime().After(cutoff) {
			if err := s.removeOwned(path, info); err != nil {
				return err
			}
		}
	}
	return nil
}

// Cleanup remove somente arquivos criados por esta instância, nunca pastas
// ou arquivos preexistentes/pertencentes a outra instância.
func (s *httpArtifactStore) Cleanup() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var firstErr error
	for path, info := range s.owned {
		if err := s.removeOwned(path, info); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil && s.root != nil {
		firstErr = s.root.Close()
		s.root = nil
	}
	return firstErr
}

// Chamado com mu adquirido. Não remove um arquivo substituído externamente.
func (s *httpArtifactStore) removeOwned(path string, original os.FileInfo) error {
	if err := removeArtifactFile(s.root, filepath.Base(path), original); err != nil {
		return err
	}
	if timer := s.timers[path]; timer != nil {
		timer.Stop()
	}
	delete(s.timers, path)
	delete(s.owned, path)
	return nil
}

func removeArtifactFile(root *os.Root, name string, original os.FileInfo) error {
	current, err := root.Lstat(name)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && os.SameFile(original, current) {
		if err := root.Remove(name); err != nil {
			return err
		}
	}
	return nil
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
		if value := header.Get(name); value != "" && len(value) <= 128 {
			result[name] = value
		}
	}
	return result
}
