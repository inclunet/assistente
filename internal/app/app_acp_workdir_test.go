package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// appComAgenteEBanco é o app dos testes de diretório: o mesmo agente de mentira
// dos outros testes de ACP, agora com banco, porque a escolha de diretório mora
// na conversa e o caminho real passa por lê-la de lá.
//
// O workspace ativo é um diretório de verdade, e não um nome inventado: é ele
// que a conversa sem escolha usa, e o caminho de produção resolve caminhos antes
// de entregá-los ao agente.
func appComAgenteEBanco(t *testing.T, agente *agenteFalso) (*App, string) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir banco em memória: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Conversation{}, &database.ChatMessage{}); err != nil {
		t.Fatalf("migrar: %v", err)
	}
	anterior := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(anterior) })

	workspace := t.TempDir()
	a := &App{ctx: context.Background(), emitter: &testEmitter{}}
	a.setCurrentUserID("dono-1")
	a.acpMgr = acp.NewManager(acp.ManagerConfig{
		WorkDir:           func() (string, error) { return workspace, nil },
		ConversationDir:   a.agentConversationDir,
		OnSessionOptions:  a.agentSessionOptionsChanged,
		OnSessionCommands: a.agentSessionCommandsChanged,
		Dial: func(cfg acp.Config, _ acp.RequestHandler) (acp.Client, error) {
			agente.mu.Lock()
			agente.anuncia = cfg.OnConfigOptions
			agente.anunciaCmd = cfg.OnCommands
			agente.mu.Unlock()
			return agente, nil
		},
	})
	t.Cleanup(a.acpMgr.Shutdown)
	return a, workspace
}

// conversaNoBanco cria a conversa como o app cria, para que ler o diretório dela
// passe pelo mesmo registro que o turno lê.
func conversaNoBanco(t *testing.T, a *App, titulo string) string {
	t.Helper()
	ctx, err := a.requireAuthenticatedContext()
	if err != nil {
		t.Fatalf("contexto autenticado: %v", err)
	}
	conv := database.Conversation{UserID: "dono-1", Title: titulo}
	if err := database.DB().WithContext(ctx).Create(&conv).Error; err != nil {
		t.Fatalf("criar conversa: %v", err)
	}
	return conv.ID
}

// A conversa nasce seguindo o workspace ativo, e a tela precisa dizer isso: o
// diretório é o alcance do que o agente pode editar, e "nenhum" não é resposta.
func TestConversaNovaMostraODiretorioDoWorkspace(t *testing.T) {
	a, workspace := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")

	out, err := a.GetAgentConversationWorkDir(conversa)
	if err != nil {
		t.Fatalf("GetAgentConversationWorkDir: %v", err)
	}
	if !acp.SameDir(out.Dir, workspace) {
		t.Fatalf("diretório mostrado = %q, quer o workspace %q", out.Dir, workspace)
	}
	if !acp.SameDir(out.WorkspaceDir, workspace) {
		t.Fatalf("workspace mostrado = %q, quer %q", out.WorkspaceDir, workspace)
	}
	if out.Pinned {
		t.Error("a conversa que nunca escolheu diretório apareceu presa a um")
	}
	if out.SessionDir != "" || out.PendingRecreate() {
		t.Errorf("conversa sem sessão anunciou recriação pendente: %+v", out)
	}
}

// Escolher um diretório prende a conversa a ele e conta que a sessão de pé vai
// ser recriada — que é o momento em que o agente perde o que já foi conversado
// (AEP-0084 D5). Contar depois seria contar quando a pessoa já estranhou a
// resposta.
func TestEscolherDiretorioPrendeAConversaEAnunciaARecriacao(t *testing.T) {
	a, workspace := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")
	conversaComSessao(t, a, conversa)
	escolhido := t.TempDir()

	out, err := a.SetAgentConversationWorkDir(conversa, escolhido)
	if err != nil {
		t.Fatalf("SetAgentConversationWorkDir: %v", err)
	}
	if !acp.SameDir(out.Dir, escolhido) {
		t.Fatalf("diretório = %q, quer %q", out.Dir, escolhido)
	}
	if !out.Pinned {
		t.Error("a conversa que escolheu diretório não ficou presa a ele")
	}
	if !acp.SameDir(out.SessionDir, workspace) {
		t.Fatalf("a sessão de pé estaria em %q, quer o workspace de antes %q", out.SessionDir, workspace)
	}
	if !out.PendingRecreate() {
		t.Fatal("a troca de diretório não avisou que a sessão será recriada")
	}

	// O turno seguinte é quem recria: a sessão nasce no diretório novo e a
	// conversa passa a dizer que não há mais nada pendente.
	conversaComSessao(t, a, conversa)
	depois, err := a.GetAgentConversationWorkDir(conversa)
	if err != nil {
		t.Fatalf("GetAgentConversationWorkDir: %v", err)
	}
	if !acp.SameDir(depois.SessionDir, escolhido) {
		t.Fatalf("a sessão nova ficou em %q, quer %q", depois.SessionDir, escolhido)
	}
	if depois.PendingRecreate() {
		t.Error("a sessão já recriada ainda anuncia recriação pendente")
	}
}

