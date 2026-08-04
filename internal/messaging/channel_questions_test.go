package messaging

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/questionnaire"
)

// canalGravado é o mensageiro do outro lado: guarda o que foi enviado em vez de
// falar com o Telegram.
type canalGravado struct {
	mu       sync.Mutex
	enviadas []mensagemDeCanal
	erro     error
	chegou   chan struct{}
}

type mensagemDeCanal struct {
	canal  string
	chatID string
	texto  string
}

func novoCanalGravado() *canalGravado {
	return &canalGravado{chegou: make(chan struct{}, 4)}
}

func (c *canalGravado) enviar(_ context.Context, canal, chatID, texto string) error {
	c.mu.Lock()
	if c.erro != nil {
		err := c.erro
		c.mu.Unlock()
		return err
	}
	c.enviadas = append(c.enviadas, mensagemDeCanal{canal: canal, chatID: chatID, texto: texto})
	c.mu.Unlock()
	select {
	case c.chegou <- struct{}{}:
	default:
	}
	return nil
}

func (c *canalGravado) esperarMensagem(tb testing.TB) mensagemDeCanal {
	tb.Helper()
	select {
	case <-c.chegou:
	case <-time.After(2 * time.Second):
		tb.Fatal("nada foi enviado ao canal")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enviadas[len(c.enviadas)-1]
}

func (c *canalGravado) quantasMensagens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.enviadas)
}

func (c *canalGravado) ultimaMensagem(tb testing.TB) mensagemDeCanal {
	tb.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.enviadas) == 0 {
		tb.Fatal("nada foi enviado ao canal")
	}
	return c.enviadas[len(c.enviadas)-1]
}

// mecanismoDePergunta monta o mecanismo com prazo de teste.
func mecanismoDePergunta(prazo time.Duration) (*ChannelQuestions, *canalGravado) {
	canal := novoCanalGravado()
	perguntas := newChannelQuestions(canal.enviar)
	perguntas.timeout = prazo
	return perguntas, canal
}

func superficieDeCanal() questionnaire.Surface {
	return questionnaire.ChannelSurface("conversa-1", "telegram", "contato-1")
}

// pedidoDePermissao é o diálogo que o handler de permissão monta: o que está em
// jogo, o bloco com a ação pedida e a decisão de uma escolha.
func pedidoDePermissao() questionnaire.RequestPayload {
	return questionnaire.RequestPayload{
		Title:       questionnaire.Keyed("app.x.title", "O agente pede permissão"),
		Description: questionnaire.Keyed("app.x.description", "O agente quer executar uma ação na sua máquina."),
		AllowCancel: true,
		Questions: []questionnaire.Question{
			{
				ID:      "action",
				Type:    "readonly_code",
				Prompt:  questionnaire.Keyed("app.x.actionPrompt", "Ação pedida"),
				Content: "npm install",
			},
			{
				ID:       "decision",
				Type:     "single_choice",
				Prompt:   questionnaire.Keyed("app.x.choicePrompt", "O que o agente pode fazer?"),
				Options:  questionnaire.PlainTexts([]string{"Permitir uma vez", "Negar"}),
				Required: true,
			},
		},
	}
}

// perguntaEmVoo dispara a pergunta e devolve por onde a resposta dela chega.
func perguntaEmVoo(tb testing.TB, perguntas *ChannelQuestions, payload questionnaire.RequestPayload) chan resultadoDePergunta {
	tb.Helper()
	pronto := make(chan resultadoDePergunta, 1)
	go func() {
		resp, err := perguntas.AskOnChannel(context.Background(), superficieDeCanal(), payload)
		pronto <- resultadoDePergunta{resp: resp, err: err}
	}()
	return pronto
}

type resultadoDePergunta struct {
	resp questionnaire.Response
	err  error
}

func esperarResultado(tb testing.TB, pronto chan resultadoDePergunta) resultadoDePergunta {
	tb.Helper()
	select {
	case res := <-pronto:
		return res
	case <-time.After(5 * time.Second):
		tb.Fatal("a pergunta no canal não voltou: o turno do agente ficaria pendurado")
		return resultadoDePergunta{}
	}
}

