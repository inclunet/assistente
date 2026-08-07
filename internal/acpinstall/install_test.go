package acpinstall

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
)

// O pacote e a versão do adaptador do Codex, que é o agente do critério de
// aceitação da Fase 3.
const (
	codexID      = "codex-acp"
	codexPacote  = "@agentclientprotocol/codex-acp"
	codexVersao  = "1.1.9"
	codexBinario = "dist/index.js"
)

// catalogoFalso serve um catálogo fixo, no lugar do serviço do registro.
type catalogoFalso struct {
	agentes []acpregistry.Agent
	motivo  string
}

func (c catalogoFalso) Catalog(context.Context) acpregistry.Catalog {
	return acpregistry.Catalog{Version: "1.0.0", Agents: c.agentes, Reason: c.motivo}
}

// agenteCodex é a linha do catálogo do adaptador do Codex, como o registro a
// publica: o pacote com a versão fixada e o argumento que o agente pede.
func agenteCodex() acpregistry.Agent {
	return acpregistry.Agent{
		ID:      codexID,
		Name:    "Codex",
		Version: codexVersao,
		Distribution: acpregistry.Distribution{
			NPX: &acpregistry.PackageDistribution{
				Package: codexPacote + "@" + codexVersao,
				Args:    []string{"--acp"},
			},
		},
	}
}

// npmFalso é o npm que o teste roda no lugar do de verdade: ele escreve no disco
// o pacote que o npm escreveria, e o CI não depende de baixar nada.
type npmFalso struct {
	// pacote e binario dizem o que montar no prefixo.
	pacote  string
	binario string
	// semComando é o npm que não sabe dizer a linha que executaria, que é o que
	// acontece quando há Node mas o `npm-cli.js` não está ao lado dele.
	semComando bool
	// manifesto substitui o manifesto padrão quando não está vazio.
	manifesto string
	// comShim faz o npm falso ligar também o atalho de lote em
	// `node_modules/.bin`, que é o que o npm de verdade cria no Windows.
	comShim bool
	// erro é o que devolver em vez de instalar.
	erro error
	// bloqueia, quando não é nil, segura a instalação até o contexto morrer —
	// é como o teste observa o cancelamento no meio do caminho.
	bloqueia chan struct{}

	mu    sync.Mutex
	specs []string
}

func (n *npmFalso) Install(ctx context.Context, prefix, spec string) error {
	n.mu.Lock()
	n.specs = append(n.specs, spec)
	n.mu.Unlock()

	if n.bloqueia != nil {
		// Deixa rastro no disco antes de parar: é o resíduo que o cancelamento
		// tem de limpar.
		_ = os.MkdirAll(filepath.Join(prefix, "node_modules"), 0o755)
		close(n.bloqueia)
		n.bloqueia = nil
		<-ctx.Done()
		return ctx.Err()
	}
	if n.erro != nil {
		return n.erro
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	dir := filepath.Join(prefix, "node_modules", filepath.FromSlash(n.pacote))
	if err := os.MkdirAll(filepath.Dir(filepath.Join(dir, filepath.FromSlash(n.binario))), 0o755); err != nil {
		return err
	}
	manifesto := n.manifesto
	if manifesto == "" {
		manifesto = `{"name":"` + n.pacote + `","version":"` + codexVersao + `","bin":{"codex-acp":"` + n.binario + `"}}`
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifesto), 0o644); err != nil {
		return err
	}
	if n.comShim {
		atalhos := filepath.Join(prefix, "node_modules", ".bin")
		if err := os.MkdirAll(atalhos, 0o755); err != nil {
			return err
		}
		for _, atalho := range []string{"codex-acp", "codex-acp.cmd", "codex-acp.ps1"} {
			if err := os.WriteFile(filepath.Join(atalhos, atalho), []byte("@echo off\n"), 0o755); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(filepath.Join(dir, filepath.FromSlash(n.binario)), []byte("#!/usr/bin/env node\n"), 0o755)
}

func (n *npmFalso) Describe(prefix, spec string) string {
	if n.semComando {
		return ""
	}
	return "node npm-cli.js install --prefix " + prefix + " " + spec
}

func (n *npmFalso) especificacoes() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.specs)
}

// runtimeComNode é a máquina que tem Node, sem executar nada para descobrir.
func runtimeComNode() acp.NodeRuntime {
	return acp.NodeRuntime{
		Found:     true,
		Node:      filepath.Join("C:", "Program Files", "nodejs", "node.exe"),
		NPMScript: filepath.Join("C:", "Program Files", "nodejs", "node_modules", "npm", "bin", "npm-cli.js"),
		Version:   "24.4.1",
	}
}

func runtimeSemNode() acp.NodeRuntime {
	return acp.NodeRuntime{Searched: []string{filepath.Join("C:", "Program Files", "nodejs", "node.exe")}}
}

// marcos guarda os progressos recebidos, para o teste conferir que os marcos
// aconteceram e em que ordem (D13).
type marcos struct {
	mu    sync.Mutex
	itens []Progress
}

