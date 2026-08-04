package acp

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// managerComDiretorioPorConversa monta o manager com a escolha de diretório que
// o teste mandar, e devolve como trocá-la depois — que é o que a pessoa faz na
// tela, com a conversa já aberta.
func managerComDiretorioPorConversa(client *fakeManagedClient, inicial string) (*Manager, func(string), *int) {
	var mu sync.Mutex
	escolhido := inicial
	dials := 0
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("workspace"), nil },
		ConversationDir: func(string) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			return escolhido, nil
		},
		Dial: func(Config, RequestHandler) (Client, error) {
			dials++
			return client, nil
		},
	})
	return m, func(novo string) {
		mu.Lock()
		defer mu.Unlock()
		escolhido = novo
	}, &dials
}

// A conversa que escolheu um diretório coloca o agente lá, e não no workspace
// ativo: o alcance do agente é o que aquela conversa autorizou, e seguir o
// workspace faria a conversa passar a editar outro projeto sem ninguém pedir.
func TestConversaComDiretorioProprioAbreASessaoNele(t *testing.T) {
	escolhido := dirDeTeste("outro-projeto")
	client := newFakeManagedClient()
	m, _, _ := managerComDiretorioPorConversa(client, escolhido)
	defer m.Shutdown()

	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.sessions) != 1 {
		t.Fatalf("sessões abertas = %d, quer 1", len(client.sessions))
	}
	if got := client.sessions[0].cwd; !sameDir(got, escolhido) {
		t.Fatalf("a sessão abriu em %q, quer %q", got, escolhido)
	}
}

// Sem escolha, a conversa continua onde sempre esteve: no workspace ativo.
func TestConversaSemEscolhaSegueOWorkspace(t *testing.T) {
	client := newFakeManagedClient()
	m, _, _ := managerComDiretorioPorConversa(client, "")
	defer m.Shutdown()

	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if got := client.sessions[0].cwd; !sameDir(got, dirDeTeste("workspace")) {
		t.Fatalf("a sessão abriu em %q, quer o workspace ativo", got)
	}
}

// Trocar o diretório recria a sessão e custa a memória da conversa, exatamente
// como a troca de workspace já custava (AEP-0084 D5). O aviso precisa sair,
// porque o agente que responde o próximo turno não viveu o que está na tela.
func TestTrocarODiretorioRecriaASessaoEAvisaAPerdaDeMemoria(t *testing.T) {
	client := newFakeManagedClient()
	m, escolher, _ := managerComDiretorioPorConversa(client, dirDeTeste("projeto-a"))
	defer m.Shutdown()
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira montagem: %v", err)
	}
	primeira := conv.Session().ID()
	if conv.TakeLostMemoryNotice() {
		t.Fatal("a conversa que acabou de nascer avisou perda de memória")
	}

	escolher(dirDeTeste("projeto-b"))
	conv, err = m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("montagem depois da troca: %v", err)
	}

	if conv.Session().ID() == primeira {
		t.Fatal("a sessão sobreviveu à troca de diretório: o agente seguiria editando a árvore antiga")
	}
	if conv.Origin() != SessionRecreated {
		t.Fatalf("origem = %v, quer sessão recriada", conv.Origin())
	}
	if !conv.TakeLostMemoryNotice() {
		t.Fatal("a troca de diretório recriou a sessão sem contar que o agente perdeu o contexto")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.closedByID) == 0 || client.closedByID[0] != primeira {
		t.Fatalf("a sessão antiga não foi encerrada: encerradas = %v", client.closedByID)
	}
	if got := client.sessions[len(client.sessions)-1].cwd; !sameDir(got, dirDeTeste("projeto-b")) {
		t.Fatalf("a sessão nova abriu em %q, quer o diretório recém-escolhido", got)
	}
}

// Reescrever o mesmo diretório de outro jeito — com barra no fim, ou relativo —
// não é troca nenhuma. Recriar a sessão aqui cobraria a memória da conversa por
// uma diferença que só existe no texto.
func TestDiretorioEquivalenteNaoRecriaASessao(t *testing.T) {
	client := newFakeManagedClient()
	m, escolher, _ := managerComDiretorioPorConversa(client, dirDeTeste("projeto"))
	defer m.Shutdown()
	ctx := context.Background()

	conv, err := m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("primeira montagem: %v", err)
	}
	primeira := conv.Session().ID()

	escolher(dirDeTeste("projeto") + string(filepath.Separator))
	conv, err = m.Conversation(ctx, testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("segunda montagem: %v", err)
	}

	if conv.Session().ID() != primeira {
		t.Fatal("o mesmo diretório escrito de outro jeito recriou a sessão")
	}
	if conv.TakeLostMemoryNotice() {
		t.Fatal("o mesmo diretório escrito de outro jeito avisou perda de memória")
	}
}

// Não conseguir ler a escolha da conversa não pode virar "use o workspace":
// o diretório é o alcance do que o agente edita, e supor um faria o agente
// trabalhar numa árvore que ninguém autorizou.
func TestFalhaAoLerODiretorioDaConversaNaoAbreSessaoNoWorkspace(t *testing.T) {
	client := newFakeManagedClient()
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("workspace"), nil },
		ConversationDir: func(string) (string, error) {
			return "", errors.New("banco indisponível")
		},
		Dial: func(Config, RequestHandler) (Client, error) { return client, nil },
	})
	defer m.Shutdown()

	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err == nil {
		t.Fatal("a montagem seguiu sem saber em que diretório o agente pode mexer")
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.sessions) != 0 {
		t.Fatalf("sessões abertas = %d, quer nenhuma", len(client.sessions))
	}
}

// ConversationWorkDir é o que a barra da conversa mostra, e precisa dizer o
// mesmo caminho que a sessão recebeu: conferir o alcance do agente por um texto
// diferente do que valeu não confere nada.
func TestConversationWorkDirDizOMesmoCaminhoQueASessaoRecebeu(t *testing.T) {
	client := newFakeManagedClient()
	m, _, dials := managerComDiretorioPorConversa(client, "./projeto-relativo")
	defer m.Shutdown()

	mostrado, err := m.ConversationWorkDir("conv-1")
	if err != nil {
		t.Fatalf("diretório da conversa: %v", err)
	}
	if *dials != 0 {
		t.Fatal("perguntar onde o agente age subiu um processo de agente")
	}
	if m.ConversationSessionDir("conv-1") != "" {
		t.Fatal("conversa sem sessão de pé disse ter uma")
	}

	conv, err := m.Conversation(context.Background(), testSpec(), "conv-1")
	if err != nil {
		t.Fatalf("conversa: %v", err)
	}
	_ = conv

	client.mu.Lock()
	usado := client.sessions[0].cwd
	client.mu.Unlock()
	if mostrado != usado {
		t.Fatalf("a barra mostraria %q e a sessão abriu em %q", mostrado, usado)
	}
	if got := m.ConversationSessionDir("conv-1"); got != usado {
		t.Fatalf("diretório da sessão de pé = %q, quer %q", got, usado)
	}
}