func TestAPerguntaVaiParaOCanalComOpcoesNumeradasEPrazo(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Minute)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())

	mensagem := canal.esperarMensagem(t)
	if mensagem.canal != "telegram" || mensagem.chatID != "contato-1" {
		t.Errorf("destino = %q/%q, quer o canal e o contato da conversa", mensagem.canal, mensagem.chatID)
	}
	for _, trecho := range []string{
		"O agente pede permissão",
		"O agente quer executar uma ação na sua máquina.",
		"npm install",
		"1 - Permitir uma vez",
		"2 - Negar",
		"Responda com o número da opção",
		"1 minuto",
	} {
		if !strings.Contains(mensagem.texto, trecho) {
			t.Errorf("a mensagem não trouxe %q:\n%s", trecho, mensagem.texto)
		}
	}

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "1"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	resultado := esperarResultado(t, pronto)
	if resultado.err != nil {
		t.Fatalf("erro inesperado: %v", resultado.err)
	}
	if resultado.resp.Answers["decision"] != "Permitir uma vez" {
		t.Errorf("resposta = %v, quer a primeira opção", resultado.resp.Answers)
	}
	if perguntas.PendingCount() != 0 {
		t.Error("a pergunta respondida continuou pendente")
	}
}

func TestNegarPeloCanalVoltaComoEscolha(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Minute)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	resultado := esperarResultado(t, pronto)
	if resultado.err != nil {
		t.Fatalf("erro inesperado: %v", resultado.err)
	}
	if resultado.resp.Answers["decision"] != "Negar" {
		t.Errorf("resposta = %v, quer a negativa", resultado.resp.Answers)
	}
}

func TestSoODonoDaConversaDecide(t *testing.T) {
	// Resposta de terceiro é decisão de ignorar, e não efeito de não achar a
	// pergunta: ela continua pendente esperando quem pode decidir.
	perguntas, canal := mecanismoDePergunta(2 * time.Second)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "intruso", "1"); res != AnswerIgnored {
		t.Fatalf("resultado = %v, quer a mensagem ignorada", res)
	}
	if perguntas.PendingCount() != 1 {
		t.Fatal("a resposta de terceiro fechou a pergunta")
	}
	if canal.quantasMensagens() != 1 {
		t.Errorf("mensagens no canal = %d, quer só a pergunta: nada é respondido a quem não é dono", canal.quantasMensagens())
	}

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta do dono entregue", res)
	}
	resultado := esperarResultado(t, pronto)
	if resultado.err != nil {
		t.Fatalf("erro inesperado: %v", resultado.err)
	}
	if resultado.resp.Answers["decision"] != "Negar" {
		t.Errorf("resposta = %v, quer a que o dono mandou", resultado.resp.Answers)
	}
}

func TestRespostaSemNumeroPedeDeNovoSemFecharAPergunta(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(2 * time.Second)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	for _, resposta := range []string{"sim", "", "3", "0", "permitir"} {
		if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", resposta); res != AnswerIgnored {
			t.Errorf("resposta %q: resultado = %v, quer ignorada", resposta, res)
		}
	}
	if perguntas.PendingCount() != 1 {
		t.Fatal("resposta que não nomeia opção fechou a pergunta")
	}
	if recado := canal.ultimaMensagem(t); !strings.Contains(recado.texto, "número da opção") {
		t.Errorf("recado = %q, quer explicar como responder", recado.texto)
	}

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "1."); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	if resultado := esperarResultado(t, pronto); resultado.err != nil {
		t.Fatalf("erro inesperado: %v", resultado.err)
	}
}

func TestSemRespostaNoPrazoOPedidoEhNegadoEAPessoaEhInformada(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(50 * time.Millisecond)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	resultado := esperarResultado(t, pronto)
	if !errors.Is(resultado.err, ErrChannelQuestionExpired) {
		t.Fatalf("erro = %v, quer %v", resultado.err, ErrChannelQuestionExpired)
	}
	if resultado.resp.Answers != nil {
		t.Errorf("resposta = %v, quer nenhuma", resultado.resp.Answers)
	}
	aviso := canal.esperarMensagem(t)
	if !strings.Contains(aviso.texto, "negado") {
		t.Errorf("aviso = %q, quer contar que o pedido foi negado", aviso.texto)
	}
	if perguntas.PendingCount() != 0 {
		t.Error("a pergunta expirada continuou pendente")
	}
	// Depois do prazo a conversa volta a ser conversa: a mensagem seguinte é
	// turno normal, e não resposta de uma pergunta que já não existe.
	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "1"); res != AnswerNotPending {
		t.Errorf("resultado = %v, quer mensagem comum", res)
	}
}