func (m *marcos) registrar(p Progress) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.itens = append(m.itens, p)
}

func (m *marcos) etapas() []Stage {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Stage, 0, len(m.itens))
	for _, item := range m.itens {
		out = append(out, item.Stage)
	}
	return out
}

func (m *marcos) ultimo() Progress {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.itens) == 0 {
		return Progress{}
	}
	return m.itens[len(m.itens)-1]
}

// cenario monta um instalador com tudo substituído: catálogo, npm, runtime e
// handshake. Nada aqui toca a rede nem sobe processo.
type cenario struct {
	instalador *Installer
	npm        *npmFalso
	marcos     *marcos
	root       string
	// handshakes conta quantas vezes o comando resolvido foi conferido.
	handshakes []([]string)
}

type opcoes struct {
	agentes   []acpregistry.Agent
	motivo    string
	runtime   func() acp.NodeRuntime
	npm       *npmFalso
	handshake error
}

func montar(t *testing.T, opts opcoes) *cenario {
	t.Helper()
	if opts.agentes == nil {
		opts.agentes = []acpregistry.Agent{agenteCodex()}
	}
	if opts.runtime == nil {
		opts.runtime = runtimeComNode
	}
	if opts.npm == nil {
		opts.npm = &npmFalso{pacote: codexPacote, binario: codexBinario}
	}
	c := &cenario{npm: opts.npm, marcos: &marcos{}, root: t.TempDir()}
	c.instalador = New(Config{
		Root:    c.root,
		Source:  catalogoFalso{agentes: opts.agentes, motivo: opts.motivo},
		NPM:     opts.npm,
		Runtime: opts.runtime,
		Handshake: func(_ context.Context, command string, args []string) error {
			c.handshakes = append(c.handshakes, append([]string{command}, args...))
			return opts.handshake
		},
		OnProgress: c.marcos.registrar,
		Now:        func() time.Time { return time.Date(2026, 8, 6, 12, 0, 0, 0, time.UTC) },
	})
	return c
}

func TestInstallResolveOParNodeMaisPontoDeEntradaEGravaORegistro(t *testing.T) {
	c := montar(t, opcoes{})

	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou o agente do catálogo: %v", err)
	}

	// O comando é o par `node` + ponto de entrada, e nunca o `.cmd` que o npm
	// liga: o app precisa conseguir encerrar o processo do agente (D8).
	if instalacao.Command != runtimeComNode().Node {
		t.Errorf("comando = %q, queria o node encontrado na máquina", instalacao.Command)
	}
	entrada := filepath.Join(c.root, codexID, codexVersao, "node_modules",
		filepath.FromSlash(codexPacote), filepath.FromSlash(codexBinario))
	// Os `args` do registro vão depois do ponto de entrada, na ordem publicada.
	if !slices.Equal(instalacao.Args, []string{entrada, "--acp"}) {
		t.Errorf("argumentos = %q, queria o ponto de entrada seguido de --acp", instalacao.Args)
	}
	// A instalação mora em `<root>/<id>/<versão>` (D5).
	if instalacao.Dir != filepath.Join(c.root, codexID, codexVersao) {
		t.Errorf("diretório = %q, queria <root>/<id>/<versão>", instalacao.Dir)
	}

	// E o `installed.json` guarda o que o app fez, para ele não ter de
	// reconstruir por adivinhação a cada abertura.
	data, err := os.ReadFile(filepath.Join(instalacao.Dir, installedFileName))
	if err != nil {
		t.Fatalf("não gravou o installed.json: %v", err)
	}
	var gravado Installation
	if err := json.Unmarshal(data, &gravado); err != nil {
		t.Fatalf("installed.json ilegível: %v", err)
	}
	if gravado.AgentID != codexID || gravado.Version != codexVersao {
		t.Errorf("registro = %+v, queria o identificador e a versão do agente", gravado)
	}
	if gravado.Distribution != DistributionNPM || gravado.Target != codexPacote+"@"+codexVersao {
		t.Errorf("registro = %+v, queria a distribuição e o alvo instalados", gravado)
	}
	if gravado.Command != instalacao.Command || !slices.Equal(gravado.Args, instalacao.Args) {
		t.Errorf("registro = %+v, queria o comando resolvido", gravado)
	}
	if gravado.InstalledAt.IsZero() {
		t.Error("registro sem data")
	}
}

