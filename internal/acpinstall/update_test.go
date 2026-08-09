package acpinstall

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"assistente/internal/acpregistry"
)

// catalogoMutavel é o registro que republica no meio do teste, que é o que
// acontece de verdade: o cron do registro atualiza o índice de hora em hora, e o
// aviso de versão nova só existe porque o catálogo muda debaixo de uma
// instalação que continua no disco (D10).
type catalogoMutavel struct {
	mu      sync.Mutex
	agentes []acpregistry.Agent
}

func (c *catalogoMutavel) Catalog(context.Context) acpregistry.Catalog {
	c.mu.Lock()
	defer c.mu.Unlock()
	return acpregistry.Catalog{Version: "1.0.0", Agents: c.agentes}
}

func (c *catalogoMutavel) publicar(agentes ...acpregistry.Agent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agentes = agentes
}

// codexNaVersao é a linha do Codex como o registro a publicaria numa versão
// diferente: o pacote fixa a versão no próprio nome, e é ela que vale.
func codexNaVersao(versao string) acpregistry.Agent {
	agente := agenteCodex()
	agente.Version = versao
	agente.Distribution.NPX.Package = codexPacote + "@" + versao
	return agente
}

// opencodeNaVersao é o agente que só publica binário, noutra versão e com o
// digest que o registro promete — ou sem nenhum.
func opencodeNaVersao(t *testing.T, versao, digest string) acpregistry.Agent {
	t.Helper()
	agente := agenteOpencode(t, digest)
	agente.Version = versao
	return agente
}

const codexVersaoNova = "1.2.0"

// cenarioComVersaoNova instala o Codex na versão do catálogo e depois publica a
// seguinte. É o estado em que o D10 começa: uma instalação que funciona e um
// registro que passou a dizer outra coisa.
func cenarioComVersaoNova(t *testing.T) (*cenario, *catalogoMutavel) {
	t.Helper()
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{agenteCodex()}}
	c := montar(t, opcoes{source: registro})
	if _, err := c.instalador.Install(context.Background(), codexID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(codexNaVersao(codexVersaoNova))
	return c, registro
}

func TestOPlanoAvisaQueOCatalogoPassouAPublicarOutraVersao(t *testing.T) {
	// O app compara a versão instalada com a do catálogo e avisa, no item do
	// agente (D10). Instalar deixa de ser oferecido: o agente está aqui, e o que
	// existe a fazer com ele é atualizar.
	c, _ := cenarioComVersaoNova(t)

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Installed == nil || plano.Installed.Version != codexVersao {
		t.Fatalf("instalada = %+v, queria a versão que está no disco", plano.Installed)
	}
	if plano.Version != codexVersaoNova {
		t.Errorf("versão do plano = %q, queria a que o catálogo publica agora", plano.Version)
	}
	if !plano.Update || !plano.CanUpdate {
		t.Errorf("plano = %+v, queria a oferta de atualizar", plano)
	}
	if plano.UpdateReason != "" {
		t.Errorf("motivo = %q, queria nenhum: dá para atualizar", plano.UpdateReason)
	}
	if plano.CanInstall {
		t.Error("ofereceu instalar por cima de uma instalação que já existe")
	}
}