func TestPerguntaSemPrazoUsaOPrazoCurtoDoCanal(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(0)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())

	mensagem := canal.esperarMensagem(t)
	if !strings.Contains(mensagem.texto, "3 minutos") {
		t.Errorf("a mensagem não anunciou o prazo do canal:\n%s", mensagem.texto)
	}
	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	esperarResultado(t, pronto)
}

func TestMensagemDeOutraConversaSegueSeuCaminho(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(2 * time.Second)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	if res := perguntas.TryAnswer(context.Background(), "conversa-2", "contato-1", "1"); res != AnswerNotPending {
		t.Errorf("resultado = %v, quer mensagem comum: a pergunta é de outra conversa", res)
	}
	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "1"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	esperarResultado(t, pronto)
}

func TestSimOuNaoViraDuasOpcoesNumeradas(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Minute)
	payload := questionnaire.RequestPayload{
		Title: questionnaire.Plain("Rodar comando?"),
		Questions: []questionnaire.Question{
			{ID: "cmd", Type: "readonly_code", Content: "rm -rf build"},
			{ID: "aprovar", Type: "boolean", Prompt: questionnaire.Plain("Autoriza?"), Required: true},
		},
	}
	pronto := perguntaEmVoo(t, perguntas, payload)

	mensagem := canal.esperarMensagem(t)
	if !strings.Contains(mensagem.texto, "1 - Sim") || !strings.Contains(mensagem.texto, "2 - Não") {
		t.Errorf("a mensagem não numerou o sim e o não:\n%s", mensagem.texto)
	}

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta entregue", res)
	}
	resultado := esperarResultado(t, pronto)
	if resultado.resp.Answers["aprovar"] != false {
		t.Errorf("resposta = %v, quer o booleano falso que o diálogo espera", resultado.resp.Answers)
	}
}

func TestOTextoDoAgenteNaoVaiCruParaOCanal(t *testing.T) {
	// A mensagem fica gravada no histórico de um app de terceiro: escape de
	// terminal e espaço invisível não têm o que fazer ali, e o invisível é como
	// se esconde conteúdo de quem está decidindo. A marca de direção é pior que
	// esconder: com ela o comando aparece de trás para frente, e quem autoriza
	// leu outra coisa.
	perguntas, canal := mecanismoDePergunta(time.Minute)
	payload := pedidoDePermissao()
	payload.Questions[0].Content = "\x1b[31mrm -rf /\x1b[0m\u200b oculto\u202e\u2066\u200f\nsegunda linha"
	pronto := perguntaEmVoo(t, perguntas, payload)

	mensagem := canal.esperarMensagem(t)
	for _, proibido := range []string{"\x1b", "\u200b", "\u202e", "\u2066", "\u200f"} {
		if strings.Contains(mensagem.texto, proibido) {
			t.Errorf("a mensagem levou %q para o canal:\n%q", proibido, mensagem.texto)
		}
	}
	if !strings.Contains(mensagem.texto, "segunda linha") {
		t.Error("o saneamento comeu a quebra de linha do bloco")
	}

	perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2")
	esperarResultado(t, pronto)
}

func TestBlocoLongoEhCortadoSemLevarAsOpcoesEmbora(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Minute)
	payload := pedidoDePermissao()
	payload.Questions[0].Content = strings.Repeat("comando muito longo ", 500)
	pronto := perguntaEmVoo(t, perguntas, payload)

	mensagem := canal.esperarMensagem(t)
	if len([]rune(mensagem.texto)) > channelMessageBudget {
		t.Errorf("a mensagem tem %d runas, quer no máximo %d", len([]rune(mensagem.texto)), channelMessageBudget)
	}
	if !strings.Contains(mensagem.texto, "texto cortado") {
		t.Error("cortou o bloco sem dizer que ele não veio inteiro")
	}
	for _, trecho := range []string{"1 - Permitir uma vez", "2 - Negar", "Responda com o número"} {
		if !strings.Contains(mensagem.texto, trecho) {
			t.Errorf("o corte levou embora %q: ninguém teria como responder", trecho)
		}
	}

	perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2")
	esperarResultado(t, pronto)
}