func TestInstallIgnoraOAtalhoDeLoteQueONpmLiga(t *testing.T) {
	// O npm liga `node_modules/.bin/codex-acp.cmd` no Windows, e é o que ele
	// mesmo publica para rodar. O app não pode usá-lo: matar um arquivo de lote
	// mata o interpretador e deixa o agente vivo, e é por isso que `spawnable()`
	// o recusa de propósito (D8, AEP-0084 D15). O comando resolvido vem do `bin`
	// do manifesto, e o atalho existir do lado não muda isso.
	c := montar(t, opcoes{npm: &npmFalso{pacote: codexPacote, binario: codexBinario, comShim: true}})

	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou o agente do catálogo: %v", err)
	}

	// O atalho está no disco: é justamente a armadilha que o teste monta.
	atalho := filepath.Join(instalacao.Dir, "node_modules", ".bin", "codex-acp.cmd")
	if _, err := os.Stat(atalho); err != nil {
		t.Fatalf("o cenário não montou o atalho de lote: %v", err)
	}

	for _, parte := range append([]string{instalacao.Command}, instalacao.Args...) {
		switch strings.ToLower(filepath.Ext(parte)) {
		case ".cmd", ".bat", ".ps1":
			t.Errorf("comando resolvido usa %q, que o app não consegue encerrar", parte)
		}
	}
	if filepath.Base(instalacao.Args[0]) != filepath.Base(codexBinario) {
		t.Errorf("ponto de entrada = %q, queria o `bin` do manifesto", instalacao.Args[0])
	}
	if strings.Contains(instalacao.Args[0], string(filepath.Separator)+".bin"+string(filepath.Separator)) {
		t.Errorf("ponto de entrada = %q, queria o arquivo do pacote e não o atalho", instalacao.Args[0])
	}
}

func TestInstallSoTerminaBemDepoisDoHandshake(t *testing.T) {
	// A instalação só é declarada concluída depois de o comando resolvido
	// responder `initialize` (D8): um provider salvo que nunca sobe é pior do que
	// uma instalação que falhou.
	c := montar(t, opcoes{})

	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	if len(c.handshakes) != 1 {
		t.Fatalf("handshakes = %d, queria exatamente um", len(c.handshakes))
	}
	conferido := c.handshakes[0]
	if !slices.Equal(conferido, append([]string{instalacao.Command}, instalacao.Args...)) {
		t.Errorf("conferiu %q, queria o comando que foi gravado", conferido)
	}
}

func TestInstallAnunciaOsMarcosNaOrdem(t *testing.T) {
	// Marcos, e não bytes (D13): começou, instalando, conferindo o agente,
	// pronto.
	c := montar(t, opcoes{})

	if _, err := c.instalador.Install(context.Background(), codexID); err != nil {
		t.Fatalf("não instalou: %v", err)
	}

	queria := []Stage{StageStarted, StageInstalling, StageVerifying, StageDone}
	if etapas := c.marcos.etapas(); !slices.Equal(etapas, queria) {
		t.Errorf("marcos = %v, queria %v", etapas, queria)
	}
}

func TestPlanSemNodeNaoOferecInstalacaoEDizOMotivo(t *testing.T) {
	// Sem Node a instalação não é oferecida e o motivo fica em texto (D7). Botão
	// cinza sem explicação é o pior desfecho para quem navega por teclado.
	c := montar(t, opcoes{runtime: runtimeSemNode})

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if plano.CanInstall {
		t.Error("ofereceu instalação numa máquina sem Node")
	}
	if plano.Runtime.Found || plano.Runtime.Name != RuntimeNode {
		t.Errorf("runtime = %+v, queria o Node.js dito como ausente", plano.Runtime)
	}
	if !strings.Contains(plano.Reason, "Node") {
		t.Errorf("motivo = %q, queria que ele nomeasse o Node", plano.Reason)
	}
	if len(plano.Runtime.Searched) == 0 {
		t.Error("não disse onde procurou o Node")
	}

	_, err = c.instalador.Install(context.Background(), codexID)
	if !errors.Is(err, ErrRuntimeMissing) {
		t.Errorf("erro = %v, queria a falta do runtime", err)
	}
	if StepOf(err) != StepRuntime {
		t.Errorf("etapa = %q, queria a do runtime: erro que não nomeia a etapa não é acionável", StepOf(err))
	}
	if len(c.npm.especificacoes()) != 0 {
		t.Error("chamou o npm numa máquina sem Node")
	}
}

func TestPlanComNodeMasSemNpmNaoOfereceInstalacao(t *testing.T) {
	// Node encontrado não garante npm ao lado dele. Sem a linha de comando a
	// confirmação prometeria executar nada, e a instalação falharia depois de
	// aceita — o consentimento do D3 é sobre um comando, e não sobre um vazio.
	c := montar(t, opcoes{npm: &npmFalso{pacote: codexPacote, binario: codexBinario, semComando: true}})

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if plano.CanInstall {
		t.Error("ofereceu instalação sem ter comando para mostrar")
	}
	if plano.InstallCommand != "" {
		t.Errorf("comando = %q, queria vazio", plano.InstallCommand)
	}
	if !strings.Contains(plano.Reason, "npm") {
		t.Errorf("motivo = %q, queria que ele nomeasse o npm que falta", plano.Reason)
	}
}

