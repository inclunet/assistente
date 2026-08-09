package acp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// managerComCofre devolve o manager com um cofre de mentira e o Config com que
// o transporte teria sido criado. O Config é o que importa: é nele que a
// credencial vira ambiente do processo.
func managerComCofre(cofre map[string]string) (*Manager, *Config) {
	var usado Config
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		ResolveCredential: func(_ context.Context, pattern string) (string, error) {
			return cofre[pattern], nil
		},
		Dial: func(cfg Config, _ RequestHandler) (Client, error) {
			usado = cfg
			return newFakeManagedClient(), nil
		},
	})
	return m, &usado
}

func specComCofre(pares map[string]string) ProviderSpec {
	spec := testSpec()
	spec.CredentialEnv = pares
	return spec
}

// O caminho inteiro do D12: o provider diz de que entrada do cofre vem a
// variável, e o processo do agente nasce com o valor dela.
func TestOAgenteSobeComACredencialDoCofreNaVariavelPedida(t *testing.T) {
	m, cfg := managerComCofre(map[string]string{"api.openai.com": "sk-do-cofre"})
	t.Cleanup(m.Shutdown)

	if _, err := m.Conversation(context.Background(), specComCofre(
		map[string]string{"OPENAI_API_KEY": "api.openai.com"}), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}

	if cfg.Secrets == nil {
		t.Fatal("o transporte subiu sem saber resolver a credencial")
	}
	secrets, err := cfg.Secrets(context.Background())
	if err != nil {
		t.Fatalf("resolver a credencial: %v", err)
	}
	if secrets["OPENAI_API_KEY"] != "sk-do-cofre" {
		t.Fatalf("credenciais = %#v, esperado o valor do cofre na variável pedida", secrets)
	}

	// E é assim que ela chega ao processo: no ambiente daquele exec, e em
	// lugar nenhum além dele.
	ambiente := buildEnv(cfg.Env, secrets)
	if !contemVariavel(ambiente, "OPENAI_API_KEY=sk-do-cofre") {
		t.Error("a variável não entrou no ambiente do processo")
	}
	// E em lugar nenhum além dele: o app nunca chama os.Setenv, então nem o
	// próprio processo do app enxerga a variável.
	if _, existe := os.LookupEnv("OPENAI_API_KEY"); existe {
		t.Fatal("a variável vazou para o ambiente do app")
	}
}

// O padrão é não injetar nada, e ele precisa continuar barato: sem par
// configurado o transporte nem recebe com quem falar sobre cofre.
func TestSemParConfiguradoOAmbienteEODeSempre(t *testing.T) {
	m, cfg := managerComCofre(map[string]string{"api.openai.com": "sk-do-cofre"})
	t.Cleanup(m.Shutdown)

	if _, err := m.Conversation(context.Background(), testSpec(), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}
	if cfg.Secrets != nil {
		t.Error("o agente sem par configurado foi atrás do cofre")
	}
}

// Subir o agente sem a variável faria ele pedir autenticação depois, longe de
// quem poderia consertar. Falhar aqui é o que o D12 manda fazer.
func TestEntradaQueNaoEstaNoCofreFalhaOSpawn(t *testing.T) {
	m, cfg := managerComCofre(map[string]string{"api.anthropic.com": "sk-outra"})
	t.Cleanup(m.Shutdown)

	if _, err := m.Conversation(context.Background(), specComCofre(
		map[string]string{"OPENAI_API_KEY": "api.openai.com"}), "conv-1"); err != nil {
		t.Fatalf("conversa: %v", err)
	}

	_, err := cfg.Secrets(context.Background())
	if err == nil {
		t.Fatal("a resolução passou sem a credencial existir")
	}
	// O erro precisa nomear o que falta e onde: quem lê tem de saber que
	// entrada cadastrar e em que provider.
	for _, esperado := range []string{"api.openai.com", "OPENAI_API_KEY", "cursor"} {
		if !strings.Contains(err.Error(), esperado) {
			t.Errorf("erro %q não menciona %q", err, esperado)
		}
	}
}

