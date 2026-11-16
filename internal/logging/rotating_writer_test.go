package logging

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRotatingWriterRetemArquivosMaisRecentes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistente.log")
	w, err := NewRotatingWriter(path, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range []string{"aaaa", "bbbb", "cccc", "dddd"} {
		if _, err := w.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{path: "dddd", filepath.Join(path+".assistente-archives", "1"): "cccc", filepath.Join(path+".assistente-archives", "2"): "bbbb"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Errorf("arquivo %s = %q, erro %v; esperado %q", name, got, err, want)
		}
	}
}

func TestRotatingWriterNaoAdotaArquivosPreexistentes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "dados.log")
	if err := os.WriteFile(path, []byte("dados existentes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotatingWriter(path, 4, 1); err == nil {
		t.Fatal("arquivo preexistente foi aceito")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "dados existentes" {
		t.Fatalf("arquivo modificado: %q, %v", got, err)
	}
}

func TestRotatingWriterLimpaConfiguracaoInicialQueFalhou(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	_, err := newRotatingWriter(path, 4, 1, func() error { return errors.New("falha simulada") })
	if err == nil {
		t.Fatal("falha inicial não foi propagada")
	}
	for _, target := range []string{path, path + ".assistente-archives"} {
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("estado parcial em %s: %v", target, err)
		}
	}
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatalf("nova tentativa falhou: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRotatingWriterGravaRegistroMesmoSeLimpezaFalhar(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := w.Write([]byte("aaaa")); err != nil {
		t.Fatal(err)
	}
	w.afterCommit = func() error { return errors.New("limpeza indisponível") }
	n, err := w.Write([]byte("bbbb"))
	if n != 4 || err == nil {
		t.Fatalf("escrita = %d, erro = %v", n, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "bbbb" {
		t.Fatalf("registro atual = %q, erro = %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(path+".assistente-archives", ".rotation-pending")); err != nil {
		t.Fatalf("estado pendente ausente: %v", err)
	}
}

func TestRotatingWriterRecusaDiretorioDeBackupNaoReconhecido(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	archive := path + ".assistente-archives"
	if err := os.Mkdir(archive, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(archive, "1")
	if err := os.WriteFile(sentinel, []byte("preservar"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotatingWriter(path, 4, 1); err == nil {
		t.Fatal("diretório desconhecido foi aceito")
	}
	got, err := os.ReadFile(sentinel)
	if err != nil || string(got) != "preservar" {
		t.Fatalf("arquivo modificado: %q, %v", got, err)
	}
}

func TestRotatingWriterRecusaArquivoSubstituido(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(dir, "original")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("arquivo alheio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotatingWriter(path, 4, 1); err == nil {
		t.Fatal("arquivo substituído foi aceito")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "arquivo alheio" {
		t.Fatalf("arquivo modificado: %q, %v", got, err)
	}
}

func TestRotatingWriterRecusaTransacaoInterrompidaComAtivoIntacto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := NewRotatingWriter(path, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("ativo")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	archive := path + ".assistente-archives"
	backup := filepath.Join(archive, ".oldest-sentinela")
	if err := os.WriteFile(backup, []byte("backup preservado"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archive, ".rotation-pending"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRotatingWriter(path, 4, 2); err == nil {
		t.Fatal("transação pendente foi aceita")
	}
	for name, want := range map[string]string{path: "ativo", backup: "backup preservado"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("arquivo %s = %q, erro = %v", name, got, err)
		}
	}
}

func TestRotatingWriterRecusaSubstituicaoDuranteExecucao(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, filepath.Join(dir, "original")); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("Windows não permite renomear o arquivo enquanto está aberto")
		}
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("arquivo alheio"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("x")); err == nil {
		t.Fatal("substituição não foi detectada")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "arquivo alheio" {
		t.Fatalf("arquivo modificado: %q, %v", got, err)
	}
}

func TestRotatingWriterRetomaAposFalhaTemporaria(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := w.Write([]byte("abcd")); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(path+".assistente-archives", "1")
	if err := os.Mkdir(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(blocker, "sentinela")
	if err := os.WriteFile(child, []byte("protegido"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("e")); err == nil {
		t.Fatal("rotação deveria falhar")
	}
	if err := os.Remove(child); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("e")); err != nil {
		t.Fatalf("não retomou: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "e" {
		t.Fatalf("registro = %q, erro = %v", got, err)
	}
}

func TestRotatingWriterRestauraRetencaoAposFalhaNoNovoArquivo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := NewRotatingWriter(path, 4, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, s := range []string{"aaaa", "bbbb", "cccc"} {
		if _, err := w.Write([]byte(s)); err != nil {
			t.Fatal(err)
		}
	}
	w.afterMove = func() error { return errors.New("falha simulada") }
	if _, err := w.Write([]byte("dddd")); err == nil {
		t.Fatal("esperado erro")
	}
	for name, want := range map[string]string{path: "cccc", filepath.Join(path+".assistente-archives", "1"): "bbbb", filepath.Join(path+".assistente-archives", "2"): "aaaa"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("arquivo %s = %q, erro = %v, esperado %q", name, got, err, want)
		}
	}
	w.afterMove = nil
	if _, err := w.Write([]byte("dddd")); err != nil {
		t.Fatalf("não retomou: %v", err)
	}
}

func TestRotatingWriterNaoDivideRegistroGrande(t *testing.T) {
	path := filepath.Join(t.TempDir(), "assistente.log")
	w, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	for _, record := range []string{"a", "registro longo"} {
		if _, err := w.Write([]byte(record)); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "registro longo" {
		t.Fatalf("registro = %q, erro %v", got, err)
	}
}

func TestRotatingWriterGiraAposIntervaloMesmoSemAtingirLimite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w, err := NewRotatingWriter(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	w.configure(100, 2, 24*time.Hour)
	if _, err := w.Write([]byte("antes")); err != nil {
		t.Fatal(err)
	}
	w.started = time.Now().Add(-25 * time.Hour)
	if err := writeLogIdentity(path, filepath.Join(path+".assistente-archives", ".assistente-log"), w.started); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	w, err = NewRotatingWriter(path, 100, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := w.Close(); err != nil {
			t.Error(err)
		}
	}()
	w.configure(100, 2, 24*time.Hour)
	if _, err := w.Write([]byte("depois")); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{path: "depois", filepath.Join(path+".assistente-archives", "1"): "antes"} {
		got, err := os.ReadFile(name)
		if err != nil || string(got) != want {
			t.Fatalf("arquivo %s = %q, erro = %v", name, got, err)
		}
	}
}

func TestRotatingWriterImpedeConcorrenciaEntreInstancias(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	first, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	if second, err := NewRotatingWriter(path, 4, 1); err == nil {
		_ = second.Close()
		t.Fatal("duas instâncias gravando no mesmo arquivo")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := NewRotatingWriter(path, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = second.Close()
}