func TestPlanMostraOrigemVersaoEOComandoDeInstalacao(t *testing.T) {
	// Antes de baixar qualquer byte, o diálogo mostra o que vai ser baixado e o
	// que será executado (D3).
	c := montar(t, opcoes{})

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("plano falhou: %v", err)
	}
	if !plano.CanInstall {
		t.Fatalf("não ofereceu instalação com Node presente: %+v", plano)
	}
	if plano.Origin != codexPacote+"@"+codexVersao {
		t.Errorf("origem = %q, queria o nome completo do pacote com a versão", plano.Origin)
	}
	if plano.Version != codexVersao || plano.Distribution != DistributionNPM {
		t.Errorf("plano = %+v, queria versão e tipo de distribuição", plano)
	}
	if plano.Dir != filepath.Join(c.root, codexID, codexVersao) {
		t.Errorf("diretório = %q, queria o de dados do app", plano.Dir)
	}
	if !strings.Contains(plano.InstallCommand, codexPacote+"@"+codexVersao) {
		t.Errorf("comando de instalação = %q, queria a linha que será executada", plano.InstallCommand)
	}
	if !slices.Equal(plano.RunArgs, []string{"--acp"}) {
		t.Errorf("argumentos de execução = %q, queria os do registro", plano.RunArgs)
	}
	if plano.Installed != nil {
		t.Error("disse que já estava instalado antes de instalar")
	}
}

func TestInstallUsaAVersaoFixadaPeloRegistro(t *testing.T) {
	// A versão instalada é a que o registro fixa, e não `latest` (D6): é o que
	// faz a instalação ser reproduzível.
	c := montar(t, opcoes{})

	if _, err := c.instalador.Install(context.Background(), codexID); err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	specs := c.npm.especificacoes()
	if !slices.Equal(specs, []string{codexPacote + "@" + codexVersao}) {
		t.Errorf("pediu ao npm %q, queria o pacote com a versão fixada", specs)
	}
}

func TestInstallRecusaPacoteSemVersaoFixada(t *testing.T) {
	agente := agenteCodex()
	agente.Distribution.NPX.Package = codexPacote
	agente.Version = ""
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	_, err := c.instalador.Install(context.Background(), codexID)
	if !errors.Is(err, ErrUnpinnedVersion) {
		t.Errorf("erro = %v, queria a recusa da versão não fixada", err)
	}
	if len(c.npm.especificacoes()) != 0 {
		t.Error("chamou o npm sem versão fixada")
	}
}

func TestInstallUsaAVersaoDoItemQuandoOPacoteNaoAPina(t *testing.T) {
	// O nome do pacote sem versão e o item publicando uma é o registro fixando a
	// versão por outro campo — e continua sendo o registro fixando.
	agente := agenteCodex()
	agente.Distribution.NPX.Package = codexPacote
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	if instalacao.Version != codexVersao {
		t.Errorf("versão = %q, queria a publicada pelo item", instalacao.Version)
	}
}

