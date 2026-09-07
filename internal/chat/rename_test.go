package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/core/ports"
)

// convRepoDeTitulo guarda o título da conversa como o banco guarda, para o teste
// olhar o que sobrou depois da renomeação.
type convRepoDeTitulo struct {
	conv      *Conversation
	erroAoLer error
	gravou    []string
}

func (r *convRepoDeTitulo) GetConversationInfo(context.Context, string) (*Conversation, error) {
	if r.erroAoLer != nil {
		return nil, r.erroAoLer
	}
	if r.conv == nil {
		return nil, nil
	}
	copia := *r.conv
	return &copia, nil
}

func (r *convRepoDeTitulo) UpdateConversation(_ context.Context, _ string, title, _ string) error {
	r.gravou = append(r.gravou, title)
	if r.conv != nil {
		r.conv.Title = title
	}
	return nil
}

func (r *convRepoDeTitulo) UpdateConversationChannel(context.Context, string, string, string) error {
	return nil
}

// repoDeMensagem responde pela mensagem do turno, que é como o interactor
// reconhece o rótulo provisório que ele mesmo escreveu.
type repoDeMensagem struct {
	MessageRepository
	mensagem *Message
}

func (r *repoDeMensagem) GetMessage(context.Context, string) (*Message, error) {
	if r.mensagem == nil {
		return nil, errors.New("mensagem não encontrada")
	}
	copia := *r.mensagem
	return &copia, nil
}

func interactorDeTitulo(em *spyEmitter, convRepo ConversationRepository, repo MessageRepository) *Interactor {
	return NewInteractor(InteractorConfig{
		Emitter:  em,
		ConvRepo: convRepo,
		Repo:     repo,
	})
}

func tituloRenomeado(t *testing.T, em *spyEmitter) (string, bool) {
	t.Helper()
	for _, e := range em.emitted {
		if e.name != "conversation:renamed" {
			continue
		}
		ev, ok := e.data.(ports.ConversationRenamedEvent)
		if !ok {
			t.Fatalf("conversation:renamed com carga inesperada: %T", e.data)
		}
		return ev.NewTitle, true
	}
	return "", false
}

// A conversa que nunca foi batizada aceita o nome do agente: "Nova Conversa" não
// é escolha de ninguém, é o que existe até aparecer coisa melhor.
func TestTituloDoAgenteSubstituiOPadrao(t *testing.T) {
	repo := &convRepoDeTitulo{conv: &Conversation{Title: DefaultConversationTitle}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, nil)

	if err := i.RenameFromAgent(context.Background(), "conv-1", "", "Corrigir o teste de anexos"); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if repo.conv.Title != "Corrigir o teste de anexos" {
		t.Fatalf("título gravado = %q", repo.conv.Title)
	}
	titulo, ok := tituloRenomeado(t, em)
	if !ok {
		t.Fatal("a renomeação não foi anunciada ao frontend")
	}
	if titulo != "Corrigir o teste de anexos" {
		t.Fatalf("título anunciado = %q", titulo)
	}
}

// O rótulo provisório é o recorte da primeira mensagem, e o app foi quem o
// escreveu: o nome do agente descreve melhor o mesmo pedido e pode tomar o lugar.
func TestTituloDoAgenteSubstituiORecorteDaMensagem(t *testing.T) {
	mensagem := &Message{ConversationID: "conv-1", Content: "arruma o teste que quebra quando o anexo vem vazio"}
	repo := &convRepoDeTitulo{conv: &Conversation{Title: automaticTitle(mensagem.Content)}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, &repoDeMensagem{mensagem: mensagem})

	if err := i.RenameFromAgent(context.Background(), "conv-1", "msg-1", "Corrigir teste de anexo vazio"); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if repo.conv.Title != "Corrigir teste de anexo vazio" {
		t.Fatalf("título gravado = %q", repo.conv.Title)
	}
	if _, ok := tituloRenomeado(t, em); !ok {
		t.Fatal("a renomeação não foi anunciada ao frontend")
	}
}

