package acpinstall

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
)

const (
	opencodeID     = "opencode"
	opencodeVersao = "0.4.2"
	opencodeURL    = "https://exemplo.test/opencode.zip"
)

// clienteSemRede é o download que nenhum teste pediu. Ele existe para o
// instalador padrão dos cenários não ter como sair para a internet.
type clienteSemRede struct{}

func (clienteSemRede) Do(context.Context, *http.Request) (*http.Response, error) {
	return nil, errors.New("este cenário não esperava baixar nada")
}

// nomeExecutavel é o nome do arquivo que sobe como processo nesta plataforma.
// No Windows ele precisa da extensão: um arquivo sem ela não é spawnável, e é
// justamente essa a diferença que a resolução do comando existe para tratar.
func nomeExecutavel(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// agenteOpencode é a linha do catálogo de um agente que só publica binário, com
// o alvo desta plataforma e o digest que o registro promete.
func agenteOpencode(t *testing.T, digest string) acpregistry.Agent {
	t.Helper()
	plataforma := PlatformTarget()
	if plataforma == "" {
		t.Skipf("o registro não nomeia alvo para %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	return acpregistry.Agent{
		ID:      opencodeID,
		Name:    "opencode",
		Version: opencodeVersao,
		Distribution: acpregistry.Distribution{
			Binary: map[string]acpregistry.BinaryTarget{
				plataforma: {
					Archive: opencodeURL,
					SHA256:  digest,
					Cmd:     "./" + nomeExecutavel(opencodeID),
					Args:    []string{"--acp"},
				},
			},
		},
	}
}

// pacoteDoOpencode é o zip que o registro serviria, com o executável dentro.
func pacoteDoOpencode(t *testing.T) []byte {
	t.Helper()
	return zipDeTesteBytes(t, []entradaZip{
		{nome: nomeExecutavel(opencodeID), conteudo: "binário de mentira", modo: 0o755},
	})
}

func TestOPlanoBinarioMostraOAlvoODigestENaoExigeRuntime(t *testing.T) {
	// Artefato binário sobe sem Node. Marcar o runtime como pré-requisito faria
	// a tela bloquear o download por falta de algo que aquele caminho não usa —
	// e são sete os agentes com digest que não têm alternativa npm.
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if !plano.CanInstall {
		t.Fatalf("não ofereceu instalação de artefato binário sem Node: %+v", plano)
	}
	if plano.Runtime.Required {
		t.Error("disse que a instalação binária depende do Node")
	}
	if plano.Distribution != DistributionBinary {
		t.Errorf("distribuição = %q, queria %q", plano.Distribution, DistributionBinary)
	}
	if plano.Origin != opencodeURL {
		t.Errorf("origem = %q, queria a URL do artefato", plano.Origin)
	}
	if plano.Target != PlatformTarget() {
		t.Errorf("alvo = %q, queria %q", plano.Target, PlatformTarget())
	}
	if plano.SHA256 != digestDe(pacote) {
		t.Errorf("digest = %q, queria o publicado pelo registro", plano.SHA256)
	}
	if plano.Dir != filepath.Join(c.root, opencodeID, opencodeVersao) {
		t.Errorf("pasta = %q, queria <root>/<id>/<versão>", plano.Dir)
	}
	if plano.InstallCommand != "" {
		t.Errorf("comando de instalação = %q, queria vazio: não se executa nada para instalar um artefato", plano.InstallCommand)
	}
}

func TestSemAlvoParaEstaPlataformaOPlanoDizIsso(t *testing.T) {
	// A cobertura do registro não é uniforme, e `windows-aarch64` é o alvo mais
	// raro. Não achar alvo é estado a explicar, e não defeito.
	agente := acpregistry.Agent{
		ID:      opencodeID,
		Name:    "opencode",
		Version: opencodeVersao,
		Distribution: acpregistry.Distribution{
			Binary: map[string]acpregistry.BinaryTarget{
				"plan9-riscv": {Archive: opencodeURL, SHA256: strings.Repeat("a", 64), Cmd: "./opencode"},
			},
		},
	}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if plano.CanInstall {
		t.Error("ofereceu instalação de um alvo que não existe para esta plataforma")
	}
	if !strings.Contains(plano.Reason, PlatformTarget()) {
		t.Errorf("motivo = %q, queria que ele nomeasse a plataforma procurada", plano.Reason)
	}
	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{Distribution: DistributionBinary}); !errors.Is(err, ErrNoPlatformTarget) {
		t.Errorf("erro = %v, queria a recusa por falta de alvo", err)
	}
}

func TestSemDigestPublicadoOPlanoOfereceInstalarEAvisaOQueNaoDaParaConferir(t *testing.T) {
	// Metade dos agentes com binário não publica `sha256`, e o Cursor é um
	// deles. Recusar não deixaria ninguém sem o agente: deixaria a pessoa
	// baixando o mesmo arquivo do mesmo host pelo site, sem guarda nenhuma. O
	// que a falta de digest muda é o quanto o app afirma, e não se ele instala
	// (D4) — daí o plano oferecer, marcado.
	agente := agenteOpencode(t, "")
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if !plano.CanInstall {
		t.Errorf("não ofereceu instalação: %q", plano.Reason)
	}
	if !plano.Unverified {
		t.Error("o plano não marcou que o app não tem como conferir este artefato")
	}
	if plano.SHA256 != "" {
		t.Errorf("digest = %q, queria vazio: o registro não publica um", plano.SHA256)
	}
}

func TestSemDigestPublicadoInstalarSemAConfirmacaoReforcadaERecusado(t *testing.T) {
	// A pergunta do D4 não pode morar só na tela: uma regra de interface deixa
	// de valer no primeiro chamador novo, e o consentimento é justamente o que
	// separa instalar de baixar sozinho. Não há campo, preferência nem valor
	// padrão que a dispense.
	cliente := &clienteFalso{corpo: pacoteDoOpencode(t)}
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    cliente,
	})

	_, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{Distribution: DistributionBinary})
	if !errors.Is(err, ErrUnverifiedNotAccepted) {
		t.Fatalf("erro = %v, queria a recusa por falta da confirmação reforçada", err)
	}
	if cliente.chamadas != 0 {
		t.Errorf("chamadas = %d, queria nenhuma: a recusa acontece antes de baixar", cliente.chamadas)
	}
}