// Cofre trancado é o mesmo caso: não dá para subir prometendo uma variável que
// não se conseguiu preencher.
func TestCofreIndisponivelFalhaOSpawnEmVezDeSubirSemAVariavel(t *testing.T) {
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		Dial: func(Config, RequestHandler) (Client, error) {
			return newFakeManagedClient(), nil
		},
	})
	t.Cleanup(m.Shutdown)

	spec := specComCofre(map[string]string{"OPENAI_API_KEY": "api.openai.com"})
	secrets := m.secretsFor(spec)
	if secrets == nil {
		t.Fatal("o par configurado deveria exigir o cofre")
	}
	if _, err := secrets(context.Background()); err == nil {
		t.Fatal("sem cofre, a resolução deveria falhar")
	}
}

// Erro de leitura do cofre não pode virar "sem credencial": a diferença entre
// "a entrada não existe" e "não deu para ler" é o que a pessoa precisa saber.
func TestErroDoCofreChegaInteiroAQuemSobeOAgente(t *testing.T) {
	falha := errors.New("cofre trancado")
	m := NewManager(ManagerConfig{
		Store:   newMemoryStore(),
		WorkDir: func() (string, error) { return dirDeTeste("projeto"), nil },
		ResolveCredential: func(context.Context, string) (string, error) {
			return "", falha
		},
		Dial: func(Config, RequestHandler) (Client, error) {
			return newFakeManagedClient(), nil
		},
	})
	t.Cleanup(m.Shutdown)

	secrets := m.secretsFor(specComCofre(map[string]string{"OPENAI_API_KEY": "api.openai.com"}))
	_, err := secrets(context.Background())
	if !errors.Is(err, falha) {
		t.Fatalf("erro = %v, esperado o do cofre por baixo", err)
	}
}

// Desligar a passagem é imediato no processo seguinte, e é só isso que dá para
// prometer: ambiente de processo não se edita depois do exec.
func TestTrocarACredencialDoCofreSobeOutroProcesso(t *testing.T) {
	comPar := specComCofre(map[string]string{"OPENAI_API_KEY": "api.openai.com"})
	semPar := testSpec()
	outraEntrada := specComCofre(map[string]string{"OPENAI_API_KEY": "api.anthropic.com"})

	if comPar.sameProcess(semPar) {
		t.Error("desligar a passagem deveria pedir processo novo")
	}
	if comPar.sameProcess(outraEntrada) {
		t.Error("trocar a entrada do cofre deveria pedir processo novo")
	}
	if !comPar.sameProcess(specComCofre(map[string]string{"OPENAI_API_KEY": "api.openai.com"})) {
		t.Error("o mesmo par não deveria derrubar o processo de pé")
	}
}

// Quando a mesma variável está nos dois lugares, quem vale é o cofre: ele é o
// caminho nomeado, e o outro é um valor colado à mão que alguém pode ter
// esquecido ali.
func TestACredencialDoCofreVenceOValorColadoNoAmbiente(t *testing.T) {
	ambiente := buildEnv(
		map[string]string{"OPENAI_API_KEY": "colada-a-mao"},
		map[string]string{"OPENAI_API_KEY": "sk-do-cofre"},
	)

	var ultima string
	for _, linha := range ambiente {
		if strings.HasPrefix(linha, "OPENAI_API_KEY=") {
			ultima = linha
		}
	}
	// A última ocorrência é a que o os/exec entrega ao processo.
	if ultima != "OPENAI_API_KEY=sk-do-cofre" {
		t.Errorf("valor entregue = %q, esperado o do cofre", ultima)
	}
}

// O stderr do agente vira log do app, linha a linha. Um agente que despeje o
// próprio ambiente num diagnóstico faria o app gravar a credencial no arquivo
// de log — é a mitigação que o D12 pede deste lado.
func TestOValorDaCredencialNaoChegaAoLog(t *testing.T) {
	registradas := make(chan string, 4)
	writer := newRedactedStderrLoggerTo(
		func(line string) { registradas <- line },
		map[string]string{"OPENAI_API_KEY": "sk-do-cofre"},
	)

	go func() {
		_, _ = fmt.Fprintln(writer, "env: OPENAI_API_KEY=sk-do-cofre PATH=/usr/bin")
		_ = writer.Close()
	}()

	linha := esperaLinha(t, registradas)
	if strings.Contains(linha, "sk-do-cofre") {
		t.Fatalf("a credencial foi para o log: %q", linha)
	}
	// O resto da linha continua legível: redigir não pode custar o
	// diagnóstico, que é a razão de o stderr ser encaminhado.
	if !strings.Contains(linha, "PATH=/usr/bin") {
		t.Errorf("o diagnóstico se perdeu junto com o segredo: %q", linha)
	}
}

