package acpinstall

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/acpregistry"
	"assistente/internal/logging"
)

// scriptExtensions são os pontos de entrada que só sobem por um interpretador.
// Um `.js` extraído de um artefato não é executável: quem o executa é o Node, e
// é o par `node` + arquivo que vai para o `installed.json` (D8).
var scriptExtensions = []string{".js", ".mjs", ".cjs"}

// binaryTarget escolhe o alvo desta plataforma no item do catálogo.
//
// A cobertura não é uniforme: só 7 dos 17 agentes com binário publicam os seis
// alvos, e `windows-aarch64` é o mais raro deles. Não achar alvo é estado a
// explicar, e não defeito — daí o erro nomear a plataforma que se procurou.
func binaryTarget(agent acpregistry.Agent) (acpregistry.BinaryTarget, string, error) {
	if len(agent.Distribution.Binary) == 0 {
		return acpregistry.BinaryTarget{}, "", failf(StepCatalog, "%w: %s", ErrNotBinary, agent.ID)
	}
	platform := PlatformTarget()
	if platform == "" {
		return acpregistry.BinaryTarget{}, "", failf(StepCatalog,
			"%w: %s/%s", ErrNoPlatformTarget, runtime.GOOS, runtime.GOARCH)
	}
	target, ok := agent.Distribution.Binary[platform]
	if !ok {
		return acpregistry.BinaryTarget{}, "", failf(StepCatalog, "%w: %s", ErrNoPlatformTarget, platform)
	}
	return target, platform, nil
}

// installBinary baixa o alvo, confere, abre e resolve o que executar.
//
// A conferência do digest acontece dentro do download, sobre o que está
// chegando. Esta fase só instala o que publica digest: sem ele não há o que
// conferir, e a confirmação reforçada que abre esse caminho é assunto da fase
// seguinte (D4).
func (i *Installer) installBinary(
	ctx context.Context,
	agent acpregistry.Agent,
	target acpregistry.BinaryTarget,
	platform, version, dir string,
) (Installation, error) {
	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageStarted})

	if err := prepareDir(dir); err != nil {
		return Installation{}, failf(StepPrepare, "não foi possível preparar %s: %w", dir, err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageInstalling})
	art, err := fetchArtifact(ctx, i.http, target.Archive, target.SHA256, dir)
	if err != nil {
		return Installation{}, failf(StepDownload, "%w", err)
	}
	if err := extractArtifact(ctx, art, dir, rawBinaryName(target.Cmd)); err != nil {
		return Installation{}, failf(StepExtract, "%w", err)
	}

	i.emit(ctx, Progress{AgentID: agent.ID, Agent: agent.Name, Stage: StageVerifying})
	command, args, err := resolveBinaryCommand(dir, target, i.runtime())
	if err != nil {
		return Installation{}, err
	}
	if err := i.handshake(ctx, command, args); err != nil {
		return Installation{}, failf(StepVerify, "o agente instalado não respondeu ao handshake do protocolo: %w", err)
	}

	installation := Installation{
		Schema:       installationSchema,
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionBinary,
		Target:       platform,
		SHA256: art.SHA256,
		// O digest é sempre gravado; o que muda é o que ele vale. Conferido, ele
		// liga o arquivo ao que o registro curou. Observado, ele não atesta
		// procedência nenhuma — é só o que chegou —, e é por isso que a origem
		// vai junto: sem ela, a próxima leitura não teria como distinguir os
		// dois, e a tela diria "verificado" sobre o que ninguém verificou (D4).
		SHA256Origin: digestOrigin(target.SHA256),
		Command:      command,
		Args:         args,
		InstalledAt:  i.now().UTC(),
		Dir:          dir,
	}
	if err := writeInstallation(dir, installation); err != nil {
		return Installation{}, failf(StepRecord, "%w", err)
	}
	logging.Infof(ctx, component, "agente %s instalado em %s a partir do alvo %s", agent.ID, dir, platform)
	return installation, nil
}