func TestComAConfirmacaoReforcadaOArtefatoSemDigestEInstaladoEODigestFicaComoObservado(t *testing.T) {
	// O digest observado não protege esta instalação — nada protege, é essa a
	// natureza do problema. Ele fica gravado, e marcado pelo que vale, para a
	// tela continuar dizendo que aquela instalação não foi verificada e para a
	// mudança do artefato ser percebida depois (D4).
	pacote := pacoteDoOpencode(t)
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	instalacao, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution:     DistributionBinary,
		AcceptUnverified: true,
	})
	if err != nil {
		t.Fatalf("não instalou o agente sem digest com a confirmação dada: %v", err)
	}
	if instalacao.SHA256 != digestDe(pacote) {
		t.Errorf("digest = %q, queria o do arquivo que chegou", instalacao.SHA256)
	}
	if instalacao.SHA256Origin != DigestObserved {
		t.Errorf("origem do digest = %q, queria %q", instalacao.SHA256Origin, DigestObserved)
	}

	// E o registro relido diz o mesmo: é ele que a tela lê para continuar
	// marcando a instalação como não verificada depois de pronta.
	gravado, ok := c.instalador.Installed(opencodeID)
	if !ok {
		t.Fatal("a instalação não foi encontrada depois de gravada")
	}
	if gravado.SHA256Origin != DigestObserved {
		t.Errorf("origem gravada = %q, queria %q", gravado.SHA256Origin, DigestObserved)
	}
}

// instalarSemDigest baixa o que o cliente estiver servindo, com a confirmação
// reforçada dada.
func instalarSemDigest(t *testing.T, c *cenario) (Installation, error) {
	t.Helper()
	return c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution:     DistributionBinary,
		AcceptUnverified: true,
	})
}

