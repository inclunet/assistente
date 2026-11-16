package logging

import (
	"assistente/internal/configdir"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettingsUsaArquivoDoUsuarioERotacaoDiaria(t *testing.T) {
	settings := DefaultSettings()
	if settings.Path != filepath.Join(configdir.GetHomeDir(), "assistente.log") || settings.RotateHours != 24 {
		t.Fatalf("defaults = %+v", settings)
	}
}

func TestManagerCriaDiretorioDePreferenciasAntesDoLogPadrao(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".assistente")
	m, err := NewManager(filepath.Join(dir, "logging.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	settings := Settings{Enabled: true, Path: filepath.Join(dir, "assistente.log"), MaxMiB: 50, Backups: 5, RotateHours: 24}
	if err := m.Save(settings); err != nil {
		t.Fatalf("ativar log padrão: %v", err)
	}
	if _, err := m.Write([]byte("primeiro registro")); err != nil {
		t.Fatal(err)
	}
}

func TestManagerRecusaDestinoIgualAoArquivoDeConfiguracao(t *testing.T) {
	config := filepath.Join(t.TempDir(), "logging.json")
	m, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	settings := Settings{Enabled: true, Path: config, MaxMiB: 1, Backups: 2}
	if err := m.Save(settings); err == nil {
		t.Fatal("aceitou sobrescrever o arquivo de configuração")
	}
	if _, err := os.Stat(config); !os.IsNotExist(err) {
		t.Fatalf("arquivo de configuração criado: %v", err)
	}
	if _, err := NewManager(config, config); err == nil {
		t.Fatal("flag aceitou arquivo de configuração")
	}
}

func TestManagerSalvaAtivaDesativaERestauraConfiguracao(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	path := filepath.Join(dir, "app.log")
	m, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	want := Settings{Enabled: true, Path: path, MaxMiB: 1, Backups: 2}
	if err := m.Save(want); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write([]byte("primeiro\n")); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m, err = NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	if got, _, _ := m.Settings(); got != want {
		t.Fatalf("configuração = %+v", got)
	}
	if _, err := m.Write([]byte("segundo\n")); err != nil {
		t.Fatal(err)
	}
	want.Enabled = false
	if err := m.Save(want); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write([]byte("descartado")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "primeiro\nsegundo\n" {
		t.Fatalf("arquivo = %q, erro = %v", got, err)
	}
}

func TestManagerMantemDestinoAnteriorSeConfiguracaoFalhar(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	good := Settings{Enabled: true, Path: filepath.Join(dir, "app.log"), MaxMiB: 1, Backups: 2}
	if err := m.Save(good); err != nil {
		t.Fatal(err)
	}
	bad := good
	bad.Path = dir
	if err := m.Save(bad); err == nil {
		t.Fatal("esperado erro ao abrir diretório como arquivo")
	}
	if got, _, _ := m.Settings(); got != good {
		t.Fatalf("configuração alterada: %+v", got)
	}
	if _, err := m.Write([]byte("continua")); err != nil {
		t.Fatal(err)
	}
}

func TestManagerFlagMantemPrecedencia(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "flag.log")
	m, err := NewManager(filepath.Join(dir, "config.json"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	settings := Settings{Enabled: true, Path: filepath.Join(dir, "configurado.log"), MaxMiB: 1, Backups: 2}
	if err := m.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, override, _ := m.Settings(); !override {
		t.Fatal("precedência da flag ausente")
	}
	if _, err := m.Write([]byte("flag")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "flag" {
		t.Fatalf("arquivo = %q, erro = %v", got, err)
	}
	if _, err := os.Stat(settings.Path); !os.IsNotExist(err) {
		t.Fatalf("destino configurado foi aberto: %v", err)
	}
}

func TestManagerFlagNaoConcorreComArquivoRotacionado(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	configured, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	settings := Settings{Enabled: true, Path: path, MaxMiB: 1, Backups: 2}
	if err := configured.Save(settings); err != nil {
		t.Fatal(err)
	}
	if override, err := NewManager(filepath.Join(dir, "outra-config.json"), path); err == nil {
		_ = override.Close()
		t.Fatal("flag conseguiu abrir arquivo ocupado pelo rotacionador")
	}
	if err := configured.Close(); err != nil {
		t.Fatal(err)
	}
	override, err := NewManager(filepath.Join(dir, "outra-config.json"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := override.Close(); err != nil {
			t.Error(err)
		}
	}()
	if reopened, err := NewRotatingWriter(path, 4, 1); err == nil {
		_ = reopened.Close()
		t.Fatal("rotacionador conseguiu abrir arquivo ocupado pela flag")
	}
}

func TestManagerFlagAntesDoRotacionadorNaoPerdeArquivo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	override, err := NewManager(filepath.Join(dir, "override.json"), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := override.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := override.Write([]byte("preservar")); err != nil {
		t.Fatal(err)
	}
	configured, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := configured.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := configured.Save(Settings{Enabled: true, Path: path, MaxMiB: 1, Backups: 2}); err == nil {
		t.Fatal("arquivo preexistente foi adotado")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "preservar" {
		t.Fatalf("arquivo = %q, erro = %v", got, err)
	}
}

func TestManagerRecusaFlagPorHardlinkParaArquivoRotacionado(t *testing.T) {
	dir := t.TempDir()
	path, alias := filepath.Join(dir, "app.log"), filepath.Join(dir, "alias.log")
	configured, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := configured.Close(); err != nil {
			t.Error(err)
		}
	}()
	if err := configured.Save(Settings{Enabled: true, Path: path, MaxMiB: 1, Backups: 2}); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Skipf("sistema não permite criar hardlink: %v", err)
	}
	if other, err := NewManager(filepath.Join(dir, "outra.json"), alias); err == nil {
		_ = other.Close()
		t.Fatal("flag aceitou hardlink para destino rotacionado")
	}
}

func TestManagerRecusaFlagEmArquivoComHardlinkPreexistente(t *testing.T) {
	dir := t.TempDir()
	path, alias := filepath.Join(dir, "app.log"), filepath.Join(dir, "alias.log")
	if err := os.WriteFile(path, []byte("preservar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Skipf("sistema não permite criar hardlink: %v", err)
	}
	if manager, err := NewManager(filepath.Join(dir, "config.json"), alias); err == nil {
		_ = manager.Close()
		t.Fatal("flag aceitou arquivo com hardlink")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "preservar" {
		t.Fatalf("arquivo = %q, erro = %v", got, err)
	}
}

func TestManagerNaoApagaErroDaFlagAoSalvarPreferencias(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "config.json"), filepath.Join(dir, "flag.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	m.writeErr = errors.New("disco cheio")
	if err := m.Save(Settings{MaxMiB: 50, Backups: 5}); err != nil {
		t.Fatal(err)
	}
	if _, _, problem := m.Settings(); problem == "" {
		t.Fatal("erro do destino da flag foi apagado")
	}
	if _, err := m.Write([]byte("recuperado")); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(Settings{MaxMiB: 50, Backups: 5}); err != nil {
		t.Fatal(err)
	}
	if _, _, problem := m.Settings(); problem != "" {
		t.Fatalf("erro não foi limpo: %s", problem)
	}
}

func TestManagerRecuperaDestinoIndisponivelEFlagFunciona(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	m, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	settings := Settings{Enabled: true, Path: filepath.Join(dir, "app.log"), MaxMiB: 1, Backups: 2}
	if err := m.Save(settings); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(settings.Path+".assistente-archives", settings.Path+".ausente"); err != nil {
		t.Fatal(err)
	}
	m, err = NewManager(config, "")
	if err != nil {
		t.Fatalf("falha não deve impedir inicialização: %v", err)
	}
	if _, _, problem := m.Settings(); problem == "" {
		t.Fatal("erro indisponível na interface")
	}
	settings.Enabled = false
	if err := m.Save(settings); err != nil {
		t.Fatal(err)
	}
	if _, _, problem := m.Settings(); problem != "" {
		t.Fatalf("erro não foi limpo: %s", problem)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("inválido"), 0o600); err != nil {
		t.Fatal(err)
	}
	flag := filepath.Join(dir, "flag.log")
	m, err = NewManager(config, flag)
	if err != nil {
		t.Fatalf("configuração inválida não deve bloquear flag válida: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := m.Write([]byte("diagnóstico")); err != nil {
		t.Fatal(err)
	}
	if _, overridden, problem := m.Settings(); !overridden || problem == "" {
		t.Fatal("deveria informar flag e erro de configuração")
	}
}

func TestManagerPreservaDadosDeRotacaoInterrompida(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.json")
	path := filepath.Join(dir, "app.log")
	m, err := NewManager(config, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Save(Settings{Enabled: true, Path: path, MaxMiB: 1, Backups: 2}); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Write([]byte("dados preservados")); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(path+".assistente-archives", "1")
	if err := os.Rename(path, backup); err != nil {
		t.Fatal(err)
	}
	m, err = NewManager(config, "")
	if err != nil {
		t.Fatalf("inicialização deve ser recuperável: %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, _, problem := m.Settings(); problem == "" {
		t.Fatal("interrupção não foi informada")
	}
	got, err := os.ReadFile(backup)
	if err != nil || string(got) != "dados preservados" {
		t.Fatalf("backup = %q, erro = %v", got, err)
	}
}

func TestManagerNaoApagaErroEnquantoRotacaoEstaPendente(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	settings := Settings{Enabled: true, Path: filepath.Join(dir, "app.log"), MaxMiB: 1, Backups: 2}
	if err := m.Save(settings); err != nil {
		t.Fatal(err)
	}
	m.writeErr = errors.New("falha de escrita")
	if err := os.WriteFile(filepath.Join(settings.Path+".assistente-archives", ".rotation-pending"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(settings); err == nil {
		t.Fatal("configuração deveria recusar rotação pendente")
	}
	if _, _, problem := m.Settings(); problem == "" {
		t.Fatal("erro foi apagado")
	}
}

func TestManagerMantemErroAteGravacaoSerRestabelecida(t *testing.T) {
	dir := t.TempDir()
	m, err := NewManager(filepath.Join(dir, "config.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	}()
	settings := Settings{Enabled: true, Path: filepath.Join(dir, "app.log"), MaxMiB: 1, Backups: 2}
	if err := m.Save(settings); err != nil {
		t.Fatal(err)
	}
	m.writeErr = errors.New("disco cheio")
	if err := m.Save(settings); err == nil {
		t.Fatal("erro não deveria ser apagado antes de escrita bem-sucedida")
	}
	if _, err := m.Write([]byte("registro recuperado")); err != nil {
		t.Fatal(err)
	}
	if err := m.Save(settings); err != nil {
		t.Fatalf("escrita foi restabelecida: %v", err)
	}
	if _, _, problem := m.Settings(); problem != "" {
		t.Fatalf("erro permaneceu: %s", problem)
	}
}
