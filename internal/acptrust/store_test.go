package acptrust

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func lojaEmDiretorioTemporario(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	return NewStoreWithDir(dir), dir
}

func TestOQueFoiAutorizadoParaSempreVoltaNaProximaVez(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if loja.Allows("cursor", "execute") {
		t.Fatal("autorizou antes de alguém ter autorizado")
	}
	if err := loja.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if !loja.Allows("cursor", "execute") {
		t.Error("a autorização permanente não sobreviveu à gravação")
	}
}

func TestAAutorizacaoDeUmPerfilNaoValeNoOutro(t *testing.T) {
	// O perfil é o que descreve com qual agente se está falando e sob quais
	// regras. Vazar autorização entre perfis liberaria uma ação que ninguém
	// autorizou ali.
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if loja.Allows("claude-code", "execute") {
		t.Error("a autorização de um perfil apareceu em outro")
	}
}

func TestClasseDiferenteContinuaPedindoPermissao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Allow("cursor", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if loja.Allows("cursor", "execute") {
		t.Error("autorizar leitura liberou execução de comando")
	}
}

func TestAutorizarDeNovoNaoDuplicaAEntrada(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	for i := 0; i < 3; i++ {
		if err := loja.Allow("cursor", "edit"); err != nil {
			t.Fatalf("autorizar: %v", err)
		}
	}

	if entradas := loja.List("cursor"); len(entradas) != 1 {
		t.Errorf("entradas = %d, quer 1: a lista de revogação viraria repetição", len(entradas))
	}
}

func TestRevogarFazOAgentePerguntarDeNovo(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)
	if err := loja.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if err := loja.Revoke("cursor", "execute"); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	if loja.Allows("cursor", "execute") {
		t.Error("a ação continuou liberada depois de revogada")
	}
}

func TestRevogarOQueNinguemAutorizouNaoFingeSucesso(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Revoke("cursor", "execute"); err != ErrEntryNotFound {
		t.Errorf("erro = %v, quer %v", err, ErrEntryNotFound)
	}
}

func TestClasseEscritaDeOutroJeitoEhAMesmaAutorizacao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)
	if err := loja.Allow("cursor", " Execute "); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if !loja.Allows("cursor", "execute") {
		t.Error("a mesma classe escrita de outro jeito virou outra autorização")
	}
}

func TestPerfilComNomePerigosoNaoEscapaDoDiretorio(t *testing.T) {
	loja, dir := lojaEmDiretorioTemporario(t)

	if err := loja.Allow("../../fora", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, subdir)); err != nil {
		t.Fatalf("nada foi gravado no diretório da loja: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "..", "fora")); err == nil {
		t.Error("o slug do perfil escreveu fora do diretório")
	}
}

func TestArquivoIlegivelNaoLiberaNemApagaNada(t *testing.T) {
	// Diante de um arquivo corrompido, perguntar de novo é a única resposta
	// segura — e gravar por cima apagaria autorizações que ainda estão lá.
	loja, dir := lojaEmDiretorioTemporario(t)
	caminho := filepath.Join(dir, subdir, "profile-cursor.json")
	if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
		t.Fatalf("preparar diretório: %v", err)
	}
	if err := os.WriteFile(caminho, []byte("{isto não é json"), 0o600); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	if loja.Allows("cursor", "execute") {
		t.Error("arquivo ilegível liberou a ação")
	}
	if err := loja.Allow("cursor", "execute"); err == nil {
		t.Error("gravou por cima de um arquivo que não conseguiu ler")
	}
	if data, err := os.ReadFile(caminho); err != nil || string(data) != "{isto não é json" {
		t.Errorf("o arquivo original foi alterado: %q, %v", string(data), err)
	}
}

func TestQuemRevogaVeTodosOsPerfisQueAutorizaramAlgo(t *testing.T) {
	// Uma autorização esquecida num perfil que não se usa há meses continua
	// valendo no dia em que ele voltar.
	loja, dir := lojaEmDiretorioTemporario(t)
	if err := loja.Allow("cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if err := loja.Allow("claude-code", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	// Arquivo alheio no mesmo diretório não vira perfil.
	if err := os.WriteFile(filepath.Join(dir, subdir, "outra-coisa.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	perfis := loja.Profiles()

	if len(perfis) != 2 {
		t.Fatalf("perfis = %q, quer os dois que autorizaram algo", perfis)
	}
	for _, esperado := range []string{"cursor", "claude-code"} {
		if !slices.Contains(perfis, esperado) {
			t.Errorf("perfil %q ficou de fora da lista", esperado)
		}
	}
}

func TestSemNadaAutorizadoNaoHaPerfilNenhum(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if perfis := loja.Profiles(); len(perfis) != 0 {
		t.Errorf("perfis = %q, quer nenhum", perfis)
	}
}

func TestSemPerfilNaoHaOndeGuardarAutorizacao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if loja.Allows("", "execute") {
		t.Error("autorizou sem saber de qual perfil")
	}
	if err := loja.Allow("", "execute"); err == nil {
		t.Error("gravou uma autorização sem perfil")
	}
}