func TestAMesmaVersaoQueVoltaComOutroArquivoERecusada(t *testing.T) {
	// É a confiança que o SSH aplica à chave de host: aceita o que aparece na
	// estreia e passa a estranhar a troca. Não protege a primeira instalação —
	// nada protege —, e passa a proteger todas as seguintes (D4).
	cliente := &clienteFalso{corpo: pacoteDoOpencode(t)}
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    cliente,
	})

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a primeira instalação falhou: %v", err)
	}
	if err := c.instalador.Remove(context.Background(), opencodeID); err != nil {
		t.Fatalf("erro ao remover o agente: %v", err)
	}

	// O mesmo agente, a mesma versão, outro arquivo.
	cliente.corpo = zipDeTesteBytes(t, []entradaZip{
		{nome: nomeExecutavel(opencodeID), conteudo: "outro binário", modo: 0o755},
	})
	_, err := instalarSemDigest(t, c)
	if !errors.Is(err, ErrArtifactChanged) {
		t.Fatalf("erro = %v, queria a recusa do artefato que mudou", err)
	}
	// E a recusa não deixa resíduo: o diretório da versão seria lido como
	// instalação na abertura seguinte.
	dir := filepath.Join(c.root, opencodeID, opencodeVersao)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sobrou %s depois da recusa (err = %v)", dir, err)
	}
}

func TestAMesmaVersaoComOMesmoArquivoContinuaInstalavel(t *testing.T) {
	// A memória serve para estranhar a troca, e não para impedir reinstalar o
	// que já se tinha: quem removeu por espaço em disco volta a instalar.
	pacote := pacoteDoOpencode(t)
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a primeira instalação falhou: %v", err)
	}
	if err := c.instalador.Remove(context.Background(), opencodeID); err != nil {
		t.Fatalf("erro ao remover o agente: %v", err)
	}
	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a reinstalação do mesmo arquivo falhou: %v", err)
	}
}

func TestAMemoriaDoArtefatoSobreviveARemocaoDoAgente(t *testing.T) {
	// Ela mora na raiz, e não dentro do diretório do agente: remover libera
	// disco, e esquecer junto faria a instalação seguinte ser de novo uma
	// estreia — que é justamente quando não há o que conferir.
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacoteDoOpencode(t)},
	})

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a instalação falhou: %v", err)
	}
	if err := c.instalador.Remove(context.Background(), opencodeID); err != nil {
		t.Fatalf("erro ao remover o agente: %v", err)
	}

	if c.instalador.knownDigest(opencodeID, opencodeVersao) == "" {
		t.Error("a memória do artefato foi embora junto com o agente")
	}
}

func TestOArtefatoComDigestPublicadoNaoEntraNaMemoria(t *testing.T) {
	// O publicado já ancora a versão por conta própria, e lembrar dele faria o
	// app recusar a republicação que o registro anuncia — que é exatamente o
	// caso em que ele deveria confiar.
	pacote := pacoteDoOpencode(t)
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, digestDe(pacote))},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{
		Distribution: DistributionBinary,
	}); err != nil {
		t.Fatalf("a instalação falhou: %v", err)
	}

	if got := c.instalador.knownDigest(opencodeID, opencodeVersao); got != "" {
		t.Errorf("memória = %q, queria vazia para artefato conferido", got)
	}
}

func TestAMemoriaIlegivelNaoTravaAInstalacao(t *testing.T) {
	// Quem consegue corromper o arquivo consegue trocar o executável instalado
	// ao lado dele: travar toda instalação por causa disso custaria caro sem
	// fechar porta nenhuma. O aviso fica no log.
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacoteDoOpencode(t)},
	})
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		t.Fatalf("erro ao preparar a raiz: %v", err)
	}
	if err := os.WriteFile(filepath.Join(c.root, knownFileName), []byte("{isso não é json"), 0o644); err != nil {
		t.Fatalf("erro ao escrever a memória corrompida: %v", err)
	}

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a instalação falhou por causa da memória ilegível: %v", err)
	}
	// E a memória volta a valer: o arquivo corrompido é substituído pelo que
	// esta instalação observou.
	if c.instalador.knownDigest(opencodeID, opencodeVersao) == "" {
		t.Error("a memória não foi refeita depois de encontrada corrompida")
	}
}