func TestSemVersaoNovaNaoHaAtualizacaoAOferecer(t *testing.T) {
	// Nada acontece sozinho, e nada é oferecido sem ter o que oferecer: o item
	// de quem está na versão do catálogo não pode sugerir uma atualização que
	// reinstalaria o mesmo pacote.
	c := montar(t, opcoes{})
	if _, err := c.instalador.Install(context.Background(), codexID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Update || plano.CanUpdate {
		t.Errorf("plano = %+v, queria nenhuma oferta de atualizar", plano)
	}
	if _, err := c.instalador.Update(context.Background(), codexID, Confirmed{}); !errors.Is(err, ErrNoUpdate) {
		t.Errorf("erro = %v, queria a recusa por já estar na versão publicada", err)
	}
}

func TestAtualizarPoeANovaAoLadoEDeixaAAnteriorNoDisco(t *testing.T) {
	// A anterior só sai depois de o provider apontar para a nova (D10), e quem
	// reponta é a camada de cima: apagar aqui deixaria o provider apontando para
	// um diretório que acabou de sumir.
	c, _ := cenarioComVersaoNova(t)

	atualizada, err := c.instalador.Update(context.Background(), codexID, Confirmed{})
	if err != nil {
		t.Fatalf("não atualizou: %v", err)
	}
	if atualizada.Installed.Version != codexVersaoNova {
		t.Errorf("versão instalada = %q, queria %q", atualizada.Installed.Version, codexVersaoNova)
	}
	if atualizada.Previous.Version != codexVersao {
		t.Errorf("versão anterior = %q, queria %q", atualizada.Previous.Version, codexVersao)
	}
	// As duas no disco, cada uma no diretório da sua versão (D5).
	for _, versao := range []string{codexVersao, codexVersaoNova} {
		if _, err := os.Stat(filepath.Join(c.root, codexID, versao, installedFileName)); err != nil {
			t.Errorf("a versão %s não está no disco: %v", versao, err)
		}
	}
	// E a nova só é declarada pronta depois do handshake (D8): são dois, o da
	// instalação inicial e o desta.
	if len(c.handshakes) != 2 {
		t.Fatalf("handshakes = %d, queria um por instalação", len(c.handshakes))
	}
	conferido := c.handshakes[1]
	if conferido[0] != atualizada.Installed.Command {
		t.Errorf("conferiu %q, queria o comando da versão nova", conferido[0])
	}
	if !strings.Contains(conferido[1], codexVersaoNova) {
		t.Errorf("conferiu %q, queria o ponto de entrada da versão nova", conferido[1])
	}
}

func TestAAtualizacaoQueFalhaDeixaDePeOQueFuncionava(t *testing.T) {
	// É a razão de instalar ao lado: a versão nova pode não subir, e quem estava
	// com um agente funcionando não pode terminar sem nenhum.
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{agenteCodex()}}
	c := montar(t, opcoes{source: registro})
	if _, err := c.instalador.Install(context.Background(), codexID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(codexNaVersao(codexVersaoNova))
	c.npm.erro = errors.New("o registro npm não respondeu")

	if _, err := c.instalador.Update(context.Background(), codexID, Confirmed{}); err == nil {
		t.Fatal("a atualização com o npm quebrado terminou bem")
	}
	instalada, ok := c.instalador.Installed(codexID)
	if !ok || instalada.Version != codexVersao {
		t.Fatalf("instalada = %+v (%v), queria a versão anterior intacta", instalada, ok)
	}
	// E a tentativa não deixa resíduo: um diretório pela metade seria lido como
	// instalação na abertura seguinte (D13).
	novo := filepath.Join(c.root, codexID, codexVersaoNova)
	if _, err := os.Stat(novo); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sobrou %s depois da falha (err = %v)", novo, err)
	}
}

func TestAtualizarOQueOAppNaoInstalouERecusado(t *testing.T) {
	// Atualizar é trocar o que o app pôs ali. Sem registro de instalação não há
	// o que trocar, e instalar do zero é outro caminho, com outra confirmação.
	c := montar(t, opcoes{})

	_, err := c.instalador.Update(context.Background(), codexID, Confirmed{})
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("erro = %v, queria a recusa por não haver instalação", err)
	}
}

func TestInstalarPorCimaDeOutraVersaoERecusado(t *testing.T) {
	// Pôr a nova ao lado sem tirar a anterior é atualizar, e atualizar tem um
	// passo no meio que o caminho de instalar não dá.
	c, _ := cenarioComVersaoNova(t)

	_, err := c.instalador.Install(context.Background(), codexID, Confirmed{})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("erro = %v, queria a recusa por já haver instalação", err)
	}
	if _, err := os.Stat(filepath.Join(c.root, codexID, codexVersaoNova)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a recusa deixou o diretório da versão nova no disco")
	}
}