// A linha comprida é cortada onde calhar, e o corte pode cair no meio da
// credencial: o pedaço que sobra não bate com o valor inteiro e escaparia da
// substituição.
func TestOCorteDaLinhaCompridaNaoDeixaPedacoDaCredencial(t *testing.T) {
	segredo := "sk-" + strings.Repeat("z", 40)
	registradas := make(chan string, 4)
	writer := newRedactedStderrLoggerTo(
		func(line string) { registradas <- line },
		map[string]string{"OPENAI_API_KEY": segredo},
	)

	// O segredo começa dez bytes antes do corte, então metade dele ficaria do
	// lado que vai para o log.
	enchimento := strings.Repeat("x", stderrLineLimit-10)
	go func() {
		_, _ = fmt.Fprintln(writer, enchimento+segredo+strings.Repeat("y", stderrLineLimit))
		_ = writer.Close()
	}()

	linha := esperaLinha(t, registradas)
	if strings.Contains(linha, "sk-zzz") {
		t.Fatalf("um pedaço da credencial sobreviveu ao corte: ...%q", linha[len(linha)-80:])
	}
}

// Credencial maior que a linha inteira não deixa começo seguro: o corte
// levaria tudo, e o que sobra é dizer que houve saída ali.
func TestCredencialMaiorQueALinhaNaoDeixaNadaEscapar(t *testing.T) {
	segredo := "sk-" + strings.Repeat("z", stderrLineLimit*2)
	registradas := make(chan string, 4)
	writer := newRedactedStderrLoggerTo(
		func(line string) { registradas <- line },
		map[string]string{"OPENAI_API_KEY": segredo},
	)

	go func() {
		_, _ = fmt.Fprintln(writer, "env: "+segredo)
		_ = writer.Close()
	}()

	linha := esperaLinha(t, registradas)
	if strings.Contains(linha, "sk-zzz") {
		t.Fatalf("um pedaço da credencial chegou ao log: %q", linha)
	}
	if !strings.Contains(linha, "truncada") {
		t.Errorf("linha = %q, esperado ao menos o registro de que houve saída", linha)
	}
}

// Um valor comprido no cofre não pode custar caro por linha de log: o stderr do
// agente é encaminhado linha a linha, e um filtro lento aqui atrasaria o
// diagnóstico inteiro.
func TestRedigirValorCompridoNaoCustaCaroPorLinha(t *testing.T) {
	redigir := newRedactor(map[string]string{"K": strings.Repeat("z", stderrLineLimit*2)})

	inicio := time.Now()
	for range 100 {
		redigir("diagnóstico curto do agente")
	}
	if levou := time.Since(inicio); levou > time.Second {
		t.Errorf("cem linhas levaram %v: o filtro está caro demais para o caminho do log", levou)
	}
}

// Sem valor a redigir o filtro é a identidade: o caminho normal — o de quem não
// liga a passagem — não paga nada por ela existir.
func TestSemCredencialORedatorNaoMexeNoTexto(t *testing.T) {
	redigir := newRedactor(nil)
	const linha = "algum diagnóstico do agente"
	if redigir(linha) != linha {
		t.Errorf("texto = %q, esperado intacto", redigir(linha))
	}
}

// Um valor que seja pedaço de outro não pode deixar o resto do maior à mostra.
func TestOValorMaiorEhRedigidoAntesDoMenor(t *testing.T) {
	redigir := newRedactor(map[string]string{
		"CURTA":  "sk-abc",
		"LONGA":  "sk-abc-def-ghi",
		"OUTRA":  "",
		"VAZIA?": "",
	})
	saida := redigir("token=sk-abc-def-ghi fim")
	if strings.Contains(saida, "def-ghi") {
		t.Errorf("sobrou pedaço do valor maior: %q", saida)
	}
}

func esperaLinha(t *testing.T, linhas <-chan string) string {
	t.Helper()
	select {
	case linha := <-linhas:
		return linha
	case <-time.After(5 * time.Second):
		t.Fatal("o leitor de stderr não entregou nada")
		return ""
	}
}

func contemVariavel(ambiente []string, esperada string) bool {
	for _, linha := range ambiente {
		if linha == esperada {
			return true
		}
	}
	return false
}