func TestAMemoriaDeOutraVersaoDoAppNaoEReescrita(t *testing.T) {
	// Voltar para uma versão anterior do app é comum, e nela a memória gravada
	// pela mais nova não é lixo: é o que vai valer de novo quando o app voltar a
	// ser aquele. Reescrevê-la para caber nesta versão apagaria os dois lados.
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacoteDoOpencode(t)},
	})
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		t.Fatalf("erro ao preparar a raiz: %v", err)
	}
	caminho := filepath.Join(c.root, knownFileName)
	doFuturo := []byte(`{"schema":99,"agents":{"outro":{"1.0.0":"abc"}}}`)
	if err := os.WriteFile(caminho, doFuturo, 0o644); err != nil {
		t.Fatalf("erro ao escrever a memória de outra versão: %v", err)
	}

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a instalação falhou por causa da memória de outra versão: %v", err)
	}

	depois, err := os.ReadFile(caminho)
	if err != nil {
		t.Fatalf("erro ao reler a memória: %v", err)
	}
	if string(depois) != string(doFuturo) {
		t.Errorf("a memória de outra versão do app foi reescrita: %s", depois)
	}
}

func TestAMemoriaSemEsquemaERefeita(t *testing.T) {
	// JSON válido sem o campo do esquema não veio de uma versão que sabe mais:
	// é o que sobra de um arquivo truncado ou editado à mão, e congelá-lo
	// deixaria a proteção desligada até alguém apagar o arquivo.
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacoteDoOpencode(t)},
	})
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		t.Fatalf("erro ao preparar a raiz: %v", err)
	}
	if err := os.WriteFile(filepath.Join(c.root, knownFileName), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("erro ao escrever a memória sem esquema: %v", err)
	}

	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a instalação falhou por causa da memória sem esquema: %v", err)
	}
	if c.instalador.knownDigest(opencodeID, opencodeVersao) == "" {
		t.Error("a memória não foi refeita depois de encontrada sem esquema")
	}
}

func TestAMemoriaGrandeDemaisERecusadaPeloNomeDela(t *testing.T) {
	// A recusa por tamanho vai para o log de quem precisa achar o arquivo, e o
	// teto vale para mais de um arquivo do pacote: dizer só o teto deixaria a
	// mensagem apontando para o registro de instalação, que não é este.
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, "")},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacoteDoOpencode(t)},
	})
	if err := os.MkdirAll(c.root, 0o755); err != nil {
		t.Fatalf("erro ao preparar a raiz: %v", err)
	}
	caminho := filepath.Join(c.root, knownFileName)
	if err := os.WriteFile(caminho, make([]byte, maxKnownBytes+1), 0o644); err != nil {
		t.Fatalf("erro ao escrever a memória grande demais: %v", err)
	}

	_, err := c.instalador.readKnown()
	if err == nil {
		t.Fatal("a memória acima do teto foi lida como se coubesse")
	}
	if !strings.Contains(err.Error(), "memória de artefatos") {
		t.Errorf("a recusa não nomeia o arquivo recusado: %v", err)
	}
	// E ela não trava a instalação, pelo mesmo motivo da memória ilegível.
	if _, err := instalarSemDigest(t, c); err != nil {
		t.Fatalf("a instalação falhou por causa da memória grande demais: %v", err)
	}
}

