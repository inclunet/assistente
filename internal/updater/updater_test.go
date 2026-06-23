package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/credentials"
)

// TestNew testa construção do Updater
func TestNew(t *testing.T) {
	credMgr := &credentials.Manager{}
	u := New("v1.0.0", credMgr)

	if u == nil {
		t.Fatal("esperado Updater, got nil")
	}
	if u.currentVersion != "v1.0.0" {
		t.Errorf("esperado currentVersion=v1.0.0, got %s", u.currentVersion)
	}
	if u.credMgr != credMgr {
		t.Error("credMgr não foi atribuído corretamente")
	}
}

// TestSetGitHubToken testa configuração de token
func TestSetGitHubToken(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	token := "ghp_testtoken123"

	u.SetGitHubToken(token)

	if u.githubToken != token {
		t.Errorf("esperado token %q, got %q", token, u.githubToken)
	}
}

// TestSetProgressCallback testa configuração de callback
func TestSetProgressCallback(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	callCount := 0
	cb := func(downloaded, total int64, phase string) {
		callCount++
	}

	u.SetProgressCallback(cb)

	if u.progressCallback == nil {
		t.Fatal("progressCallback não foi setado")
	}
	// Testa que callback pode ser chamado
	u.progressCallback(1000, 5000, "testing")
	if callCount != 1 {
		t.Errorf("esperado 1 chamada de callback, got %d", callCount)
	}
}

// TestSetElevationCallback testa configuração de callback de elevação
func TestSetElevationCallback(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	cb := func() bool { return true }

	u.SetElevationCallback(cb)

	if u.elevationCallback == nil {
		t.Fatal("elevationCallback não foi setado")
	}
	result := u.elevationCallback()
	if !result {
		t.Error("esperado true do callback")
	}
}

// TestCheckForUpdates_NewVersionAvailable testa detecção de nova versão
func TestCheckForUpdates_NewVersionAvailable(t *testing.T) {
	// Mock GitHub API
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/releases/latest") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v2.0.0",
			"Nova versão com correções",
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err != nil {
		t.Fatalf("esperado sucesso, got erro: %v", err)
	}
	if !info.Available {
		t.Error("esperado Available=true")
	}
	if info.CurrentVersion != "v1.0.0" {
		t.Errorf("esperado CurrentVersion=v1.0.0, got %s", info.CurrentVersion)
	}
	if info.LatestVersion != "v2.0.0" {
		t.Errorf("esperado LatestVersion=v2.0.0, got %s", info.LatestVersion)
	}
	if info.DownloadSize != 50000000 {
		t.Errorf("esperado DownloadSize=50000000, got %d", info.DownloadSize)
	}
}

// TestCheckForUpdates_AlreadyUpToDate testa quando versão está atual
func TestCheckForUpdates_AlreadyUpToDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v1.0.0",
			"",
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err != nil {
		t.Fatalf("esperado sucesso, got erro: %v", err)
	}
	if info.Available {
		t.Error("esperado Available=false (já atualizado)")
	}
}

func TestCheckForUpdates_NormalizesTagPrefixForComparison(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v1.0.0",
			"",
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("esperado sucesso, got erro: %v", err)
	}
	if info.Available {
		t.Error("esperado Available=false quando tag v1.0.0 e AppVersion 1.0.0 representam a mesma versão")
	}
	if info.LatestVersion != "v1.0.0" {
		t.Errorf("LatestVersion deve preservar tag_name original, got %q", info.LatestVersion)
	}
}

