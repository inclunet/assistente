package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/inconshreveable/go-update"
)

const (
	// ManifestURL é o URL do manifest no GitHub Pages
	ManifestURL = "https://inclunet.github.io/assistente/update-manifest.json"
	
	// CheckInterval é o intervalo padrão para verificar atualizações
	CheckInterval = 6 * time.Hour

	// GitHubAPIURL é a base da API do GitHub para releases
	GitHubAPIURL = "https://api.github.com/repos/inclunet/assistente/releases/latest"
)

// Manifest representa o arquivo de metadados de versões
type Manifest struct {
	Version  string            `json:"version"`
	Released string            `json:"released"`
	Notes    string            `json:"notes,omitempty"`
	Builds   map[string]Build  `json:"builds"`
}

// Build representa informações de um build específico
type Build struct {
	URL      string `json:"url"`
	Checksum string `json:"checksum"` // formato: "sha256:hash"
	Size     int64  `json:"size"`
}

// UpdateInfo contém informações sobre uma atualização disponível
type UpdateInfo struct {
	Available      bool   `json:"available"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	ReleaseNotes   string `json:"releaseNotes,omitempty"`
	ReleaseDate    string `json:"releaseDate,omitempty"`
	DownloadSize   int64  `json:"downloadSize,omitempty"`
}

// Updater gerencia verificação e aplicação de atualizações
type Updater struct {
	currentVersion string
	manifestURL    string
	githubToken    string // Token para acessar releases privadas (opcional)
	httpClient     *http.Client
}

// New cria um novo Updater
func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		manifestURL:    ManifestURL,
		githubToken:    "", // Pode ser configurado depois se repo for privado
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// SetGitHubToken configura token para acessar releases privadas
func (u *Updater) SetGitHubToken(token string) {
	u.githubToken = token
}

// CheckForUpdates verifica se há uma nova versão disponível
func (u *Updater) CheckForUpdates(ctx context.Context) (*UpdateInfo, error) {
	manifest, err := u.fetchManifest(ctx)
	if err != nil {
		return nil, fmt.Errorf("falha ao buscar manifest: %w", err)
	}

	info := &UpdateInfo{
		CurrentVersion: u.currentVersion,
		LatestVersion:  manifest.Version,
		ReleaseNotes:   manifest.Notes,
		ReleaseDate:    manifest.Released,
	}

	// Verifica se há nova versão
	if manifest.Version != u.currentVersion {
		info.Available = true
		
		// Obtém informações do build para a plataforma atual
		buildKey := u.getBuildKey()
		if build, ok := manifest.Builds[buildKey]; ok {
			info.DownloadSize = build.Size
		}
	}

	return info, nil
}

// ApplyUpdate baixa e aplica a atualização
func (u *Updater) ApplyUpdate(ctx context.Context) error {
	manifest, err := u.fetchManifest(ctx)
	if err != nil {
		return fmt.Errorf("falha ao buscar manifest: %w", err)
	}

	// Verifica se há nova versão
	if manifest.Version == u.currentVersion {
		return fmt.Errorf("já está na versão mais recente (%s)", u.currentVersion)
	}

	// Obtém build para a plataforma atual
	buildKey := u.getBuildKey()
	build, ok := manifest.Builds[buildKey]
	if !ok {
		return fmt.Errorf("build não disponível para plataforma: %s", buildKey)
	}

	// Baixa o novo binário
	binary, err := u.downloadBinary(ctx, build.URL)
	if err != nil {
		return fmt.Errorf("falha ao baixar binário: %w", err)
	}
	defer binary.Close()

	// Verifica checksum
	if err := u.verifyChecksum(binary, build.Checksum); err != nil {
		return fmt.Errorf("falha na verificação de checksum: %w", err)
	}

	// Reseta para o início do arquivo após verificar checksum
	if seeker, ok := binary.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// Aplica a atualização
	err = update.Apply(binary, update.Options{
		// TargetPath pode ser especificado se quiser atualizar um binário diferente
	})
	if err != nil {
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("falha ao aplicar update e rollback: %v (rollback error: %v)", err, rerr)
		}
		return fmt.Errorf("falha ao aplicar update (rollback realizado): %w", err)
	}

	return nil
}

// fetchManifest busca o manifest de atualizações
func (u *Updater) fetchManifest(ctx context.Context) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u.manifestURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status HTTP inesperado: %d", resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("falha ao decodificar manifest: %w", err)
	}

	return &manifest, nil
}

// downloadBinary baixa o binário da URL especificada
f// Adiciona token se configurado (para releases privadas do GitHub)
	if u.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.githubToken)
		req.Header.Set("Accept", "application/octet-stream")
	}

	unc (u *Updater) downloadBinary(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("status HTTP inesperado ao baixar binário: %d", resp.StatusCode)
	}

	// Salva em arquivo temporário para permitir seek
	tmpFile, err := os.CreateTemp("", "assistente-update-*")
	if err != nil {
		resp.Body.Close()
		return nil, fmt.Errorf("falha ao criar arquivo temporário: %w", err)
	}

	_, err = io.Copy(tmpFile, resp.Body)
	resp.Body.Close()
	if err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, fmt.Errorf("falha ao salvar binário: %w", err)
	}

	// Volta ao início do arquivo
	tmpFile.Seek(0, io.SeekStart)

	return tmpFile, nil
}

// verifyChecksum verifica o checksum do binário
func (u *Updater) verifyChecksum(r io.Reader, expectedChecksum string) error {
	// Formato esperado: "sha256:hash"
	if len(expectedChecksum) < 7 || expectedChecksum[:7] != "sha256:" {
		return fmt.Errorf("formato de checksum inválido (esperado: sha256:hash)")
	}

	expectedHash := expectedChecksum[7:]

	hasher := sha256.New()
	if _, err := io.Copy(hasher, r); err != nil {
		return fmt.Errorf("falha ao calcular hash: %w", err)
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != expectedHash {
		return fmt.Errorf("checksum não corresponde (esperado: %s, obtido: %s)", expectedHash, actualHash)
	}

	return nil
}

// getBuildKey retorna a chave do build para a plataforma atual
func (u *Updater) getBuildKey() string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	
	// Normaliza nomes de plataformas
	switch os {
	case "darwin":
		return "darwin-" + arch // darwin-amd64, darwin-arm64
	case "windows":
		return "windows-" + arch // windows-amd64
	case "linux":
		return "linux-" + arch // linux-amd64, linux-arm64
	default:
		return fmt.Sprintf("%s-%s", os, arch)
	}
}

// GetExecutablePath retorna o caminho do executável atual
func GetExecutablePath() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(executable)
}