func TestInstalarOArtefatoBaixaConfereAbreEGravaORegistro(t *testing.T) {
	pacote := pacoteDoOpencode(t)
	digest := digestDe(pacote)
	cliente := &clienteFalso{corpo: pacote}
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, digest)},
		runtime: runtimeSemNode,
		http:    cliente,
	})

	instalacao, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{Distribution: DistributionBinary})
	if err != nil {
		t.Fatalf("não instalou o agente distribuído como binário: %v", err)
	}

	dir := filepath.Join(c.root, opencodeID, opencodeVersao)
	esperado := filepath.Join(dir, nomeExecutavel(opencodeID))
	if instalacao.Command != esperado {
		t.Errorf("comando = %q, queria o executável extraído em %q", instalacao.Command, esperado)
	}
	if len(instalacao.Args) != 1 || instalacao.Args[0] != "--acp" {
		t.Errorf("argumentos = %v, queria os que o registro publica", instalacao.Args)
	}
	if instalacao.Distribution != DistributionBinary || instalacao.Target != PlatformTarget() {
		t.Errorf("instalação = %+v, queria distribuição binária com o alvo desta plataforma", instalacao)
	}
	// O digest gravado é o do que chegou, e a origem diz que ele foi conferido
	// contra o publicado — é o que separa esta instalação das que a Fase 5 vai
	// permitir sem prova de procedência (D4).
	if instalacao.SHA256 != digest || instalacao.SHA256Origin != DigestVerified {
		t.Errorf("digest = %q/%q, queria %q conferido", instalacao.SHA256, instalacao.SHA256Origin, digest)
	}
	if _, err := os.Stat(esperado); err != nil {
		t.Errorf("o executável não ficou no disco: %v", err)
	}
	// O pacote baixado não é parte da instalação: deixá-lo ali dobraria o espaço
	// ocupado por cada agente.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("erro ao ler a instalação: %v", err)
	}
	for _, entrada := range entradas {
		if strings.HasSuffix(entrada.Name(), ".download") {
			t.Errorf("o pacote baixado ficou em %s", entrada.Name())
		}
	}

	if len(c.handshakes) != 1 || c.handshakes[0][0] != esperado {
		t.Errorf("handshakes = %v, queria uma conferência do comando resolvido", c.handshakes)
	}
	if got := c.marcos.etapas(); !slices.Equal(got, []Stage{StageStarted, StageInstalling, StageVerifying, StageDone}) {
		t.Errorf("marcos = %v, queria a sequência completa", got)
	}

	// E o registro relido descreve a mesma instalação: é ele que a tela lê para
	// oferecer "usar o comando instalado" na abertura seguinte (D5).
	relido, ok := c.instalador.Installed(opencodeID)
	if !ok {
		t.Fatal("o registro gravado não foi reconhecido na releitura")
	}
	if relido.Command != esperado || relido.SHA256 != digest {
		t.Errorf("registro relido = %+v, queria o mesmo comando e digest", relido)
	}
}

func TestOArtefatoQueNaoBateComODigestNaoViraInstalacao(t *testing.T) {
	// Digest divergente é o artefato que não é o que o registro prometeu. Ele
	// não fica no disco esperando alguém executá-lo por engano (D4), e o
	// diretório da instalação some junto (D13).
	pacote := pacoteDoOpencode(t)
	c := montar(t, opcoes{
		agentes: []acpregistry.Agent{agenteOpencode(t, strings.Repeat("b", 64))},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})

	if _, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{Distribution: DistributionBinary}); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("erro = %v, queria a recusa pelo digest", err)
	}
	dir := filepath.Join(c.root, opencodeID, opencodeVersao)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a instalação recusada deixou %s no disco (%v)", dir, err)
	}
	ultimo := c.marcos.ultimo()
	if ultimo.Stage != StageFailed || ultimo.Step != StepDownload {
		t.Errorf("marco = %+v, queria a falha nomeando a etapa do download", ultimo)
	}
	if len(c.handshakes) != 0 {
		t.Errorf("conferiu o protocolo de um artefato recusado: %v", c.handshakes)
	}
}

func TestOAgenteComOsDoisCaminhosPrefereONpmEcaiParaOBinarioSemNode(t *testing.T) {
	// npm baixa menos, atualiza incrementalmente e tem integridade conferida
	// pelo próprio npm. Sem Node, porém, aquele caminho não existe — e recusar
	// a instalação ali mandaria instalar o Node para não usá-lo.
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	agente.Distribution.NPX = &acpregistry.PackageDistribution{Package: "opencode-acp@" + opencodeVersao}

	comNode := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})
	plano, err := comNode.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Distribution != DistributionNPM {
		t.Errorf("distribuição = %q, queria o npm quando há Node", plano.Distribution)
	}

	semNode := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode})
	plano, err = semNode.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Distribution != DistributionBinary || !plano.CanInstall {
		t.Errorf("plano = %+v, queria o artefato binário oferecido numa máquina sem Node", plano)
	}

	// Node encontrado não garante npm: o `npm-cli.js` pode não estar ao lado
	// dele, e aí o caminho npm não existe do mesmo jeito.
	semNpm := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: func() acp.NodeRuntime {
		nodeSemNpm := runtimeComNode()
		nodeSemNpm.NPMScript, nodeSemNpm.NPM = "", ""
		return nodeSemNpm
	}})
	plano, err = semNpm.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Distribution != DistributionBinary || !plano.CanInstall {
		t.Errorf("plano = %+v, queria o artefato binário oferecido numa máquina com Node mas sem npm", plano)
	}
}