func TestInstallCanceladoNaoDeixaResiduo(t *testing.T) {
	// A instalação pode ser cancelada sem deixar resíduo (D13): uma instalação
	// interrompida não deixa meio agente no disco.
	comecou := make(chan struct{})
	c := montar(t, opcoes{npm: &npmFalso{pacote: codexPacote, binario: codexBinario, bloqueia: comecou}})

	pronto := make(chan error, 1)
	go func() {
		_, err := c.instalador.Install(context.Background(), codexID)
		pronto <- err
	}()

	<-comecou
	if !c.instalador.Installing(codexID) {
		t.Error("não registrou a instalação em voo, e então não haveria o que cancelar")
	}
	if !c.instalador.Cancel(codexID) {
		t.Fatal("não achou a instalação para cancelar")
	}

	select {
	case err := <-pronto:
		if err == nil {
			t.Fatal("a instalação cancelada terminou bem")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a instalação não respondeu ao cancelamento")
	}

	dir := filepath.Join(c.root, codexID, codexVersao)
	if _, err := os.Stat(dir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("sobrou resíduo em %s (stat: %v)", dir, err)
	}
	if _, instalado := c.instalador.Installed(codexID); instalado {
		t.Error("a instalação cancelada aparece como instalada")
	}
	if etapa := c.marcos.ultimo().Stage; etapa != StageCancelled {
		t.Errorf("último marco = %q, queria o cancelamento: cancelar é decisão, e não falha", etapa)
	}
}

func TestOPlanoProcuraORuntimeUmaVezSo(t *testing.T) {
	// O comando que o plano mostra tem de ser o do Node que ele diz ter
	// encontrado. Com duas procuras, o que a confirmação promete executar sai de
	// uma delas e o "Node encontrado em" sai da outra — e a máquina pode ter
	// mudado no meio.
	procuras := 0
	c := montar(t, opcoes{runtime: func() acp.NodeRuntime {
		procuras++
		return runtimeComNode()
	}})
	// O npm de mentira do cenário atende sem consultar o runtime; o de verdade é
	// quem procuraria de novo, e é o caminho que a contagem cobre.
	c.instalador.npm = lazyNPM{lookup: c.instalador.runtime}

	if _, err := c.instalador.Plan(context.Background(), codexID); err != nil {
		t.Fatalf("o plano falhou: %v", err)
	}

	if procuras != 1 {
		t.Errorf("procurou o runtime %d vezes, queria uma", procuras)
	}
}

func TestInstallQueEstouraOPrazoFalhaEmVezDeDizerQueFoiCancelada(t *testing.T) {
	// Cancelar é decisão de quem clicou. Prazo estourado não é decisão de
	// ninguém, e anunciá-lo como cancelamento diria "nada ficou no disco, você
	// pediu" para quem não pediu nada — sem nomear a etapa que travou.
	comecou := make(chan struct{})
	c := montar(t, opcoes{npm: &npmFalso{pacote: codexPacote, binario: codexBinario, bloqueia: comecou}})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := c.instalador.Install(ctx, codexID); err == nil {
		t.Fatal("a instalação que estourou o prazo terminou bem")
	}

	if etapa := c.marcos.ultimo().Stage; etapa != StageFailed {
		t.Errorf("último marco = %q, queria a falha: ninguém pediu para parar", etapa)
	}
	if passo := c.marcos.ultimo().Step; passo == "" {
		t.Error("a falha não nomeou a etapa, que é o que a torna acionável (D13)")
	}
}

func TestInstallComHandshakeQueFalhaNaoDeixaInstalacao(t *testing.T) {
	// Instalação que produz um comando que não fala ACP não é instalação
	// bem-sucedida (D8), e o desfecho ruim é "não deu para instalar" em vez de
	// "provider salvo que falha na primeira conversa".
	c := montar(t, opcoes{handshake: errors.New("o agente não respondeu ao initialize")})

	_, err := c.instalador.Install(context.Background(), codexID)
	if err == nil {
		t.Fatal("declarou concluída uma instalação cujo agente não subiu")
	}
	if StepOf(err) != StepVerify {
		t.Errorf("etapa = %q, queria a da conferência do agente", StepOf(err))
	}
	if _, err := os.Stat(filepath.Join(c.root, codexID, codexVersao)); !errors.Is(err, os.ErrNotExist) {
		t.Error("deixou a instalação que não passou pelo handshake no disco")
	}
	ultimo := c.marcos.ultimo()
	if ultimo.Stage != StageFailed || ultimo.Step != StepVerify {
		t.Errorf("último marco = %+v, queria a falha nomeando a etapa", ultimo)
	}
}

func TestInstallRepassaOErroDoNpm(t *testing.T) {
	// Node velho demais, proxy corporativo e registro npm privado são problemas
	// da máquina: quem vai resolvê-los precisa da mensagem original.
	c := montar(t, opcoes{npm: &npmFalso{
		pacote:  codexPacote,
		binario: codexBinario,
		erro:    errors.New("EBADENGINE required: { node: '>=22' }"),
	}})

	_, err := c.instalador.Install(context.Background(), codexID)
	if err == nil {
		t.Fatal("a instalação passou com o npm falhando")
	}
	if !strings.Contains(err.Error(), "EBADENGINE") {
		t.Errorf("erro = %q, queria a mensagem original do npm", err)
	}
	if StepOf(err) != StepInstall {
		t.Errorf("etapa = %q, queria a do npm", StepOf(err))
	}
	if motivo := c.marcos.ultimo().Reason; !strings.Contains(motivo, "EBADENGINE") {
		t.Errorf("motivo anunciado = %q, queria a mensagem do npm", motivo)
	}
}

func TestInstallComPontoDeEntradaNaoResolvidoFalhaEmVezDeAdivinhar(t *testing.T) {
	c := montar(t, opcoes{npm: &npmFalso{
		pacote:    codexPacote,
		binario:   codexBinario,
		manifesto: `{"name":"` + codexPacote + `","version":"` + codexVersao + `"}`,
	}})

	_, err := c.instalador.Install(context.Background(), codexID)
	if err == nil {
		t.Fatal("aceitou um pacote sem `bin`")
	}
	if StepOf(err) != StepResolve {
		t.Errorf("etapa = %q, queria a da resolução do comando", StepOf(err))
	}
	if !errors.Is(err, acp.ErrNPMEntryPoint) {
		t.Errorf("erro = %v, queria o de ponto de entrada não resolvido", err)
	}
}

func TestInstallRecusaSegundaInstalacaoDoMesmoAgente(t *testing.T) {
	// Duas escrevendo no mesmo diretório deixariam meio agente no disco.
	comecou := make(chan struct{})
	c := montar(t, opcoes{npm: &npmFalso{pacote: codexPacote, binario: codexBinario, bloqueia: comecou}})

	go func() { _, _ = c.instalador.Install(context.Background(), codexID) }()
	<-comecou

	_, err := c.instalador.Install(context.Background(), codexID)
	if !errors.Is(err, ErrInstalling) {
		t.Errorf("erro = %v, queria a recusa da instalação concorrente", err)
	}
	c.instalador.Cancel(codexID)
}

func TestInstallRecusaAgenteJaInstalado(t *testing.T) {
	c := montar(t, opcoes{})
	if _, err := c.instalador.Install(context.Background(), codexID); err != nil {
		t.Fatalf("não instalou: %v", err)
	}

	_, err := c.instalador.Install(context.Background(), codexID)
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Errorf("erro = %v, queria a recusa de reinstalar por cima", err)
	}

	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil {
		t.Fatalf("plano falhou: %v", err)
	}
	if plano.CanInstall || plano.Installed == nil {
		t.Errorf("plano = %+v, queria o estado de já instalado", plano)
	}
}

