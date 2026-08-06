package acpregistry

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// O motivo atravessa para a tela como código, e não como frase (D2): a frase
// precisa existir nos três locales do app, e uma sentença montada em Go só
// existiria em português.

func TestReasonCodeForClassificaCadaDesfecho(t *testing.T) {
	casos := []struct {
		nome string
		err  error
		quer Reason
	}{
		{"sem erro", nil, ""},
		{"major desconhecido", fmt.Errorf("%w: 2.0.0", ErrUnsupportedVersion), ReasonUnsupportedVersion},
		{"índice quebrado", fmt.Errorf("%w: json", ErrMalformedIndex), ReasonMalformedIndex},
		{"interrompida", fmt.Errorf("busca: %w", context.Canceled), ReasonCanceled},
		{"sem resposta no tempo", fmt.Errorf("busca: %w", context.DeadlineExceeded), ReasonTimeout},
		{"registro respondeu com erro", fmt.Errorf("%w: HTTP 503", ErrBadStatus), ReasonBadStatus},
		{"sem rede", errors.New("dial tcp: lookup cdn: no such host"), ReasonUnreachable},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			code, _ := reasonCodeFor(caso.err)
			if code != caso.quer {
				t.Errorf("código = %q, quer %q", code, caso.quer)
			}
		})
	}
}

func TestReasonCodeForSaneiaODetalheDoTransporte(t *testing.T) {
	// O erro de transporte pode carregar o que o outro lado escreveu, e ele vai
	// para a tela e para um leitor de telas junto da frase traduzida.
	_, detalhe := reasonCodeFor(errors.New("proxy disse:\r\nlinha dois\u0007"))
	if detalhe == "" {
		t.Fatal("o desfecho sem rede é o único com detalhe, e ele veio vazio")
	}
	if strings.ContainsAny(detalhe, "\r\n\u0007") {
		t.Errorf("detalhe = %q, quer uma linha só sem controle", detalhe)
	}
}

func TestORegistroQueRespondeComErroNaoViraFalhaDeRede(t *testing.T) {
	// Servidor que respondeu 503 não é servidor inalcançável: dizer "não foi
	// possível falar com o registro" mandaria conferir a própria rede em vez de
	// esperar o outro lado voltar.
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())
	servidor.status(http.StatusServiceUnavailable)

	if _, err := servico.Refresh(context.Background()); !errors.Is(err, ErrBadStatus) {
		t.Fatalf("erro = %v, quer ErrBadStatus", err)
	}
	catalogo := servico.Catalog(context.Background())
	if catalogo.ReasonCode != ReasonBadStatus {
		t.Errorf("código = %q, quer %q", catalogo.ReasonCode, ReasonBadStatus)
	}
	// O detalhe é o status, e nada além dele: a linha de status é texto do
	// servidor, e ela acabaria na tela e no anúncio.
	if catalogo.ReasonDetail != "HTTP 503" {
		t.Errorf("detalhe = %q, quer %q", catalogo.ReasonDetail, "HTTP 503")
	}
}

func TestOsDesfechosSemParteVariavelNaoTemDetalhe(t *testing.T) {
	// Detalhe existe para o que este pacote não sabe redigir de antemão. Nos
	// outros desfechos ele seria a mesma frase duas vezes, uma delas em
	// português dentro de uma tela em outro idioma.
	for _, err := range []error{
		fmt.Errorf("%w: 2.0.0", ErrUnsupportedVersion),
		fmt.Errorf("%w: json", ErrMalformedIndex),
		fmt.Errorf("busca: %w", context.Canceled),
		fmt.Errorf("busca: %w", context.DeadlineExceeded),
	} {
		if _, detalhe := reasonCodeFor(err); detalhe != "" {
			t.Errorf("erro %v veio com detalhe %q", err, detalhe)
		}
	}
}

func TestOCodigoDoMotivoChegaAoCatalogo(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	if _, err := servico.Refresh(context.Background()); err != nil {
		t.Fatalf("primeira carga falhou: %v", err)
	}
	if catalogo := servico.Catalog(context.Background()); catalogo.ReasonCode != "" {
		t.Errorf("código = %q num catálogo carregado, quer vazio", catalogo.ReasonCode)
	}

	servidor.serve(indiceMalformado)
	catalogo, err := servico.Refresh(context.Background())
	if !errors.Is(err, ErrMalformedIndex) {
		t.Fatalf("erro = %v, quer ErrMalformedIndex", err)
	}
	if catalogo.ReasonCode != ReasonMalformedIndex {
		t.Errorf("código = %q, quer %q", catalogo.ReasonCode, ReasonMalformedIndex)
	}
	// E o código sobrevive à próxima leitura: a tela que abrir depois da falha
	// precisa do mesmo motivo que a que estava aberta durante ela.
	if depois := servico.Catalog(context.Background()); depois.ReasonCode != ReasonMalformedIndex {
		t.Errorf("código na leitura seguinte = %q, quer %q", depois.ReasonCode, ReasonMalformedIndex)
	}
}

func TestOCodigoDoMotivoSaiComACargaBemSucedida(t *testing.T) {
	servidor := novoServidor(t, indiceBom)
	servico, _ := novoServico(t, servidor.URL, t.TempDir())

	servidor.status(http.StatusInternalServerError)
	if _, err := servico.Refresh(context.Background()); err == nil {
		t.Fatal("a busca que falhou não devolveu erro")
	}
	if catalogo := servico.Catalog(context.Background()); catalogo.ReasonCode != ReasonBadStatus {
		t.Fatalf("código = %q, quer %q", catalogo.ReasonCode, ReasonBadStatus)
	}

	servidor.serve(indiceBom)
	catalogo, err := servico.Refresh(context.Background())
	if err != nil {
		t.Fatalf("a segunda busca falhou: %v", err)
	}
	if catalogo.ReasonCode != "" || catalogo.ReasonDetail != "" || catalogo.Reason != "" {
		t.Errorf("motivo = (%q, %q, %q), quer tudo vazio depois de carregar", catalogo.ReasonCode, catalogo.ReasonDetail, catalogo.Reason)
	}
}