func TestInstalarPorUmPlanoQueDeixouDeValerERecusado(t *testing.T) {
	// O caminho escolhido depende da máquina, e ela muda: o Node pode aparecer
	// entre a confirmação e o clique, quando alguém termina de instalá-lo em
	// outra janela. Quem confirmou baixar um arquivo com determinado digest não
	// consentiu com um `npm install` (D3).
	pacote := pacoteDoOpencode(t)
	agente := agenteOpencode(t, digestDe(pacote))
	agente.Distribution.NPX = &acpregistry.PackageDistribution{Package: "opencode-acp@" + opencodeVersao}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, http: &clienteFalso{corpo: pacote}})

	_, err := c.instalador.Install(context.Background(), opencodeID, Confirmed{Distribution: DistributionBinary})
	if !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("erro = %v, queria a recusa do plano vencido", err)
	}
	if len(c.handshakes) != 0 {
		t.Errorf("instalou alguma coisa: %v", c.handshakes)
	}

	// E a distribuição igual não basta: o registro republica versão e digest
	// sozinho, e a mesma linha do catálogo pode passar a apontar outro arquivo.
	semNode := montar(t, opcoes{
		agentes: []acpregistry.Agent{agente},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})
	confirmados := map[string]Confirmed{
		"outra origem": {Distribution: DistributionBinary, Origin: "https://exemplo.test/outro.zip"},
		"outro digest": {Distribution: DistributionBinary, SHA256: strings.Repeat("c", 64)},
	}
	for nome, confirmado := range confirmados {
		if _, err := semNode.instalador.Install(context.Background(), opencodeID, confirmado); !errors.Is(err, ErrPlanChanged) {
			t.Errorf("%s: erro = %v, queria a recusa do plano vencido", nome, err)
		}
	}
	// E a instalação sem exigência nenhuma continua valendo, para quem programa
	// contra o pacote em vez de contra a tela.
	livre := montar(t, opcoes{
		agentes: []acpregistry.Agent{agente},
		runtime: runtimeSemNode,
		http:    &clienteFalso{corpo: pacote},
	})
	if _, err := livre.instalador.Install(context.Background(), opencodeID, Confirmed{}); err != nil {
		t.Errorf("não instalou sem plano confirmado: %v", err)
	}
}

func TestSemNodeEComArtefatoQueNaoServeOMotivoContinuaSendoORuntime(t *testing.T) {
	// Cair para o binário só ajuda quando ele é instalável. Com um instalador
	// do sistema, que o app não abre, desviar para lá trocaria "instale o
	// Node" — que resolve — por "este formato não é instalável", que não dá o
	// que fazer a quem tem o caminho npm à mão.
	agente := agenteOpencode(t, strings.Repeat("a", 64))
	alvo := agente.Distribution.Binary[PlatformTarget()]
	alvo.Archive = "https://exemplo.test/opencode.msi"
	agente.Distribution.Binary[PlatformTarget()] = alvo
	agente.Distribution.NPX = &acpregistry.PackageDistribution{Package: "opencode-acp@" + opencodeVersao}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Distribution != DistributionNPM {
		t.Errorf("distribuição = %q, queria continuar no npm", plano.Distribution)
	}
	if !strings.Contains(plano.Reason, "Node") {
		t.Errorf("motivo = %q, queria que ele nomeasse o Node que falta", plano.Reason)
	}
}