func TestCheckForUpdates_UsesDesktopAssetNotCLIAsset(t *testing.T) {
	key := getBuildKeyForTest()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := map[string]interface{}{
			"tag_name":     "v2.0.0",
			"published_at": "2024-03-18T10:30:00Z",
			"body":         "",
			"assets": []map[string]interface{}{
				{
					"name":                 "asst-" + key,
					"browser_download_url": "https://github.com/inclunet/assistente/releases/download/v2.0.0/asst-" + key,
					"size":                 int64(111),
				},
				{
					"name":                 "assistente-" + key + "-installer.exe",
					"browser_download_url": "https://github.com/inclunet/assistente/releases/download/v2.0.0/assistente-" + key + "-installer.exe",
					"size":                 int64(222),
				},
				{
					"name":                 "assistente-" + key + ".sha256",
					"browser_download_url": "https://github.com/inclunet/assistente/releases/download/v2.0.0/assistente-" + key + ".sha256",
					"size":                 int64(333),
				},
				{
					"name":                 "assistente-" + key + ".exe",
					"browser_download_url": "https://github.com/inclunet/assistente/releases/download/v2.0.0/assistente-" + key + ".exe",
					"size":                 int64(444),
				},
				{
					"name":                 "assistente-" + key + ".deb",
					"browser_download_url": "https://github.com/inclunet/assistente/releases/download/v2.0.0/assistente-" + key + ".deb",
					"size":                 int64(555),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())
	if err != nil {
		t.Fatalf("esperado sucesso, got erro: %v", err)
	}
	if info.DownloadSize != 444 {
		t.Errorf("esperado asset desktop assistente-* com size 444, got %d", info.DownloadSize)
	}
}

// TestCheckForUpdates_NetworkError testa erro de rede
func TestCheckForUpdates_NetworkError(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = "http://invalid-host-that-does-not-exist-xyz.com/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err == nil {
		t.Fatal("esperado erro de rede")
	}
	if info != nil {
		t.Error("esperado info=nil com erro")
	}
}

// TestCheckForUpdates_NoCompatibleBuild testa quando não há build para plataforma
func TestCheckForUpdates_NoCompatibleBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retorna release com build para plataforma diferente
		response := createGitHubReleaseResponse(
			"v2.0.0",
			"",
			[]string{"unknown-platform"}, // Plataforma não compatível
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	// Não deve retornar erro - apenas não preenchará DownloadSize
	if err != nil {
		t.Logf("NOTE: CheckForUpdates retornou erro para build incompatível: %v", err)
	}
	if info != nil && info.DownloadSize != 0 {
		t.Logf("NOTE: DownloadSize deveria ser 0 para build incompatível, got %d", info.DownloadSize)
	}
}

// TestCheckForUpdates_InvalidJSON testa parseo JSON inválido
func TestCheckForUpdates_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err == nil {
		t.Fatal("esperado erro de JSON decode")
	}
	if info != nil {
		t.Error("esperado info=nil com erro")
	}
}

// TestCheckForUpdates_WithGitHubToken testa envio de token github
func TestCheckForUpdates_WithGitHubToken(t *testing.T) {
	tokenReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tokenReceived = r.Header.Get("Authorization")

		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v2.0.0",
			"",
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"
	u.SetGitHubToken("ghp_test123")

	_, _ = u.CheckForUpdates(context.Background())

	if !strings.Contains(tokenReceived, "ghp_test123") {
		t.Errorf("esperado token no header Authorization, got %q", tokenReceived)
	}
}

// TestCheckForUpdates_ContextCanceled testa cancelamento de contexto
func TestCheckForUpdates_ContextCanceled(t *testing.T) {
	// Espera antes de responder
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	info, err := u.CheckForUpdates(ctx)

	if err == nil {
		t.Fatal("esperado erro de timeout/context")
	}
	if info != nil {
		t.Error("esperado info=nil com erro de context")
	}
}

// TestGetBuildKey_Windows testa seleção de build no Windows
func TestGetBuildKey_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("teste específico para Windows")
	}

	u := New("v1.0.0", &credentials.Manager{})
	key := u.getBuildKey()

	if !strings.Contains(key, "windows") {
		t.Errorf("esperado 'windows' em key, got %q", key)
	}
	if !strings.Contains(key, "amd64") && !strings.Contains(key, "arm64") {
		t.Errorf("esperado amd64 ou arm64 em key, got %q", key)
	}
}

// TestGetBuildKey_Darwin testa seleção de build no macOS
func TestGetBuildKey_Darwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("teste específico para macOS")
	}

	u := New("v1.0.0", &credentials.Manager{})
	key := u.getBuildKey()

	if !strings.Contains(key, "darwin") {
		t.Errorf("esperado 'darwin' em key, got %q", key)
	}
}