func TestRemoveApagaODiretorioEDeixaOComandoInexistente(t *testing.T) {
	// Remover é apagar o diretório do agente (D5). O provider que apontava para
	// lá fica com um comando que não existe, e o health do AEP-0084 D12 já sabe
	// dizer isso — não há estado novo a inventar.
	c := montar(t, opcoes{})
	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	entrada := instalacao.Args[0]
	if _, err := os.Stat(entrada); err != nil {
		t.Fatalf("o ponto de entrada não estava no disco depois de instalar: %v", err)
	}

	if err := c.instalador.Remove(context.Background(), codexID); err != nil {
		t.Fatalf("não removeu: %v", err)
	}
	if _, err := os.Stat(filepath.Join(c.root, codexID)); !errors.Is(err, os.ErrNotExist) {
		t.Error("o diretório do agente sobreviveu à remoção")
	}
	if _, err := os.Stat(entrada); !errors.Is(err, os.ErrNotExist) {
		t.Error("o ponto de entrada gravado no provider continua existindo depois da remoção")
	}
	if _, instalado := c.instalador.Installed(codexID); instalado {
		t.Error("o agente removido continua aparecendo como instalado")
	}
	// E depois de remover dá para instalar de novo: a remoção não deixa estado
	// que atrapalhe.
	plano, err := c.instalador.Plan(context.Background(), codexID)
	if err != nil || !plano.CanInstall {
		t.Errorf("plano depois da remoção = %+v (erro %v), queria instalação oferecida", plano, err)
	}
}

func TestRemoveDeAgenteNaoInstaladoDizIsso(t *testing.T) {
	c := montar(t, opcoes{})

	if err := c.instalador.Remove(context.Background(), codexID); !errors.Is(err, ErrNotInstalled) {
		t.Errorf("erro = %v, queria dizer que não havia instalação", err)
	}
}

func TestListSoContaOQueTemRegistroLegivel(t *testing.T) {
	// Diretório sem `installed.json` legível é resíduo, e tratá-lo como
	// instalação faria a tela oferecer usar um agente que não existe.
	c := montar(t, opcoes{})
	if _, err := c.instalador.Install(context.Background(), codexID); err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(c.root, "residuo", "9.9.9", "node_modules"), 0o755); err != nil {
		t.Fatalf("não deu para montar o resíduo: %v", err)
	}

	lista := c.instalador.List()
	if len(lista) != 1 || lista[0].AgentID != codexID {
		t.Errorf("lista = %+v, queria só o agente instalado", lista)
	}
}

// regravarRegistro reescreve o `installed.json` de uma instalação depois de
// mexer nele. É como o arquivo é adulterado no disco de quem usa o app: ele fica
// no home, e nada impede que tenha sido copiado, editado à mão ou deixado por um
// layout antigo.
func regravarRegistro(t *testing.T, dir string, mexer func(*Installation)) {
	t.Helper()
	registro, err := readInstallation(dir)
	if err != nil {
		t.Fatalf("não deu para ler o registro em %s: %v", dir, err)
	}
	mexer(&registro)
	if err := writeInstallation(dir, registro); err != nil {
		t.Fatalf("não deu para regravar o registro: %v", err)
	}
}

func TestRegistroGrandeDemaisNaoVaiInteiroParaAMemoria(t *testing.T) {
	// O registro que o app escreve tem algumas centenas de bytes. Um adulterado
	// de tamanho absurdo seria lido inteiro só por alguém ter aberto a tela que
	// lista o que está instalado.
	c := montar(t, opcoes{})
	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	gordo := append(make([]byte, 0, maxInstalledBytes+8), []byte(`{"schema":1,"command":"x"`)...)
	for len(gordo) <= maxInstalledBytes {
		gordo = append(gordo, ' ')
	}
	if err := os.WriteFile(filepath.Join(instalacao.Dir, installedFileName), append(gordo, '}'), 0o644); err != nil {
		t.Fatalf("não deu para inchar o registro: %v", err)
	}

	if _, ok := c.instalador.Installed(codexID); ok {
		t.Error("aceitou um registro maior que o teto")
	}
}