func TestSemNodeOBinarioSemDigestPassaASerOCaminho(t *testing.T) {
	// É o caso que a Fase 5 abre, e ele é o do Cursor: máquina sem Node, agente
	// que publica os dois caminhos e não publica digest. Antes o plano parava em
	// "instale o Node" para usar um npm de que o artefato não precisa; agora ele
	// oferece o artefato, marcado como o que o app não consegue conferir (D4).
	agente := agenteOpencode(t, "")
	agente.Distribution.NPX = &acpregistry.PackageDistribution{Package: "opencode-acp@" + opencodeVersao}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}, runtime: runtimeSemNode})

	plano, err := c.instalador.Plan(context.Background(), opencodeID)
	if err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}
	if plano.Distribution != DistributionBinary {
		t.Errorf("distribuição = %q, queria o artefato binário", plano.Distribution)
	}
	if !plano.CanInstall || !plano.Unverified {
		t.Errorf("plano = %+v, queria oferecer instalação marcada como não verificável", plano)
	}
	if plano.Runtime.Required {
		t.Error("exigiu o Node para instalar um artefato que não o usa")
	}
}

func TestOComandoDoRegistroQueSobeComoProcessoEOQueVaiParaOProvider(t *testing.T) {
	dir := t.TempDir()
	nome := nomeExecutavel("agente")
	escreverExecutavel(t, filepath.Join(dir, nome))

	comando, args, err := resolveBinaryCommand(dir,
		acpregistry.BinaryTarget{Cmd: "./" + nome, Args: []string{"--acp"}}, acp.NodeRuntime{})
	if err != nil {
		t.Fatalf("não resolveu o comando publicado: %v", err)
	}
	if comando != filepath.Join(dir, nome) {
		t.Errorf("comando = %q, queria o executável extraído", comando)
	}
	if len(args) != 1 || args[0] != "--acp" {
		t.Errorf("argumentos = %v, queria os do registro", args)
	}
}

func TestOBinarioSemBitDeExecucaoRecebeOBitEmVezDeFalharNoSpawn(t *testing.T) {
	// Zip montado no Windows não guarda modo POSIX, e o executável sai da
	// extração sem o bit. Aceitá-lo assim empurraria a falha para o handshake,
	// longe de onde ela é explicável; recusá-lo desistiria de um agente que só
	// precisa de um chmod no que o próprio app acabou de escrever.
	if runtime.GOOS == "windows" {
		t.Skip("no Windows o bit de execução não existe")
	}
	dir := t.TempDir()
	alvo := filepath.Join(dir, "agente")
	if err := os.WriteFile(alvo, []byte("binário de mentira"), 0o644); err != nil {
		t.Fatalf("erro ao escrever o executável: %v", err)
	}

	comando, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: "./agente"}, acp.NodeRuntime{})
	if err != nil {
		t.Fatalf("não resolveu o comando sem bit de execução: %v", err)
	}
	if comando != alvo {
		t.Errorf("comando = %q, queria %q", comando, alvo)
	}
	info, err := os.Stat(alvo)
	if err != nil {
		t.Fatalf("erro ao conferir o executável: %v", err)
	}
	if info.Mode().Perm()&0o111 == 0 {
		t.Errorf("modo = %v, queria o bit de execução ligado", info.Mode().Perm())
	}
}

func TestOPontoDeEntradaDeScriptSobePeloNode(t *testing.T) {
	// O que o Node executa não é executável: passá-lo direto ao sistema não
	// criaria processo nenhum. O par `node` + arquivo é o mesmo que a instalação
	// por npm grava, e pelo mesmo motivo (D8, AEP-0084 D15).
	dir := t.TempDir()
	escreverExecutavel(t, filepath.Join(dir, "agente.js"))
	node := acp.NodeRuntime{Found: true, Node: filepath.Join(dir, "node")}

	comando, args, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: "agente.js", Args: []string{"--acp"}}, node)
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada de script: %v", err)
	}
	if comando != node.Node {
		t.Errorf("comando = %q, queria o Node", comando)
	}
	if len(args) != 2 || args[0] != filepath.Join(dir, "agente.js") || args[1] != "--acp" {
		t.Errorf("argumentos = %v, queria o script antes dos argumentos do registro", args)
	}

	// E sem Node a falha nomeia o que falta, em vez de dizer só que não deu.
	if _, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: "agente.js"}, acp.NodeRuntime{}); err == nil ||
		!strings.Contains(err.Error(), RuntimeNode) {
		t.Errorf("erro = %v, queria que ele nomeasse o Node ausente", err)
	}
}