// Nome escolhido na tela é decisão de alguém, e o agente não a desfaz. Perder um
// título que a pessoa digitou é pior do que ficar com um rótulo pior.
func TestTituloEscolhidoNaoEhSubstituidoPeloAgente(t *testing.T) {
	mensagem := &Message{ConversationID: "conv-1", Content: "arruma o teste"}
	repo := &convRepoDeTitulo{conv: &Conversation{Title: "Entrega da sexta"}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, &repoDeMensagem{mensagem: mensagem})

	if err := i.RenameFromAgent(context.Background(), "conv-1", "msg-1", "Corrigir teste de anexo"); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if repo.conv.Title != "Entrega da sexta" {
		t.Fatalf("o título escolhido foi trocado por %q", repo.conv.Title)
	}
	if len(repo.gravou) != 0 {
		t.Fatalf("gravou título sem precisar: %v", repo.gravou)
	}
	if titulo, ok := tituloRenomeado(t, em); ok {
		t.Fatalf("anunciou renomeação que não houve: %q", titulo)
	}
}

// A mensagem de outra conversa não pode servir de prova de que este título é
// provisório: quem manda o identificador é o turno, e trocar de conversa no meio
// abriria um jeito de sobrescrever nome escolhido.
func TestTituloDeOutraConversaNaoAutorizaRenomear(t *testing.T) {
	mensagem := &Message{ConversationID: "conv-2", Content: "arruma o teste"}
	repo := &convRepoDeTitulo{conv: &Conversation{Title: automaticTitle(mensagem.Content)}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, &repoDeMensagem{mensagem: mensagem})

	if err := i.RenameFromAgent(context.Background(), "conv-1", "msg-1", "Corrigir teste"); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if repo.conv.Title == "Corrigir teste" {
		t.Fatal("o título foi trocado com base numa mensagem de outra conversa")
	}
}

// Título repetido não é renomeação: gravar e anunciar de novo faria a aba piscar
// e o leitor de telas repetir o mesmo nome a cada turno.
func TestTituloIgualNaoRenomeiaDeNovo(t *testing.T) {
	repo := &convRepoDeTitulo{conv: &Conversation{Title: "Corrigir teste"}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, nil)

	if err := i.RenameFromAgent(context.Background(), "conv-1", "", "Corrigir teste"); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if len(repo.gravou) != 0 {
		t.Fatalf("gravou o mesmo título de novo: %v", repo.gravou)
	}
	if _, ok := tituloRenomeado(t, em); ok {
		t.Fatal("anunciou renomeação para o mesmo título")
	}
}

// O agente manda o tamanho que quiser. O título vive em aba e em lista, e o corte
// tem que cair em fronteira de caractere: partir um acento ao meio deixaria lixo
// no fim do nome, inclusive no que o leitor de telas anuncia.
func TestTituloLongoEhCortadoEmFronteiraDeCaractere(t *testing.T) {
	repo := &convRepoDeTitulo{conv: &Conversation{Title: DefaultConversationTitle}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, nil)

	longo := strings.Repeat("ã", 80)
	if err := i.RenameFromAgent(context.Background(), "conv-1", "", longo); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	gravado := repo.conv.Title
	if !strings.HasSuffix(gravado, "…") {
		t.Fatalf("o título longo não foi marcado como cortado: %q", gravado)
	}
	if n := len([]rune(gravado)); n > maxAutomaticTitle {
		t.Fatalf("título com %d caracteres, teto é %d", n, maxAutomaticTitle)
	}
	if strings.ContainsRune(gravado, '\uFFFD') {
		t.Fatalf("o corte partiu um caractere ao meio: %q", gravado)
	}
}

// Título vazio (ou só espaço) não é nome nenhum: renomear para ele deixaria a aba
// sem rótulo, e sem rótulo o leitor de telas não tem o que anunciar.
func TestTituloVazioNaoRenomeia(t *testing.T) {
	repo := &convRepoDeTitulo{conv: &Conversation{Title: DefaultConversationTitle}}
	em := &spyEmitter{}
	i := interactorDeTitulo(em, repo, nil)

	if err := i.RenameFromAgent(context.Background(), "conv-1", "", "   "); err != nil {
		t.Fatalf("renomear: %v", err)
	}

	if len(repo.gravou) != 0 {
		t.Fatalf("gravou título vazio: %v", repo.gravou)
	}
}