// digestOrigin diz o que o digest gravado vale, pelo que o registro publicou.
func digestOrigin(published string) string {
	if published == "" {
		return DigestObserved
	}
	return DigestVerified
}

// rawBinaryName é o nome que o executável recebe quando o artefato vem sem
// embalagem. Ele sai do `cmd` do registro porque é por ele que o agente vai ser
// procurado logo em seguida.
func rawBinaryName(cmd string) string {
	return entryPath(cmd)
}

// entryPath normaliza o `cmd` do registro em caminho relativo.
//
// O campo mistura as duas convenções — o Cursor publica
// `./dist-package\cursor-agent.cmd` —, e a contrabarra ali é separador de
// diretório, não parte do nome. É o oposto do que vale para o nome de uma
// entrada de archive, onde a contrabarra é recusada justamente por não haver
// como saber: aqui o formato do registro diz o que ela é.
func entryPath(cmd string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(cmd, `\`, "/"))
	clean = strings.TrimPrefix(clean, "./")
	return clean
}

// resolveBinaryCommand decide o que executar depois de o artefato estar aberto,
// na ordem do D8.
//
// O registro resolveu o download, a versão e a plataforma, que é a parte
// perecível. O que ele não resolve é o que este app consegue executar: o `.cmd`
// do Cursor e o `./opencode` sem extensão do Windows ARM são `cmd` publicados
// que não sobem como processo aqui.
func resolveBinaryCommand(dir string, target acpregistry.BinaryTarget, node acp.NodeRuntime) (string, []string, error) {
	rel := entryPath(target.Cmd)
	candidate, err := resolveFileEntry(dir, rel)
	if err != nil {
		// O `cmd` é do índice, que é dado externo: um que aponte para fora do
		// diretório da instalação é recusado antes de virar linha de comando
		// (D9). O `.` cai aqui junto: como comando ele é o próprio diretório, e
		// a alternativa do Windows sobre ele seria `<dir>.exe` — irmão da
		// instalação, não parte dela.
		return "", nil, failf(StepResolve, "%w: %s", ErrCommandNotResolved, acp.SanitizeLabel(target.Cmd))
	}
	args := slices.Clone(target.Args)
	tried := []string{candidate}

	// 1. O par `node` + ponto de entrada, quando o que o registro publicou é um
	// script. A heurística é a mesma da instalação por npm, aplicada aqui a um
	// arquivo do artefato. Ela vem antes do comando publicado porque em POSIX o
	// sistema aceitaria executar o `.js` direto — o que só funciona se ele
	// tiver shebang, e deixaria o desfecho dependendo do sistema em que o app
	// está rodando.
	if isRegularFile(candidate) && isScript(candidate) {
		if !node.Found {
			return "", nil, failf(StepResolve,
				"%w: %s precisa do %s, que não foi encontrado nesta máquina",
				ErrCommandNotResolved, shownPath(filepath.Base(candidate)), RuntimeNode)
		}
		return node.Node, append([]string{candidate}, args...), nil
	}

	// 2. O que o registro mandou executar, quando ele sobe como processo.
	if runnable(candidate) && acp.Spawnable(candidate) {
		return candidate, args, nil
	}

	// 3. No Windows, o executável de verdade ao lado do nome publicado. O
	// `opencode` do alvo ARM manda `./opencode` e entrega `opencode.exe`; o
	// Cursor manda um `.cmd` que embrulha um `.exe` de mesmo nome.
	if runtime.GOOS == "windows" {
		for _, alternative := range windowsAlternatives(candidate) {
			tried = append(tried, alternative)
			if runnable(alternative) && acp.Spawnable(alternative) {
				return alternative, args, nil
			}
		}
	}

	// 4. Falha dizendo o que foi tentado: sem isso, "não consegui resolver o
	// comando" não é verificável por quem abriu o diretório para olhar.
	return "", nil, failf(StepResolve, "%w: procurei por %s", ErrCommandNotResolved, describePaths(tried))
}

// windowsAlternatives são os nomes que podem ser o executável de verdade.
func windowsAlternatives(candidate string) []string {
	out := []string{candidate + ".exe"}
	if ext := filepath.Ext(candidate); ext != "" {
		out = append(out, strings.TrimSuffix(candidate, ext)+".exe")
	}
	return out
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// runnable diz se este arquivo pode ser o comando do provider.
//
// Fora do Windows não basta existir: sem o bit de execução o processo não sobe,
// e aceitar o arquivo aqui empurraria a falha para o handshake — longe do lugar
// onde ela é explicável. Quando o bit falta, o app o liga: quem publicou o
// archive é que esqueceu de marcá-lo (zip montado no Windows não guarda modo
// POSIX), e o arquivo é o que o registro nomeou como comando, dentro de um
// diretório que este app acabou de criar. Chmod que não passa não vira erro:
// o próximo ramo tenta, e a falha final diz o que foi procurado.
func runnable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" || info.Mode().Perm()&0o111 != 0 {
		return true
	}
	return os.Chmod(path, info.Mode().Perm()|0o111) == nil
}

func isScript(path string) bool {
	return slices.Contains(scriptExtensions, strings.ToLower(filepath.Ext(path)))
}

// binaryPlan é o item do catálogo que se instala baixando um artefato.
//
// O que ele não tem, e o plano por npm tem, é uma linha de comando a mostrar:
// aqui não se executa nada para instalar, e o que responde "o que vai
// acontecer" é a URL de onde o arquivo vem, junto do digest que será conferido
// contra ele (D3).
func (i *Installer) binaryPlan(agent acpregistry.Agent, version string) Plan {
	plan := Plan{
		AgentID:      agent.ID,
		Name:         agent.Name,
		Version:      version,
		Distribution: DistributionBinary,
		// O Node não é pré-requisito de artefato binário. Ele volta a ser se a
		// resolução do comando cair no ponto de entrada de script, e é lá que a
		// falta dele é dita — não aqui, onde ainda não se sabe o que o artefato
		// traz dentro.
		Runtime: RuntimeStatus{Name: RuntimeNode},
	}

	target, platform, err := binaryTarget(agent)
	if err != nil {
		plan.Reason = acp.SanitizeLabel(err.Error())
		return plan
	}
	plan.Target = platform
	plan.Origin = target.Archive
	plan.SHA256 = target.SHA256
	plan.Unverified = target.SHA256 == ""
	plan.RunArgs = slices.Clone(target.Args)
	plan.Dir = i.agentVersionDir(agent.ID, version)

	if installed, ok := i.installationAt(plan.Dir); ok {
		plan.Installed = &installed
	}

	switch {
	case version == "":
		// A versão vira nome de diretório e é o que separa a instalação nova da
		// que está em uso (D5, D10). Sem ela a instalação não teria onde morar,
		// e oferecer o botão levaria a uma recusa depois de aceito.
		plan.Reason = ErrUnpinnedVersion.Error()
	case i.root == "" || plan.Dir == "":
		plan.Reason = "não foi possível descobrir o diretório de dados do app para instalar o agente"
	case plan.Installed != nil:
		plan.Reason = ErrAlreadyInstalled.Error()
	default:
		if _, err := formatOf(target.Archive); err != nil {
			plan.Reason = acp.SanitizeLabel(err.Error())
			break
		}
		plan.CanInstall = true
	}
	return plan
}

// binaryDistribution diz se o agente publica artefato binário.
func binaryDistribution(agent acpregistry.Agent) bool {
	return len(agent.Distribution.Binary) > 0
}

// distributionFor decide por qual caminho este agente é instalado nesta
// máquina. Plano e instalação chamam a mesma função: a tela que mostra "vai
// baixar tal arquivo" e o código que baixa não podem discordar.
//
// O pacote npm vem primeiro quando os dois existem. Ele baixa menos, atualiza
// incrementalmente e tem integridade conferida pelo próprio npm contra o
// `integrity` do registro dele — enquanto o artefato depende de o autor ter
// publicado o `sha256`, e metade deles não publica.
//
// A exceção é a máquina onde o npm não está disponível — sem Node, ou com um
// Node cujo `npm-cli.js` não está ao lado dele. Ali o caminho npm não existe, e
// recusar a instalação por falta de um runtime que o artefato não usa seria
// mandar instalar o Node para não usá-lo.
func (i *Installer) distributionFor(agent acpregistry.Agent, node acp.NodeRuntime) string {
	binary := binaryDistribution(agent)
	if agent.Distribution.NPX == nil {
		if binary {
			return DistributionBinary
		}
		return ""
	}
	if _, _, npm := node.NPMCommand(); binary && !npm && binaryInstallable(agent) {
		return DistributionBinary
	}
	return DistributionNPM
}

// binaryInstallable diz se o artefato desta plataforma é instalável agora: alvo
// publicado e um formato que o app sabe abrir.
//
// A falta de digest não entra aqui. Ela decide o quanto o app afirma sobre o
// que instalou, e não se ele instala (D4): o que ela acrescenta é uma pergunta,
// feita antes de baixar.
func binaryInstallable(agent acpregistry.Agent) bool {
	target, _, err := binaryTarget(agent)
	if err != nil {
		return false
	}
	_, err = formatOf(target.Archive)
	return err == nil
}

// installFromBinary confere as recusas e instala baixando o artefato.
//
// As recusas são conferidas aqui, e não pelo plano: o plano existe para a tela e
// devolve o motivo em texto, e um texto não diz a quem chamou *qual* recusa
// aconteceu. Repetir as perguntas é o que faz `errors.Is` valer para quem
// programa contra este pacote.
func (i *Installer) installFromBinary(ctx context.Context, agent acpregistry.Agent, confirmed Confirmed) (Installation, error) {
	target, platform, err := binaryTarget(agent)
	if err != nil {
		return Installation{}, err
	}
	// O que a tela mostrou tem de ser o que vai ser baixado: o índice é
	// revalidado sozinho, e a mesma linha do catálogo pode passar a apontar
	// outro arquivo entre a confirmação e o clique (D3).
	if err := confirmed.check(Confirmed{
		Distribution: DistributionBinary,
		Origin:       target.Archive,
		SHA256:       target.SHA256,
	}); err != nil {
		return Installation{}, err
	}
	// A pergunta do artefato sem digest é refeita aqui, e não só na tela: o
	// consentimento é o que separa instalar de baixar sozinho, e uma regra que
	// mora só na interface deixa de valer no primeiro chamador novo (D4).
	if target.SHA256 == "" && !confirmed.AcceptUnverified {
		return Installation{}, failf(StepCatalog, "%w: %s", ErrUnverifiedNotAccepted, acp.SanitizeLabel(agent.ID))
	}
	if _, err := formatOf(target.Archive); err != nil {
		return Installation{}, failf(StepCatalog, "%w", err)
	}
	version := sanitizeVersion(agent.Version)
	if version == "" {
		return Installation{}, failf(StepCatalog, "%w: %s", ErrUnpinnedVersion, acp.SanitizeLabel(agent.ID))
	}
	dir := i.agentVersionDir(agent.ID, version)
	if dir == "" {
		return Installation{}, failf(StepPrepare,
			"não foi possível montar o diretório de instalação do agente %s versão %s",
			acp.SanitizeLabel(agent.ID), acp.SanitizeLabel(version))
	}
	if existing, ok := i.installationAt(dir); ok {
		return Installation{}, failf(StepPrepare, "%w: %s %s", ErrAlreadyInstalled, agent.Name, existing.Version)
	}
	return i.run(ctx, agent, dir, func(ctx context.Context) (Installation, error) {
		return i.installBinary(ctx, agent, target, platform, version, dir)
	})
}