func TestNoWindowsOCmdQueNaoSobeEResolvidoParaOExecutavelAoLado(t *testing.T) {
	// Dois casos reais: o `./opencode` sem extensão do alvo ARM, que entrega um
	// `opencode.exe`, e o `.cmd` do Cursor, que embrulha um `.exe` de mesmo
	// nome. Um arquivo de lote deixaria o agente como processo neto, e matar o
	// que o app segura não encerraria quem está editando arquivos.
	if runtime.GOOS != "windows" {
		t.Skip("fora do Windows todo arquivo sobe como processo")
	}
	casos := []struct{ nome, cmd, executavel string }{
		{"sem extensão", "./opencode", "opencode.exe"},
		{"arquivo de lote", "./dist-package\\cursor-agent.cmd", filepath.Join("dist-package", "cursor-agent.exe")},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			dir := t.TempDir()
			alvo := filepath.Join(dir, caso.executavel)
			if err := os.MkdirAll(filepath.Dir(alvo), 0o755); err != nil {
				t.Fatalf("erro ao montar a instalação: %v", err)
			}
			escreverExecutavel(t, alvo)

			comando, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: caso.cmd}, acp.NodeRuntime{})
			if err != nil {
				t.Fatalf("não resolveu o comando: %v", err)
			}
			if comando != alvo {
				t.Errorf("comando = %q, queria %q", comando, alvo)
			}
		})
	}
}

func TestOCmdQueApontaParaForaDaInstalacaoERecusado(t *testing.T) {
	// O `cmd` é do índice, que é dado externo (D9): um que aponte para fora do
	// diretório da instalação viraria linha de comando de um provider.
	dir := t.TempDir()
	for _, cmd := range []string{"../fora", "/usr/bin/agente", "./sub/../../fora"} {
		if _, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: cmd}, acp.NodeRuntime{}); !errors.Is(err, ErrCommandNotResolved) {
			t.Errorf("%q: erro = %v, queria a recusa do comando", cmd, err)
		}
	}
}

func TestOCmdQueEOProprioDiretorioERecusado(t *testing.T) {
	// O `.` fica dentro do diretório e passaria pela guarda de travessia, mas
	// como comando ele é o próprio diretório da instalação — e a alternativa do
	// Windows sobre ele seria `<dir>.exe`, um irmão da instalação. O caminho de
	// fora aparece aqui sem `..` nenhum, então a recusa é a do arquivo.
	dir := filepath.Join(t.TempDir(), "instalacao")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("erro ao preparar o diretório: %v", err)
	}
	escreverExecutavel(t, dir+".exe")

	for _, cmd := range []string{".", "./", "./sub/.."} {
		comando, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: cmd}, acp.NodeRuntime{})
		if !errors.Is(err, ErrCommandNotResolved) {
			t.Errorf("%q: comando = %q, erro = %v, queria a recusa do comando", cmd, comando, err)
		}
	}
}

func TestOComandoQueNaoEstaLaFalhaDizendoOndeSeProcurou(t *testing.T) {
	// "Não consegui resolver o comando" não é verificável por quem abriu o
	// diretório para olhar: a mensagem diz o que foi procurado.
	dir := t.TempDir()
	_, _, err := resolveBinaryCommand(dir, acpregistry.BinaryTarget{Cmd: "./agente"}, acp.NodeRuntime{})
	if !errors.Is(err, ErrCommandNotResolved) {
		t.Fatalf("erro = %v, queria a recusa do comando", err)
	}
	if !strings.Contains(err.Error(), "agente") {
		t.Errorf("erro = %q, queria que ele dissesse o que procurou", err)
	}
}

// escreverExecutavel deixa no lugar um arquivo que o teste trata como binário.
func escreverExecutavel(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("binário de mentira"), 0o755); err != nil {
		t.Fatalf("erro ao escrever %s: %v", path, err)
	}
}
