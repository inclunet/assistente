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
	// GitHubAPIURL é a base da API do GitHub para releases
	GitHubAPIURL = "https://api.github.com/repos/inclunet/assistente/releases/latest"

	// CheckInterval é o intervalo padrão para verificar atualizações
	CheckInterval = 6 * time.Hour
)

// Manifest representa o arquivo de metadados de versões
type Manifest struct {
	Version  string           `json:"version"`
	Released string           `json:"released"`
	Notes    string           `json:"notes,omitempty"`
	Builds   map[string]Build `json:"builds"`
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
	githubAPIURL   string
	githubToken    string // Token para acessar releases privadas (opcional)
	httpClient     *http.Client
}

// New cria um novo Updater
func New(currentVersion string) *Updater {
	return &Updater{
		currentVersion: currentVersion,
		githubAPIURL:   GitHubAPIURL,
		githubToken:    "", // Pode ser configurado depois se releases forem privadas
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

// fetchManifest busca o manifest de atualizações da API do GitHub
func (u *Updater) fetchManifest(ctx context.Context) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u.githubAPIURL, nil)
	if err != nil {
		return nil, err
	}

	// Adiciona token se configurado (para releases privadas)
	if u.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.githubToken)
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status HTTP inesperado: %d", resp.StatusCode)
	}

	// Parse GitHub Release response
	var ghRelease struct {
		TagName     string    `json:"tag_name"`
		PublishedAt time.Time `json:"published_at"`
		Body        string    `json:"body"`
		Assets      []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return nil, fmt.Errorf("falha ao decodificar release: %w", err)
	}

	// Converte para formato Manifest
	manifest := &Manifest{
		Version:  ghRelease.TagName,
		Released: ghRelease.PublishedAt.Format(time.RFC3339),
		Notes:    ghRelease.Body,
		Builds:   make(map[string]Build),
	}

	// Mapeia assets para builds
	for _, asset := range ghRelease.Assets {
		// Extrai plataforma do nome do asset
		// Exemplo: assistente-windows-amd64.exe -> windows-amd64
		var buildKey string
		switch {
		case contains(asset.Name, "windows-amd64"):
			buildKey = "windows-amd64"
		case contains(asset.Name, "darwin-amd64"):
			buildKey = "darwin-amd64"
		case contains(asset.Name, "darwin-arm64"):
			buildKey = "darwin-arm64"
		case contains(asset.Name, "linux-amd64"):
			buildKey = "linux-amd64"
		default:
			continue // Skip instaladores e outros arquivos
		}

		// Ignora instaladores (queremos apenas executáveis)
		if contains(asset.Name, "installer") || contains(asset.Name, ".dmg") || contains(asset.Name, ".AppImage") {
			continue
		}

		manifest.Builds[buildKey] = Build{
			URL:      asset.BrowserDownloadURL,
			Checksum: "", // Checksums virão de arquivo separado se necessário
			Size:     asset.Size,
		}
	}

	return manifest, nil
}

// contains verifica se uma string contém outra (helper)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// downloadBinary baixa o binário da URL especificada
func (u *Updater) downloadBinary(ctx context.Context, url string) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Adiciona token se configurado (para releases privadas do GitHub)
	if u.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.githubToken)
		req.Header.Set("Accept", "application/octet-stream")
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