func TestORegistroSobreviveAQuedaNoMeioDaGravacao(t *testing.T) {
	// O registro é o que declara a instalação existente. Gravado por cima, uma
	// queda no meio deixaria um JSON pela metade, e o agente instalado sumiria
	// da tela na abertura seguinte — registro ilegível não conta como instalação.
	dir := t.TempDir()
	antigo := Installation{Schema: installationSchema, AgentID: codexID, Version: "1.0.0", Command: "node"}
	if err := writeInstallation(dir, antigo); err != nil {
		t.Fatalf("não gravou o registro antigo: %v", err)
	}

	novo := antigo
	novo.Version = "2.0.0"
	if err := writeInstallation(dir, novo); err != nil {
		t.Fatalf("não regravou o registro: %v", err)
	}

	lido, err := readInstallation(dir)
	if err != nil {
		t.Fatalf("o registro regravado ficou ilegível: %v", err)
	}
	if lido.Version != "2.0.0" {
		t.Errorf("versão = %q, queria a regravada", lido.Version)
	}
	// E o temporário não fica para trás: o diretório da instalação é lido por
	// versão, e lixo ali vira pergunta na próxima abertura.
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("não deu para listar o diretório: %v", err)
	}
	if len(entradas) != 1 {
		t.Errorf("sobrou arquivo além do registro: %d entradas", len(entradas))
	}
}

func TestRegistroQueDescreveOutraInstalacaoNaoContaComoInstalado(t *testing.T) {
	// O caminho é `<id>/<versão>` (D5), e o registro tem de falar do diretório em
	// que está. Um que veio de outro lugar diria à tela uma versão que não é a
	// que está no disco, e a comparação que decide se há atualização (D10)
	// passaria a comparar o número errado.
	c := montar(t, opcoes{})
	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	regravarRegistro(t, instalacao.Dir, func(r *Installation) { r.Version = "0.0.1" })

	if _, ok := c.instalador.Installed(codexID); ok {
		t.Error("aceitou um registro que descreve outra versão")
	}
	if lista := c.instalador.List(); len(lista) != 0 {
		t.Errorf("lista = %+v, queria vazia", lista)
	}
}

func TestRegistroQueApontaParaForaDaInstalacaoNaoContaComoInstalado(t *testing.T) {
	// Se o registro pudesse apontar para qualquer executável da máquina, trocar o
	// arquivo bastaria para o app subir outra coisa achando que subiu o agente
	// que instalou — e a tela ainda ofereceria "usar o comando instalado" (D9).
	c := montar(t, opcoes{})
	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	fora := filepath.Join(t.TempDir(), "outra-coisa.js")
	if err := os.WriteFile(fora, []byte("//"), 0o644); err != nil {
		t.Fatalf("não deu para gravar o arquivo de fora: %v", err)
	}
	regravarRegistro(t, instalacao.Dir, func(r *Installation) { r.Args = []string{fora} })

	if _, ok := c.instalador.Installed(codexID); ok {
		t.Error("aceitou um registro que executa algo de fora da instalação")
	}
}

func TestComDuasVersoesNoDiscoValeAInstaladaPorUltimo(t *testing.T) {
	// Duas versões podem morar lado a lado: é isso que permite baixar a nova sem
	// derrubar a que está em uso (D10). Pela ordem do diretório, que é
	// alfabética, a `10.0.0` viria antes da `2.0.0` — e a tela mostraria a
	// instalação errada.
	c := montar(t, opcoes{})
	antiga, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	// A versão nova tem número menor em ordem alfabética justamente para o teste
	// não passar por acidente.
	nova := versaoNoDisco(t, c.root, codexID, "10.0.0", antiga.InstalledAt.Add(time.Hour))

	instalada, ok := c.instalador.Installed(codexID)
	if !ok {
		t.Fatal("não achou instalação nenhuma")
	}
	if instalada.Version != nova {
		t.Errorf("versão = %q, queria a instalada por último (%q)", instalada.Version, nova)
	}
}

// versaoNoDisco monta uma instalação já pronta em `<root>/<id>/<versão>`, como a
// que sobra de uma atualização feita antes. Devolve a versão criada.
func versaoNoDisco(t *testing.T, root, agentID, version string, quando time.Time) string {
	t.Helper()
	dir := filepath.Join(root, agentID, version)
	entrada := filepath.Join(dir, "node_modules", "agente", "index.js")
	if err := os.MkdirAll(filepath.Dir(entrada), 0o755); err != nil {
		t.Fatalf("não deu para montar a versão %s: %v", version, err)
	}
	if err := os.WriteFile(entrada, []byte("//"), 0o644); err != nil {
		t.Fatalf("não deu para gravar o ponto de entrada: %v", err)
	}
	registro := Installation{
		Schema:       installationSchema,
		AgentID:      agentID,
		Name:         "Codex",
		Version:      version,
		Distribution: DistributionNPM,
		Command:      runtimeComNode().Node,
		Args:         []string{entrada},
		InstalledAt:  quando,
	}
	if err := writeInstallation(dir, registro); err != nil {
		t.Fatalf("não deu para gravar o registro da versão %s: %v", version, err)
	}
	return version
}