func TestRemoveVersionApagaSoAVersaoPedida(t *testing.T) {
	// É o último passo da atualização, e ele não pode levar junto a versão que
	// acabou de subir.
	c, _ := cenarioComVersaoNova(t)
	if _, err := c.instalador.Update(context.Background(), codexID, Confirmed{}); err != nil {
		t.Fatalf("não atualizou: %v", err)
	}

	if err := c.instalador.RemoveVersion(context.Background(), codexID, codexVersao); err != nil {
		t.Fatalf("não removeu a versão anterior: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.root, codexID, codexVersao)); !errors.Is(err, os.ErrNotExist) {
		t.Error("a versão anterior continua no disco")
	}
	instalada, ok := c.instalador.Installed(codexID)
	if !ok || instalada.Version != codexVersaoNova {
		t.Fatalf("instalada = %+v (%v), queria a versão nova de pé", instalada, ok)
	}
}

func TestRemoveVersionNaoApagaDiretorioQueNaoDescreveUmaInstalacao(t *testing.T) {
	// O caminho é montado a partir de dois textos externos, e apagar o que a
	// régua deixar passar sem conferir o que está lá dentro é apagar por
	// adivinhação (D9).
	c := montar(t, opcoes{})
	intruso := filepath.Join(c.root, codexID, "9.9.9")
	if err := os.MkdirAll(intruso, 0o755); err != nil {
		t.Fatalf("erro ao montar o diretório intruso: %v", err)
	}

	err := c.instalador.RemoveVersion(context.Background(), codexID, "9.9.9")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("erro = %v, queria a recusa por não haver instalação ali", err)
	}
	if _, err := os.Stat(intruso); err != nil {
		t.Errorf("apagou um diretório que não descreve instalação nenhuma: %v", err)
	}
}

func TestAInstalacaoConferidaNaoETrocadaPorUmaSemDigest(t *testing.T) {
	// Se o agente parar de publicar digest entre uma versão e a seguinte, o app
	// mantém o que está instalado e explica: aceitar a troca faria do aviso de
	// atualização um caminho para contornar o D4.
	pacote := pacoteDoOpencode(t)
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{opencodeNaVersao(t, opencodeVersao, digestDe(pacote))}}
	c := montar(t, opcoes{source: registro, runtime: runtimeSemNode, http: &clienteFalso{corpo: pacote}})
	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(opencodeNaVersao(t, "0.5.0", ""))

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.Update {
		t.Fatal("o plano não avisou da versão nova")
	}
	if plano.CanUpdate {
		t.Error("ofereceu trocar uma instalação conferida por uma que o app não tem como conferir")
	}
	if !strings.Contains(plano.UpdateReason, "verificação de integridade") {
		t.Errorf("motivo = %q, queria a explicação da recusa", plano.UpdateReason)
	}

	// E a recusa não mora só no plano: quem chama o pacote direto ouve a mesma
	// coisa, com o erro que dá para distinguir.
	_, err = c.instalador.Update(context.Background(), opencodeID, Confirmed{AcceptUnverified: true})
	if !errors.Is(err, ErrVerificationWouldDrop) {
		t.Fatalf("erro = %v, queria a recusa da troca de artefato conferido", err)
	}
	instalada, _ := c.instalador.Installed(opencodeID)
	if instalada.Version != opencodeVersao {
		t.Errorf("instalada = %q, queria a versão conferida intacta", instalada.Version)
	}
}

func TestAVersaoNovaSemAlvoParaEstaPlataformaAindaEAvisada(t *testing.T) {
	// Ter o agente instalado e ver o registro publicar uma versão que não chega
	// a esta máquina são duas coisas, e a tela precisa das duas: dizer só "não
	// há alvo" esconde que existe versão nova, e dizer só "há versão nova"
	// prometeria uma atualização que não acontece.
	pacote := pacoteDoOpencode(t)
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{opencodeNaVersao(t, opencodeVersao, digestDe(pacote))}}
	c := montar(t, opcoes{source: registro, runtime: runtimeSemNode, http: &clienteFalso{corpo: pacote}})
	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	semAlvo := opencodeNaVersao(t, "0.5.0", digestDe(pacote))
	semAlvo.Distribution.Binary = map[string]acpregistry.BinaryTarget{
		"plan9-riscv": {Archive: opencodeURL, SHA256: digestDe(pacote), Cmd: "./opencode"},
	}
	registro.publicar(semAlvo)

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.Update {
		t.Fatal("o plano não avisou da versão nova")
	}
	if plano.CanUpdate {
		t.Error("ofereceu atualizar para uma versão que não tem alvo para esta plataforma")
	}
	if !strings.Contains(plano.UpdateReason, PlatformTarget()) {
		t.Errorf("motivo = %q, queria que ele nomeasse a plataforma procurada", plano.UpdateReason)
	}
}

