package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// ProgressCallback é chamado durante o download para reportar progresso
type ProgressCallback func(bytesDownloaded, totalBytes int64, phase string)

// ElevationCallback é chamado quando elevação é necessária (retorna true se usuário autorizou)
type ElevationCallback func() bool

// Updater gerencia verificação e aplicação de atualizações
type Updater struct {
	currentVersion    string
	githubAPIURL      string
	githubToken       string // Token para acessar releases privadas (opcional)
	httpClient        *http.Client
	progressCallback  ProgressCallback
	elevationCallback ElevationCallback
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

// SetProgressCallback configura callback para reportar progresso
func (u *Updater) SetProgressCallback(callback ProgressCallback) {
	u.progressCallback = callback
}

// SetElevationCallback configura callback para solicitar elevação
func (u *Updater) SetElevationCallback(callback ElevationCallback) {
	u.elevationCallback = callback
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

	// No Windows, usa o instalador diretamente (mais confiável)
	if runtime.GOOS == "windows" {
		log.Printf("[Updater] 📦 Usando instalador NSIS para atualização no Windows...")
		return u.applyUpdateWindows(ctx, manifest)
	}

	// Em outras plataformas (Linux/macOS), usa atualização in-place
	log.Printf("[Updater] Aplicando atualização in-place...")
	return u.applyUpdateInPlace(ctx, manifest)
}

// applyUpdateElevated relança o processo de atualização com privilégios elevados
// NOTA: Desabilitado - preferimos usar o instalador NSIS diretamente
/*
func (u *Updater) applyUpdateElevated(ctx context.Context, manifest *Manifest) error {
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

	// Salva em arquivo temporário
	tmpFile, ok := binary.(*os.File)
	if !ok {
		return fmt.Errorf("tipo de arquivo inesperado")
	}
	tmpPath := tmpFile.Name()

	// Reporta instalação
	if u.progressCallback != nil {
		u.progressCallback(0, 100, "installing")
	}

	// Obtém caminho do executável atual
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("falha ao obter caminho do executável: %w", err)
	}

	// Cria script PowerShell para fazer a substituição com elevação
	// O script vai: aguardar app fechar, substituir exe, reiniciar
	script := fmt.Sprintf(`
# Aguarda 3 segundos para garantir que o app fechou
Start-Sleep -Seconds 3

# Move o novo executável para o local correto
$source = '%s'
$destination = '%s'
Move-Item -Path $source -Destination $destination -Force

# Reinicia o aplicativo
Start-Process -FilePath $destination

# Remove o script
Remove-Item -Path $PSCommandPath -Force
`, tmpPath, exePath, exePath)

	scriptFile, err := os.CreateTemp("", "update-*.ps1")
	if err != nil {
		return fmt.Errorf("falha ao criar script: %w", err)
	}
	scriptPath := scriptFile.Name()

	if _, err := scriptFile.WriteString(script); err != nil {
		return fmt.Errorf("falha ao escrever script: %w", err)
	}
	scriptFile.Close()

	// Executa PowerShell com elevação usando RunAs
	// Usa ArgumentList como array para evitar problemas com aspas
	psCommand := fmt.Sprintf(`Start-Process -FilePath powershell.exe -Verb RunAs -ArgumentList '-ExecutionPolicy','Bypass','-File','%s'`, scriptPath)

	cmd := exec.Command("powershell", "-Command", psCommand)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("falha ao solicitar elevação: %w", err)
	}

	log.Printf("[Updater] ✅ Processo de atualização elevado iniciado")

	// Importante: aguarda um pouco para garantir que o UAC foi mostrado
	time.Sleep(500 * time.Millisecond)

	// Encerra o aplicativo atual para permitir a substituição
	log.Printf("[Updater] 🔄 Encerrando aplicativo para permitir atualização...")
	os.Exit(0)

	return nil
}
*/


// applyUpdateWindows baixa e executa o instalador do Windows
func (u *Updater) applyUpdateWindows(ctx context.Context, manifest *Manifest) error {
	// Procura pelo instalador nos assets do GitHub
	var installerURL string
	var installerSize int64

	// Busca o instalador no GitHub release
	req, err := http.NewRequestWithContext(ctx, "GET", u.githubAPIURL, nil)
	if err != nil {
		return err
	}

	if u.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.githubToken)
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var ghRelease struct {
		Assets []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
			Size               int64  `json:"size"`
		} `json:"assets"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&ghRelease); err != nil {
		return fmt.Errorf("falha ao decodificar release: %w", err)
	}

	log.Printf("[Updater] Buscando instalador entre %d assets...", len(ghRelease.Assets))

	// Procura o instalador - aceita vários padrões de nomes
	for _, asset := range ghRelease.Assets {
		log.Printf("[Updater] Asset encontrado: %s", asset.Name)
		
		assetLower := strings.ToLower(asset.Name)
		
		// Aceita: *installer*.exe, *windows*.exe, *setup*.exe
		isInstaller := (strings.Contains(assetLower, "installer") ||
			strings.Contains(assetLower, "setup") ||
			strings.Contains(assetLower, "windows")) &&
			strings.HasSuffix(assetLower, ".exe")
		
		if isInstaller {
			installerURL = asset.BrowserDownloadURL
			installerSize = asset.Size
			log.Printf("[Updater] ✓ Instalador selecionado: %s (%d bytes)", asset.Name, asset.Size)
			break
		}
	}

	if installerURL == "" {
		return fmt.Errorf("instalador do Windows não encontrado no release (encontrados %d assets)", len(ghRelease.Assets))
	}

	log.Printf("[Updater] Baixando instalador: %s", installerURL)

	// Baixa o instalador
	installerFile, err := u.downloadInstaller(ctx, installerURL, installerSize)
	if err != nil {
		return fmt.Errorf("falha ao baixar instalador: %w", err)
	}
	// NÃO remove o instalador aqui - ele precisa executar em background
	// O instalador NSIS se auto-remove após a instalação

	// Reporta instalação
	if u.progressCallback != nil {
		u.progressCallback(0, 100, "installing")
	}

	log.Printf("[Updater] Executando instalador: %s", installerFile)

	// Executa o instalador de forma silenciosa em background
	// /S = silent mode no NSIS
	// O instalador irá aguardar o app fechar e então substituir o executável
	cmd := exec.Command(installerFile, "/S")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("falha ao executar instalador: %w", err)
	}

	log.Printf("[Updater] ✅ Instalador iniciado em segundo plano")
	log.Printf("[Updater] O aplicativo será atualizado quando você fechar e reabrir")

	return nil
}

// applyUpdateInPlace aplica atualização substituindo o executável (Linux/macOS)
func (u *Updater) applyUpdateInPlace(ctx context.Context, manifest *Manifest) error {
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

	// Reporta verificação
	if u.progressCallback != nil {
		u.progressCallback(0, 100, "verifying")
	}

	// Verifica checksum apenas se fornecido
	if build.Checksum != "" {
		if err := u.verifyChecksum(binary, build.Checksum); err != nil {
			return fmt.Errorf("falha na verificação de checksum: %w", err)
		}
	} else {
		log.Printf("[Updater] ⚠️ Checksum não fornecido, pulando verificação")
	}

	// Reseta para o início do arquivo após verificar checksum
	if seeker, ok := binary.(io.Seeker); ok {
		seeker.Seek(0, io.SeekStart)
	}

	// Reporta instalação
	if u.progressCallback != nil {
		u.progressCallback(0, 100, "installing")
	}

	// Aplica a atualização
	err = update.Apply(binary, update.Options{})
	if err != nil {
		log.Printf("[Updater] ❌ Erro ao aplicar atualização: %v", err)
		if rerr := update.RollbackError(err); rerr != nil {
			return fmt.Errorf("falha ao aplicar update e rollback: %v (rollback error: %v)", err, rerr)
		}
		// Retorna o erro original (wrapped) para preservar a mensagem completa
		return fmt.Errorf("falha ao aplicar update (rollback realizado): %w", err)
	}

	log.Printf("[Updater] ✅ Atualização aplicada com sucesso")
	return nil
}

// isPermissionError verifica se o erro é relacionado a permissões
// isPermissionError verifica se o erro é relacionado a permissões
// NOTA: Desabilitado - não usamos mais detecção de permissões no Windows
/*
func isPermissionError(err error) bool {
	if err == nil {
		return false
	}

	// Verifica a string do erro e todos os erros wrapped
	var errMsg string
	for e := err; e != nil; e = errors.Unwrap(e) {
		errMsg += strings.ToLower(e.Error()) + " "
	}

	// Verifica mensagens comuns de erro de permissão
	isPerm := strings.Contains(errMsg, "access is denied") ||
		strings.Contains(errMsg, "permission denied") ||
		strings.Contains(errMsg, "access denied") ||
		strings.Contains(errMsg, "cannot create")

	if isPerm {
		log.Printf("[Updater] ✓ Detectado erro de permissão na mensagem: %s", errMsg)
	} else {
		log.Printf("[Updater] ✗ Não é erro de permissão: %s", errMsg)
	}

	return isPerm
}
*/


// downloadInstaller baixa o instalador do Windows
func (u *Updater) downloadInstaller(ctx context.Context, url string, totalBytes int64) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	if u.githubToken != "" {
		req.Header.Set("Authorization", "Bearer "+u.githubToken)
		req.Header.Set("Accept", "application/octet-stream")
	}

	resp, err := u.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status HTTP inesperado ao baixar instalador: %d", resp.StatusCode)
	}

	// Cria arquivo temporário para o instalador
	tmpFile, err := os.CreateTemp("", "assistente-installer-*.exe")
	if err != nil {
		return "", fmt.Errorf("falha ao criar arquivo temporário: %w", err)
	}
	defer tmpFile.Close()

	// Reporta progresso durante download
	if u.progressCallback != nil {
		u.progressCallback(0, totalBytes, "downloading")
	}

	var bytesDownloaded int64
	buffer := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buffer[:n]); writeErr != nil {
				os.Remove(tmpFile.Name())
				return "", fmt.Errorf("falha ao escrever no arquivo: %w", writeErr)
			}
			bytesDownloaded += int64(n)
			if u.progressCallback != nil {
				u.progressCallback(bytesDownloaded, totalBytes, "downloading")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			os.Remove(tmpFile.Name())
			return "", fmt.Errorf("falha ao baixar instalador: %w", err)
		}
	}

	return tmpFile.Name(), nil
}
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

	// Reporta progresso durante download
	totalBytes := resp.ContentLength
	if u.progressCallback != nil {
		u.progressCallback(0, totalBytes, "downloading")
	}

	var bytesDownloaded int64
	buffer := make([]byte, 32*1024) // 32KB buffer
	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := tmpFile.Write(buffer[:n]); writeErr != nil {
				resp.Body.Close()
				tmpFile.Close()
				os.Remove(tmpFile.Name())
				return nil, fmt.Errorf("falha ao escrever no arquivo: %w", writeErr)
			}
			bytesDownloaded += int64(n)
			if u.progressCallback != nil {
				u.progressCallback(bytesDownloaded, totalBytes, "downloading")
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			resp.Body.Close()
			tmpFile.Close()
			os.Remove(tmpFile.Name())
			return nil, fmt.Errorf("falha ao baixar binário: %w", err)
		}
	}

	resp.Body.Close()

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
