package acptrust

import (
	"assistente/internal/testutil/authorizationfixture"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func lojaEmDiretorioTemporario(t *testing.T) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	return NewStore(authorizationfixture.Options(t, dir, "")), dir
}

func TestLegacyACPDoesNotImportWorkspaceOrGlobalGrants(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(authorizationfixture.Options(t, dir, dir))
	for _, scope := range []string{"workspace", "global"} {
		path := filepath.Join(dir, subdir, scope+".json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(`{"version":1,"entries":[{"kind":"execute","scope":"`+scope+`"}]}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if allowed, err := store.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || allowed {
		t.Fatalf("origem ACP inexistente no legado concedeu: %v %v", allowed, err)
	}
}

func TestLegacyACPResolvesOnlyUnambiguousProfileIdentity(t *testing.T) {
	for _, keys := range [][]string{{"dev.local"}, {"dev.local", "devlocal"}, {}} {
		t.Run(strings.Join(keys, ","), func(t *testing.T) {
			store, dir := lojaEmDiretorioTemporario(t)
			store.profiles = func() ([]string, error) { return keys, nil }
			path := filepath.Join(dir, subdir, "profile-devlocal.json")
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(`{"version":1,"entries":[{"kind":"execute"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			profiles, err := store.Profiles(authorizationfixture.Context())
			if len(keys) != 1 {
				if err == nil {
					t.Fatal("origem ambígua ou desconhecida foi importada")
				}
				for _, key := range []string{"dev.local", "devlocal"} {
					if allowed, err := store.Allows(authorizationfixture.Context(), key, "execute"); err == nil || allowed {
						t.Fatalf("perfil %s recebeu concessão: %v %v", key, allowed, err)
					}
				}
				return
			}
			if err != nil || !slices.Equal(profiles, keys) {
				t.Fatalf("perfil original perdido: %v %v", profiles, err)
			}
			if allowed, err := store.Allows(authorizationfixture.Context(), "dev.local", "execute"); err != nil || !allowed {
				t.Fatalf("concessão original perdida: %v %v", allowed, err)
			}
			if allowed, _ := store.Allows(authorizationfixture.Context(), "devlocal", "execute"); allowed {
				t.Fatal("concessão transferida ao alias")
			}
		})
	}
}

func TestOQueFoiAutorizadoParaSempreVoltaNaProximaVez(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || allowed {
		t.Fatal("autorizou antes de alguém ter autorizado")
	}
	if err := loja.Allow(authorizationfixture.Context(), "cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || !allowed {
		t.Error("a autorização permanente não sobreviveu à gravação")
	}
}

func TestAAutorizacaoDeUmPerfilNaoValeNoOutro(t *testing.T) {
	// O perfil é o que descreve com qual agente se está falando e sob quais
	// regras. Vazar autorização entre perfis liberaria uma ação que ninguém
	// autorizou ali.
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Allow(authorizationfixture.Context(), "cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if allowed, err := loja.Allows(authorizationfixture.Context(), "claude-code", "execute"); err != nil || allowed {
		t.Error("a autorização de um perfil apareceu em outro")
	}
}

func TestClasseDiferenteContinuaPedindoPermissao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Allow(authorizationfixture.Context(), "cursor", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || allowed {
		t.Error("autorizar leitura liberou execução de comando")
	}
}

func TestAutorizarDeNovoNaoDuplicaAEntrada(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	for i := 0; i < 3; i++ {
		if err := loja.Allow(authorizationfixture.Context(), "cursor", "edit"); err != nil {
			t.Fatalf("autorizar: %v", err)
		}
	}

	if entradas, err := loja.List(authorizationfixture.Context(), "cursor"); err != nil || len(entradas) != 1 {
		t.Errorf("entradas = %d, quer 1: a lista de revogação viraria repetição", len(entradas))
	}
}

func TestRevogarFazOAgentePerguntarDeNovo(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)
	if err := loja.Allow(authorizationfixture.Context(), "cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if err := loja.Revoke(authorizationfixture.Context(), "cursor", "execute"); err != nil {
		t.Fatalf("revogar: %v", err)
	}

	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || allowed {
		t.Error("a ação continuou liberada depois de revogada")
	}
}

func TestRevogarOQueNinguemAutorizouNaoFingeSucesso(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if err := loja.Revoke(authorizationfixture.Context(), "cursor", "execute"); err != ErrEntryNotFound {
		t.Errorf("erro = %v, quer %v", err, ErrEntryNotFound)
	}
}

func TestClasseEscritaDeOutroJeitoEhAMesmaAutorizacao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)
	if err := loja.Allow(authorizationfixture.Context(), "cursor", " Execute "); err != nil {
		t.Fatalf("autorizar: %v", err)
	}

	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err != nil || !allowed {
		t.Error("a mesma classe escrita de outro jeito virou outra autorização")
	}
}

func TestPerfilComNomePerigosoNaoRecebeAutorizacao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)
	if err := loja.Allow(authorizationfixture.Context(), "../../fora", "execute"); err == nil {
		t.Fatal("aceitou identidade de perfil inválida")
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

	if allowed, err := loja.Allows(authorizationfixture.Context(), "cursor", "execute"); err == nil || allowed {
		t.Error("arquivo ilegível liberou a ação")
	}
	if err := loja.Allow(authorizationfixture.Context(), "cursor", "execute"); err == nil {
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
	if err := loja.Allow(authorizationfixture.Context(), "cursor", "execute"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	if err := loja.Allow(authorizationfixture.Context(), "claude-code", "read"); err != nil {
		t.Fatalf("autorizar: %v", err)
	}
	// Arquivo alheio no mesmo diretório não vira perfil.
	if err := os.MkdirAll(filepath.Join(dir, subdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, subdir, "outra-coisa.json"), []byte("{}"), 0o600); err != nil {
		t.Fatalf("preparar arquivo: %v", err)
	}

	perfis, err := loja.Profiles(authorizationfixture.Context())
	if err != nil {
		t.Fatal(err)
	}

	if len(perfis) != 2 {
		t.Fatalf("perfis = %q, quer os dois que autorizaram algo", perfis)
	}
	for _, esperado := range []string{"cursor", "claude-code"} {
		if !slices.Contains(perfis, esperado) {
			t.Errorf("perfil %q ficou de fora da lista", esperado)
		}
	}
}

func TestAChaveDoPerfilEhAMesmaQueNomeiaOArquivo(t *testing.T) {
	// Quem cruza a lista de perfis com as autorizações compara por aqui.
	if chave := ProfileKey(" Cursor "); chave != "cursor" {
		t.Errorf("chave = %q, quer a forma que nomeia o arquivo", chave)
	}
	if chave := ProfileKey("../fora"); chave != "" {
		t.Errorf("chave = %q, quer rejeitar identidade inválida", chave)
	}
}

func TestSemNadaAutorizadoNaoHaPerfilNenhum(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if perfis, err := loja.Profiles(authorizationfixture.Context()); err != nil || len(perfis) != 0 {
		t.Errorf("perfis = %q, quer nenhum", perfis)
	}
}

func TestSemPerfilNaoHaOndeGuardarAutorizacao(t *testing.T) {
	loja, _ := lojaEmDiretorioTemporario(t)

	if allowed, err := loja.Allows(authorizationfixture.Context(), "", "execute"); err == nil || allowed {
		t.Error("autorizou sem saber de qual perfil")
	}
	if err := loja.Allow(authorizationfixture.Context(), "", "execute"); err == nil {
		t.Error("gravou uma autorização sem perfil")
	}
}