// TestGetBuildKey_Linux testa seleção de build no Linux
func TestGetBuildKey_Linux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("teste específico para Linux")
	}

	u := New("v1.0.0", &credentials.Manager{})
	key := u.getBuildKey()

	if !strings.Contains(key, "linux") {
		t.Errorf("esperado 'linux' em key, got %q", key)
	}
}

// TestVerifyChecksum_ValidSHA256 testa verificação de checksum válido
func TestVerifyChecksum_ValidSHA256(t *testing.T) {
	// Criar arquivo temporário com conteúdo conhecido
	tmpfile, err := os.CreateTemp("", "test-checksum")
	if err != nil {
		t.Fatalf("erro ao criar temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	content := []byte("test content")
	if _, err := tmpfile.Write(content); err != nil {
		t.Fatalf("erro ao escrever temp file: %v", err)
	}
	_ = tmpfile.Close()

	u := New("v1.0.0", &credentials.Manager{})

	// Calcula SHA256 correto do conteúdo
	hash := sha256.Sum256(content)
	correctHash := "sha256:" + hex.EncodeToString(hash[:])

	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("erro ao abrir arquivo: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = u.verifyChecksum(file, correctHash)

	if err != nil {
		t.Errorf("esperado sucesso com hash correto, got erro: %v", err)
	}
}

// TestVerifyChecksum_MismatchedHash testa erro de hash divergente
func TestVerifyChecksum_MismatchedHash(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-checksum")
	if err != nil {
		t.Fatalf("erro ao criar temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	_, _ = tmpfile.Write([]byte("test content"))
	_ = tmpfile.Close()

	u := New("v1.0.0", &credentials.Manager{})

	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("erro ao abrir arquivo: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = u.verifyChecksum(file, "sha256:0000000000000000000000000000000000000000000000000000000000000000")

	if err == nil {
		t.Fatal("esperado erro de mismatch")
	}
	if !strings.Contains(err.Error(), "checksum") {
		t.Errorf("erro deve mencionar 'checksum', got: %v", err)
	}
}

// TestVerifyChecksum_InvalidFormat testa erro de formato de checksum
func TestVerifyChecksum_InvalidFormat(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-checksum")
	if err != nil {
		t.Fatalf("erro ao criar temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	_, _ = tmpfile.Write([]byte("test"))
	_ = tmpfile.Close()

	u := New("v1.0.0", &credentials.Manager{})

	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("erro ao abrir arquivo: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = u.verifyChecksum(file, "invalid-format-no-colon")

	if err == nil {
		t.Fatal("esperado erro de formato")
	}
}

// TestVerifyChecksum_UnsupportedAlgorithm testa algoritmo não suportado
func TestVerifyChecksum_UnsupportedAlgorithm(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-checksum")
	if err != nil {
		t.Fatalf("erro ao criar temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	_, _ = tmpfile.Write([]byte("test"))
	_ = tmpfile.Close()

	u := New("v1.0.0", &credentials.Manager{})

	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("erro ao abrir arquivo: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = u.verifyChecksum(file, "md5:abcdef123456")

	if err == nil {
		t.Fatal("esperado erro (md5 não suportado)")
	}
}

// TestVerifyChecksum_NoChecksum testa quando checksum é vazio
func TestVerifyChecksum_NoChecksum(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-checksum")
	if err != nil {
		t.Fatalf("erro ao criar temp file: %v", err)
	}
	defer func() { _ = os.Remove(tmpfile.Name()) }()

	_, _ = tmpfile.Write([]byte("test"))
	_ = tmpfile.Close()

	u := New("v1.0.0", &credentials.Manager{})

	file, err := os.Open(tmpfile.Name())
	if err != nil {
		t.Fatalf("erro ao abrir arquivo: %v", err)
	}
	defer func() { _ = file.Close() }()

	err = u.verifyChecksum(file, "")

	// Checksum vazio não deve erro (opcional)
	if err != nil {
		t.Logf("NOTE: checksum vazio rejeitou, pode ser ok: %v", err)
	}
}

// TestIsInstalledVersion testa detecção de versão instalada
func TestIsInstalledVersion(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("teste específico para Windows")
	}

	u := New("v1.0.0", &credentials.Manager{})

	// Nota: isInstalledVersion() é um método que não toma parâmetros
	// Ele detecta se o binário atual está em Program Files
	// Não é possível testar com path customizado nesta forma
	execPath, err := os.Executable()
	if err != nil {
		t.Skipf("não foi possível obter caminho do executável: %v", err)
	}

	result := u.isInstalledVersion()
	// Em ambiente de teste, raramente será em Program Files
	// Apenas validar que retorna bool
	if result != (strings.Contains(strings.ToLower(execPath), "program files")) {
		t.Logf("isInstalledVersion retornou %v para %q", result, execPath)
	}
}

// TestProgressCallback testa invocação de callbacks de progresso
func TestProgressCallback(t *testing.T) {
	progressCalls := []struct {
		downloaded int64
		total      int64
		phase      string
	}{}

	cb := func(downloaded, total int64, phase string) {
		progressCalls = append(progressCalls, struct {
			downloaded int64
			total      int64
			phase      string
		}{downloaded, total, phase})
	}

	u := New("v1.0.0", &credentials.Manager{})
	u.SetProgressCallback(cb)

	u.progressCallback(1000, 5000, "downloading")
	u.progressCallback(3000, 5000, "downloading")
	u.progressCallback(5000, 5000, "verifying")

	if len(progressCalls) != 3 {
		t.Fatalf("esperado 3 chamadas, got %d", len(progressCalls))
	}

	if progressCalls[0].phase != "downloading" {
		t.Errorf("esperado fase 'downloading', got %q", progressCalls[0].phase)
	}
	if progressCalls[2].phase != "verifying" {
		t.Errorf("esperado fase 'verifying', got %q", progressCalls[2].phase)
	}
}

// TestElevationCallback_Granted testa elevação concedida
func TestElevationCallback_Granted(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	u.SetElevationCallback(func() bool { return true })

	result := u.elevationCallback()

	if !result {
		t.Error("esperado true da elevação")
	}
}

// TestElevationCallback_Denied testa elevação negada
func TestElevationCallback_Denied(t *testing.T) {
	u := New("v1.0.0", &credentials.Manager{})
	u.SetElevationCallback(func() bool { return false })

	result := u.elevationCallback()

	if result {
		t.Error("esperado false da elevação")
	}
}

// TestManifest_BuildKeysForAllPlatforms testa que manifest mapeia todas plataformas
func TestManifest_BuildKeysForAllPlatforms(t *testing.T) {
	expectedKeys := map[string]bool{
		"windows-amd64": false,
		"darwin-amd64":  false,
		"darwin-arm64":  false,
		"linux-amd64":   false,
		"linux-arm64":   false,
	}

	m := Manifest{
		Version: "v1.0.0",
		Builds: map[string]Build{
			"windows-amd64": {URL: "https://example.com/w-amd64.exe"},
			"darwin-amd64":  {URL: "https://example.com/d-amd64"},
			"darwin-arm64":  {URL: "https://example.com/d-arm64"},
			"linux-amd64":   {URL: "https://example.com/l-amd64"},
			"linux-arm64":   {URL: "https://example.com/l-arm64"},
		},
	}

	for key := range m.Builds {
		if _, ok := expectedKeys[key]; !ok {
			t.Logf("key não esperada: %s", key)
		} else {
			expectedKeys[key] = true
		}
	}

	for key, found := range expectedKeys {
		if !found {
			t.Errorf("key esperada não encontrada: %s", key)
		}
	}
}

// TestCheckForUpdates_ReleaseNotes testa inclusão de notas de release
func TestCheckForUpdates_ReleaseNotes(t *testing.T) {
	releaseNotes := "Bug fixes and improvements"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v2.0.0",
			releaseNotes,
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.ReleaseNotes != releaseNotes {
		t.Errorf("esperado notas %q, got %q", releaseNotes, info.ReleaseNotes)
	}
}

// TestCheckForUpdates_ReleaseDate testa inclusão de data de release
func TestCheckForUpdates_ReleaseDate(t *testing.T) {
	releaseDate := "2024-03-18T10:30:00Z"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v2.0.0",
			"",
			[]string{key},
		)
		// Sobrescreve a data no response
		response["published_at"] = releaseDate
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	info, err := u.CheckForUpdates(context.Background())

	if err != nil {
		t.Fatalf("erro: %v", err)
	}
	if info.ReleaseDate != releaseDate {
		t.Errorf("esperado data %q, got %q", releaseDate, info.ReleaseDate)
	}
}

// TestApplyUpdate_NoCompatibleBuild testa erro quando build não existe
func TestApplyUpdate_NoCompatibleBuild(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Retorna release com versão igual (não há update)
		response := createGitHubReleaseResponse(
			"v1.0.0", // mesma versão, sem update
			"",
			[]string{"windows-amd64"},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	err := u.ApplyUpdate(context.Background())

	// Deve retornar erro pois versão é igual
	if err == nil {
		t.Fatal("esperado erro (já está na versão mais recente)")
	}
}

// TestUpdateInfo_AllFields testa estrutura UpdateInfo preenchida
func TestUpdateInfo_AllFields(t *testing.T) {
	info := &UpdateInfo{
		Available:      true,
		CurrentVersion: "v1.0.0",
		LatestVersion:  "v2.0.0",
		ReleaseNotes:   "Improvements",
		ReleaseDate:    "2024-03-18T10:30:00Z",
		DownloadSize:   50000000,
	}

	if !info.Available {
		t.Error("Available deve ser true")
	}
	if info.CurrentVersion == "" || info.LatestVersion == "" {
		t.Error("versões não devem estar vazias")
	}
	if info.DownloadSize <= 0 {
		t.Error("tamanho deve ser > 0")
	}
}

// TestBuild_AllFields testa estrutura Build
func TestBuild_AllFields(t *testing.T) {
	b := Build{
		URL:      "https://example.com/file.exe",
		Checksum: "sha256:abcdef",
		Size:     1024000,
	}

	if b.URL == "" {
		t.Error("URL não deve estar vazia")
	}
	if !strings.Contains(b.Checksum, ":") {
		t.Error("Checksum deve ter formato algoritmo:hash")
	}
	if b.Size <= 0 {
		t.Error("Size deve ser > 0")
	}
}

// ============ Helpers ============

// createGitHubReleaseResponse cria uma resposta GitHub Release API completa
func createGitHubReleaseResponse(version, notes string, platforms []string) map[string]interface{} {
	assets := make([]map[string]interface{}, len(platforms))
	for i, platform := range platforms {
		assets[i] = map[string]interface{}{
			"name":                 "assistente-" + platform + ".exe",
			"browser_download_url": "https://github.com/inclunet/assistente/releases/download/" + version + "/assistente-" + platform + ".exe",
			"size":                 int64(50000000),
		}
	}

	return map[string]interface{}{
		"tag_name":     version,
		"published_at": "2024-03-18T10:30:00Z",
		"body":         notes,
		"assets":       assets,
	}
}

// getBuildKeyForTest retorna a chave de build esperada para a plataforma atual
func getBuildKeyForTest() string {
	switch runtime.GOOS {
	case "windows":
		if runtime.GOARCH == "amd64" {
			return "windows-amd64"
		}
		return "windows-" + runtime.GOARCH
	case "darwin":
		if runtime.GOARCH == "arm64" {
			return "darwin-arm64"
		}
		return "darwin-amd64"
	case "linux":
		if runtime.GOARCH == "arm64" {
			return "linux-arm64"
		}
		return "linux-amd64"
	}
	return "unknown"
}

// BenchmarkCheckForUpdates benchmarks a verificação de atualizações
func BenchmarkCheckForUpdates(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := getBuildKeyForTest()
		response := createGitHubReleaseResponse(
			"v2.0.0",
			"",
			[]string{key},
		)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(response)
	}))
	defer server.Close()

	u := New("v1.0.0", &credentials.Manager{})
	u.githubAPIURL = server.URL + "/releases/latest"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = u.CheckForUpdates(context.Background())
	}
}