// Voltar ao workspace ativo é escolha legítima, e é o caminho vazio: sem isso a
// conversa ficaria presa para sempre ao primeiro diretório escolhido.
func TestDiretorioVazioDevolveAConversaAoWorkspace(t *testing.T) {
	a, workspace := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")

	if _, err := a.SetAgentConversationWorkDir(conversa, t.TempDir()); err != nil {
		t.Fatalf("prender ao diretório: %v", err)
	}
	out, err := a.SetAgentConversationWorkDir(conversa, "  ")
	if err != nil {
		t.Fatalf("soltar do diretório: %v", err)
	}
	if out.Pinned {
		t.Error("a conversa continuou presa depois de voltar ao workspace")
	}
	if !acp.SameDir(out.Dir, workspace) {
		t.Fatalf("diretório = %q, quer o workspace %q", out.Dir, workspace)
	}
}

// Caminho que não existe é recusado na hora da escolha. Aceitá-lo trocaria um
// erro que a pessoa corrige agora por um agente dizendo que não encontra
// arquivo nenhum no próximo turno.
func TestDiretorioInexistenteEhRecusado(t *testing.T) {
	a, _ := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")
	inexistente := filepath.Join(t.TempDir(), "nao-existe")

	if _, err := a.SetAgentConversationWorkDir(conversa, inexistente); err == nil {
		t.Fatal("um caminho que não existe virou o alcance do agente")
	}
	out, err := a.GetAgentConversationWorkDir(conversa)
	if err != nil {
		t.Fatalf("GetAgentConversationWorkDir: %v", err)
	}
	if out.Pinned {
		t.Error("a escolha recusada foi gravada mesmo assim")
	}
}

// Arquivo não é diretório, e apontar o agente para um deixaria a sessão nascer
// num lugar em que ele não tem o que fazer.
func TestArquivoNaoServeComoDiretorio(t *testing.T) {
	a, _ := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")
	arquivo := filepath.Join(t.TempDir(), "anotacoes.md")
	if err := os.WriteFile(arquivo, []byte("nada"), 0o600); err != nil {
		t.Fatalf("criar arquivo: %v", err)
	}

	_, err := a.SetAgentConversationWorkDir(conversa, arquivo)
	if err == nil {
		t.Fatal("um arquivo foi aceito como diretório de trabalho")
	}
	if !strings.Contains(err.Error(), "não é um diretório") {
		t.Fatalf("o erro não explica o que houve: %v", err)
	}
}

// O caminho relativo que a pessoa digita vira absoluto antes de valer: é assim
// que a comparação da próxima montagem reconhece o mesmo diretório, e é o texto
// que a barra mostra como alcance do agente.
func TestCaminhoRelativoViraAbsoluto(t *testing.T) {
	a, _ := appComAgenteEBanco(t, novoAgenteFalso())
	conversa := conversaNoBanco(t, a, "Conversa")

	out, err := a.SetAgentConversationWorkDir(conversa, ".")
	if err != nil {
		t.Fatalf("SetAgentConversationWorkDir: %v", err)
	}
	if !filepath.IsAbs(out.Dir) {
		t.Fatalf("diretório gravado = %q, quer um caminho absoluto", out.Dir)
	}
}

// A escolha é da conversa, e não do app: outra conversa segue no workspace,
// senão trocar o alcance de uma trocaria o de todas.
func TestOutraConversaNaoHerdaODiretorioEscolhido(t *testing.T) {
	a, workspace := appComAgenteEBanco(t, novoAgenteFalso())
	primeira := conversaNoBanco(t, a, "Primeira")
	segunda := conversaNoBanco(t, a, "Segunda")

	if _, err := a.SetAgentConversationWorkDir(primeira, t.TempDir()); err != nil {
		t.Fatalf("prender a primeira: %v", err)
	}
	out, err := a.GetAgentConversationWorkDir(segunda)
	if err != nil {
		t.Fatalf("GetAgentConversationWorkDir: %v", err)
	}
	if !acp.SameDir(out.Dir, workspace) {
		t.Fatalf("a segunda conversa foi para %q, quer o workspace %q", out.Dir, workspace)
	}
}

// A escolha some junto com a sessão do agente se alguém conseguir gravá-la numa
// conversa de outra pessoa. Conversa que não é de quem pede não existe para ela.
func TestConversaDeOutraPessoaNaoAceitaEscolha(t *testing.T) {
	a, _ := appComAgenteEBanco(t, novoAgenteFalso())
	outra := database.Conversation{UserID: "dono-2", Title: "De outra pessoa"}
	if err := database.DB().Create(&outra).Error; err != nil {
		t.Fatalf("criar conversa alheia: %v", err)
	}

	if _, err := a.SetAgentConversationWorkDir(outra.ID, t.TempDir()); err == nil {
		t.Fatal("a escolha foi gravada numa conversa de outra pessoa")
	}
}