func TestAInstalacaoQueJaNaoEraConferidaPodeSerAtualizada(t *testing.T) {
	// A regra protege o que foi conferido. Quem instalou um artefato sem digest
	// — com a confirmação reforçada do D4 — não perde nada ao trocá-lo por
	// outro do mesmo tipo, e travar a atualização ali deixaria justamente esses
	// agentes presos na primeira versão que instalaram.
	pacote := pacoteDoOpencode(t)
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{opencodeNaVersao(t, opencodeVersao, "")}}
	c := montar(t, opcoes{source: registro, runtime: runtimeSemNode, http: &clienteFalso{corpo: pacote}})
	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{AcceptUnverified: true}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(opencodeNaVersao(t, "0.5.0", ""))

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.CanUpdate {
		t.Fatalf("não ofereceu atualizar: %q", plano.UpdateReason)
	}

	atualizada, err := c.instalador.Update(context.Background(), opencodeID, Confirmed{AcceptUnverified: true})
	if err != nil {
		t.Fatalf("não atualizou: %v", err)
	}
	if atualizada.Installed.Version != "0.5.0" {
		t.Errorf("versão instalada = %q, queria a nova", atualizada.Installed.Version)
	}
	// E a marca continua dizendo o que aquele arquivo vale: atualizar não
	// promove um artefato observado a conferido.
	if Verified(atualizada.Installed) {
		t.Error("a versão nova sem digest foi gravada como conferida")
	}
}

func TestAAtualizacaoSemDigestTambemExigeAConfirmacaoReforcada(t *testing.T) {
	// A pergunta do D4 é por artefato, e não por agente: quem aceitou um arquivo
	// sem digest há dois meses não consentiu com o de hoje.
	pacote := pacoteDoOpencode(t)
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{opencodeNaVersao(t, opencodeVersao, "")}}
	c := montar(t, opcoes{source: registro, runtime: runtimeSemNode, http: &clienteFalso{corpo: pacote}})
	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{AcceptUnverified: true}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(opencodeNaVersao(t, "0.5.0", ""))

	_, err := c.instalador.Update(context.Background(), opencodeID, Confirmed{})
	if !errors.Is(err, ErrUnverifiedNotAccepted) {
		t.Fatalf("erro = %v, queria a recusa por falta da confirmação reforçada", err)
	}
}

func TestSemNodeOMotivoDeNaoAtualizarEOMesmoDeNaoInstalar(t *testing.T) {
	// O que impede pôr a versão nova no disco impede atualizar, e o motivo é o
	// mesmo texto: dizer só "não dá para atualizar" mandaria procurar um
	// problema no agente quando o que falta é o runtime (D7).
	registro := &catalogoMutavel{agentes: []acpregistry.Agent{agenteCodex()}}
	c := montar(t, opcoes{source: registro})
	if _, err := c.instalador.Install(context.Background(), codexID, Confirmed{}); err != nil {
		t.Fatalf("a instalação inicial falhou: %v", err)
	}
	registro.publicar(codexNaVersao(codexVersaoNova))

	// O mesmo disco, sem Node: é o que acontece com quem desinstalou o runtime
	// depois de instalar o agente.
	semNode := montar(t, opcoes{source: registro, runtime: runtimeSemNode, root: c.root})
	plano, err := semNode.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.Update {
		t.Fatal("o plano não avisou da versão nova")
	}
	if plano.CanUpdate {
		t.Error("ofereceu atualizar sem o runtime que o caminho usa")
	}
	if plano.UpdateReason != ErrRuntimeMissing.Error() {
		t.Errorf("motivo = %q, queria %q", plano.UpdateReason, ErrRuntimeMissing.Error())
	}
}