func TestVariosBlocosCortadosNaoEstouramOTetoDaMensagem(t *testing.T) {
	// Cada corte acrescenta a marca de "texto cortado", e ela é parte do bloco:
	// cobrá-la fora do orçamento faria a mensagem passar do teto justamente no
	// diálogo com mais de um bloco longo — que é o que a plataforma recusa.
	perguntas, canal := mecanismoDePergunta(time.Minute)
	payload := pedidoDePermissao()
	longo := strings.Repeat("comando muito longo ", 500)
	payload.Questions[0].Content = longo
	extras := []questionnaire.Question{
		{ID: "arquivo", Type: "readonly_code", Prompt: questionnaire.Plain("Arquivo"), Content: longo},
		{ID: "diff", Type: "readonly_code", Prompt: questionnaire.Plain("Alteração"), Content: longo},
	}
	payload.Questions = append(payload.Questions[:1], append(extras, payload.Questions[1])...)
	pronto := perguntaEmVoo(t, perguntas, payload)

	mensagem := canal.esperarMensagem(t)
	if tamanho := len([]rune(mensagem.texto)); tamanho > channelMessageBudget {
		t.Errorf("a mensagem tem %d runas, quer no máximo %d", tamanho, channelMessageBudget)
	}
	if cortes := strings.Count(mensagem.texto, channelTruncatedMark); cortes != len(extras)+1 {
		t.Errorf("marcas de corte = %d, quer uma por bloco longo (%d)", cortes, len(extras)+1)
	}
	for _, trecho := range []string{"1 - Permitir uma vez", "2 - Negar", "Responda com o número"} {
		if !strings.Contains(mensagem.texto, trecho) {
			t.Errorf("o corte levou embora %q: ninguém teria como responder", trecho)
		}
	}

	perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "2")
	esperarResultado(t, pronto)
}

func TestDialogoQueNaoCabeNumaMensagemNaoEhPerguntado(t *testing.T) {
	casos := map[string][]questionnaire.Question{
		"texto livre obrigatório": {
			{ID: "porque", Type: "text", Prompt: questionnaire.Plain("Por quê?"), Required: true},
			{ID: "decision", Type: "boolean", Prompt: questionnaire.Plain("Pode?")},
		},
		"duas decisões": {
			{ID: "a", Type: "boolean", Prompt: questionnaire.Plain("Pode?")},
			{ID: "b", Type: "single_choice", Prompt: questionnaire.Plain("Qual?"), Options: questionnaire.PlainTexts([]string{"x", "y"})},
		},
		"nenhuma decisão": {
			{ID: "aviso", Type: "readonly_code", Content: "só leitura"},
		},
		"escolha sem opção nenhuma": {
			{ID: "decision", Type: "single_choice", Prompt: questionnaire.Plain("Qual?")},
		},
		// A pergunta do agente que aceita marcar mais de uma opção: um número
		// não diz quantas nem quais, e aceitar uma lista escrita à mão faria uma
		// resposta parecida decidir por aproximação.
		"escolha múltipla": {
			{ID: "prompt", Type: "readonly_code", Content: "Quais provedores habilitar?"},
			{ID: "resposta", Type: "multiple_choice", Prompt: questionnaire.Plain("Sua resposta"),
				Options: questionnaire.PlainTexts([]string{"Google", "GitHub"})},
		},
	}
	for nome, questoes := range casos {
		t.Run(nome, func(t *testing.T) {
			perguntas, canal := mecanismoDePergunta(time.Minute)
			_, err := perguntas.AskOnChannel(context.Background(), superficieDeCanal(),
				questionnaire.RequestPayload{Questions: questoes})
			if !errors.Is(err, ErrChannelQuestionUnsupported) {
				t.Fatalf("erro = %v, quer %v", err, ErrChannelQuestionUnsupported)
			}
			// Quem perguntou trata isso como "a pergunta não apareceu", e não
			// como "ninguém respondeu a tempo".
			if !errors.Is(err, questionnaire.ErrAskerUnavailable) {
				t.Errorf("erro = %v, quer embrulhar %v", err, questionnaire.ErrAskerUnavailable)
			}
			if canal.quantasMensagens() != 0 {
				t.Error("mandou ao canal uma pergunta que não tem como ser respondida")
			}
			if perguntas.PendingCount() != 0 {
				t.Error("deixou pendente uma pergunta que nunca foi feita")
			}
		})
	}
}