func TestRegistroQueSaiDaInstalacaoPorUmLinkNaoContaComoInstalado(t *testing.T) {
	// Caminho de dentro do diretório que leva para fora dele: o texto passa na
	// guarda, o destino não. Quem escreveu o registro adulterado também pode ter
	// posto o link ali, e o que se executa é o destino.
	c := montar(t, opcoes{})
	instalacao, err := c.instalador.Install(context.Background(), codexID)
	if err != nil {
		t.Fatalf("não instalou: %v", err)
	}
	fora := filepath.Join(t.TempDir(), "outra-coisa.js")
	if err := os.WriteFile(fora, []byte("//"), 0o644); err != nil {
		t.Fatalf("não deu para gravar o arquivo de fora: %v", err)
	}
	link := filepath.Join(instalacao.Dir, "atalho.js")
	if err := os.Symlink(fora, link); err != nil {
		// No Windows criar link exige privilégio, e não é a máquina que está
		// sendo testada aqui.
		t.Skipf("não deu para criar o link neste sistema: %v", err)
	}
	regravarRegistro(t, instalacao.Dir, func(r *Installation) { r.Args = []string{link} })

	if _, ok := c.instalador.Installed(codexID); ok {
		t.Error("aceitou um registro que sai da instalação por um link")
	}
}

func TestRegistroDaInstalacaoQueOAppFezContinuaValendo(t *testing.T) {
	// A guarda não pode recusar o caso normal: o `node` mora fora do diretório da
	// instalação, e é o ponto de entrada que fica dentro dele.
	c := montar(t, opcoes{})
	if _, err := c.instalador.Install(context.Background(), codexID); err != nil {
		t.Fatalf("não instalou: %v", err)
	}

	instalada, ok := c.instalador.Installed(codexID)
	if !ok {
		t.Fatal("recusou a instalação que o próprio app acabou de fazer")
	}
	if instalada.Version != codexVersao {
		t.Errorf("versão = %q, queria a instalada", instalada.Version)
	}
}

func TestPlanDeAgenteSemDistribuicaoNpmExplicaEmTexto(t *testing.T) {
	// O catálogo mostra tudo o que o registro tem (D1); o que muda é o que o app
	// diz que consegue fazer com cada linha nesta máquina.
	agente := acpregistry.Agent{
		ID:   "opencode",
		Name: "opencode",
		Distribution: acpregistry.Distribution{
			Binary: map[string]acpregistry.BinaryTarget{
				"windows-x86_64": {Archive: "https://exemplo.invalid/o.zip", Cmd: "./opencode.exe"},
			},
		},
	}
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	plano, err := c.instalador.Plan(context.Background(), "opencode")
	if err != nil {
		t.Fatalf("o plano falhou em vez de explicar: %v", err)
	}
	if plano.CanInstall {
		t.Error("ofereceu instalação por npm de um agente que só tem binário")
	}
	if !strings.Contains(plano.Reason, "npm") {
		t.Errorf("motivo = %q, queria que ele dissesse que a distribuição não é npm", plano.Reason)
	}
	// E o plano não diz que a distribuição é npm: ele se contradiria, com a
	// distribuição afirmando uma coisa e o motivo, logo abaixo, a contrária.
	if plano.Distribution != "" {
		t.Errorf("distribuição = %q, queria vazia num agente que não publica por npm", plano.Distribution)
	}

	if _, err := c.instalador.Install(context.Background(), "opencode"); !errors.Is(err, ErrNotNPM) {
		t.Errorf("erro = %v, queria a recusa da distribuição", err)
	}
}

func TestPlanDeAgenteForaDoCatalogoDizQueEleNaoEstaLa(t *testing.T) {
	c := montar(t, opcoes{})

	if _, err := c.instalador.Plan(context.Background(), "agente-que-nao-existe"); !errors.Is(err, ErrNotInCatalog) {
		t.Errorf("erro = %v, queria dizer que o agente não está no catálogo", err)
	}
}

func TestPlanSemCatalogoCarregadoRepassaOMotivo(t *testing.T) {
	// A primeira execução sem rede não tem catálogo, e o agente pode muito bem
	// estar no registro: o motivo do catálogo vazio é mais útil do que dizer que
	// o agente não existe (D2).
	c := montar(t, opcoes{agentes: []acpregistry.Agent{}, motivo: "o registro ACP não respondeu no tempo esperado"})

	_, err := c.instalador.Plan(context.Background(), codexID)
	if err == nil {
		t.Fatal("não explicou o catálogo vazio")
	}
	if !strings.Contains(err.Error(), "não respondeu no tempo esperado") {
		t.Errorf("erro = %q, queria o motivo do catálogo vazio", err)
	}
}

func TestInstallRecusaIdentificadorQueSaiDoDiretorioDeDados(t *testing.T) {
	// O `id` vira nome de diretório em `~/.assistente/agents/<id>/` (D5), e ele
	// vem do catálogo, que é dado externo (D9). O índice recusa isso na
	// fronteira; a guarda aqui vale para quem chamar de outro lugar.
	agente := agenteCodex()
	agente.ID = ".." + string(os.PathSeparator) + "fora"
	c := montar(t, opcoes{agentes: []acpregistry.Agent{agente}})

	if _, err := c.instalador.Install(context.Background(), agente.ID); err == nil {
		t.Fatal("aceitou um identificador que sai do diretório de dados")
	}
}
