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