func TestSegundaPerguntaNaMesmaConversaNaoAtropelaAPrimeira(t *testing.T) {
	// Duas perguntas abertas na mesma conversa fariam o "1" valer para as duas.
	perguntas, canal := mecanismoDePergunta(2 * time.Second)
	pronto := perguntaEmVoo(t, perguntas, pedidoDePermissao())
	canal.esperarMensagem(t)

	_, err := perguntas.AskOnChannel(context.Background(), superficieDeCanal(), pedidoDePermissao())
	if !errors.Is(err, ErrChannelQuestionBusy) {
		t.Fatalf("erro = %v, quer %v", err, ErrChannelQuestionBusy)
	}
	if canal.quantasMensagens() != 1 {
		t.Errorf("mensagens = %d, quer só a primeira pergunta", canal.quantasMensagens())
	}

	if res := perguntas.TryAnswer(context.Background(), "conversa-1", "contato-1", "1"); res != AnswerDelivered {
		t.Fatalf("resultado = %v, quer a resposta da primeira pergunta", res)
	}
	esperarResultado(t, pronto)
}

func TestPerguntaQueNaoChegaAoCanalNegaNaHora(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Hour)
	canal.erro = fmt.Errorf("mensageiro fora do ar")

	_, err := perguntas.AskOnChannel(context.Background(), superficieDeCanal(), pedidoDePermissao())
	if !errors.Is(err, ErrChannelQuestionUndeliverable) {
		t.Fatalf("erro = %v, quer %v", err, ErrChannelQuestionUndeliverable)
	}
	if !errors.Is(err, questionnaire.ErrAskerUnavailable) {
		t.Errorf("erro = %v, quer embrulhar %v", err, questionnaire.ErrAskerUnavailable)
	}
	if perguntas.PendingCount() != 0 {
		t.Error("pergunta que não foi enviada ficou esperando resposta")
	}
}

func TestTurnoCanceladoTiraAPerguntaDoCanal(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Hour)
	ctx, cancelar := context.WithCancel(context.Background())
	pronto := make(chan resultadoDePergunta, 1)
	go func() {
		resp, err := perguntas.AskOnChannel(ctx, superficieDeCanal(), pedidoDePermissao())
		pronto <- resultadoDePergunta{resp: resp, err: err}
	}()
	canal.esperarMensagem(t)

	cancelar()
	resultado := esperarResultado(t, pronto)
	if !errors.Is(resultado.err, context.Canceled) {
		t.Fatalf("erro = %v, quer o cancelamento", resultado.err)
	}
	if errors.Is(resultado.err, ErrChannelQuestionExpired) {
		t.Error("cancelamento virou prazo estourado: são coisas diferentes")
	}
	if perguntas.PendingCount() != 0 {
		t.Error("a pergunta do turno cancelado continuou pendente")
	}
	if canal.quantasMensagens() != 1 {
		t.Error("avisou de prazo numa pergunta que foi cancelada")
	}
}

func TestSuperficieQueNaoEhCanalNaoViraMensagem(t *testing.T) {
	perguntas, canal := mecanismoDePergunta(time.Minute)
	for nome, superficie := range map[string]questionnaire.Surface{
		"tela":             questionnaire.DesktopSurface("conversa-1"),
		"sem interlocutor": questionnaire.NoSurface("conversa-1"),
	} {
		t.Run(nome, func(t *testing.T) {
			_, err := perguntas.AskOnChannel(context.Background(), superficie, pedidoDePermissao())
			if !errors.Is(err, questionnaire.ErrNoInterlocutor) {
				t.Fatalf("erro = %v, quer %v", err, questionnaire.ErrNoInterlocutor)
			}
			if canal.quantasMensagens() != 0 {
				t.Error("mandou para o canal a pergunta de outra superfície")
			}
		})
	}
}

func TestSemMensageiroNaoHaComoPerguntar(t *testing.T) {
	perguntas := newChannelQuestions(nil)
	_, err := perguntas.AskOnChannel(context.Background(), superficieDeCanal(), pedidoDePermissao())
	if !errors.Is(err, questionnaire.ErrAskerUnavailable) {
		t.Fatalf("erro = %v, quer %v", err, questionnaire.ErrAskerUnavailable)
	}

	var nulo *ChannelQuestions
	if res := nulo.TryAnswer(context.Background(), "conversa-1", "contato-1", "1"); res != AnswerNotPending {
		t.Errorf("resultado = %v, quer mensagem comum", res)
	}
}
