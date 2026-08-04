package acp

import (
	"cmp"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// AgentKind identifica o agente de código procurado na máquina. Hoje só o
// Cursor tem detecção; o Claude Code entra pelo mesmo caminho quando chegar a
// vez dele (AEP-0084 Fase 7).
type AgentKind string

// AgentKindCursor é o CLI do Cursor em modo ACP (`agent acp`).
const AgentKindCursor AgentKind = "cursor"

// acpSubcommand é o argumento que põe o CLI em modo ACP.
const acpSubcommand = "acp"

// Install é o que a procura encontrou. Não achar não é erro: é um estado que a
// tela precisa explicar, junto do que fazer para resolver (AEP-0084 Fase 3).
// Por isso Searched vem preenchido nos dois casos — "não encontrado" sem dizer
// onde se olhou não ajuda quem vai corrigir.
type Install struct {
	// Found diz se há o que subir.
	Found bool

	// Command e Args sobem o agente em modo ACP. São exatamente o que vai para
	// ACPCommand e ACPArgs do provider (AEP-0084 D15): comando e argumentos, e
	// não um caminho mágico que o app reinterpreta depois.
	Command string
	Args    []string

	// Version é a versão instalada quando o layout a revela — no Windows, o
	// nome do diretório versionado. Pelo PATH não há como saber sem executar o
	// agente, e a detecção não executa nada.
	Version string

	// Source é o arquivo que decidiu a detecção, para a tela poder dizer de
	// onde veio o comando que ela sugeriu.
	Source string

	// Searched são os diretórios e arquivos consultados, na ordem. Só o que é
	// específico da máquina entra aqui: a procura no PATH acontece sempre e é a
	// mensagem da tela que a menciona, no idioma de quem lê.
	Searched []string
}

// DetectAgent procura o agente de código nesta máquina.
func DetectAgent(kind AgentKind) (Install, error) {
	switch kind {
	case AgentKindCursor:
		return detectCursor(systemProbe()), nil
	default:
		// O nome vem da chamada da UI e pode chegar de qualquer lugar: sai
		// citado e achatado, como todo texto de fora (AEP-0084 D11).
		return Install{}, fmt.Errorf("agente de código desconhecido: %q", singleLine(string(kind)))
	}
}

// probe é tudo o que a detecção pergunta ao sistema. Existe para o teste poder
// descrever uma máquina — Windows com Cursor instalado, Unix sem nada — em vez
// de depender de como está a máquina que roda o teste.
type probe struct {
	goos     string
	getenv   func(string) string
	lookPath func(string) (string, error)
	isFile   func(string) bool
	readDir  func(string) ([]fs.DirEntry, error)
}

func systemProbe() probe {
	return probe{
		goos:     runtime.GOOS,
		getenv:   os.Getenv,
		lookPath: exec.LookPath,
		isFile:   isRegularFile,
		readDir:  os.ReadDir,
	}
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// detectCursor procura o CLI do Cursor. A ordem não é arbitrária: no Windows o
// par `node.exe index.js` da versão instalada vem antes do wrapper porque é o
// único candidato que o app consegue matar sozinho. Spawnar o wrapper deixaria
// um `node.exe` filho segurando os canos do protocolo, e derrubar o pai não
// encerraria o agente — que é um processo que edita arquivos (AEP-0084, risco
// de processo órfão no Windows).
func detectCursor(p probe) Install {
	searched := &searchLog{}
	candidates := []func(probe, *searchLog) (Install, bool){}
	if p.goos == "windows" {
		candidates = append(candidates,
			cursorVersionedNode,
			cursorScriptWrapper,
		)
	}
	candidates = append(candidates, cursorOnPath, cursorInLocalBin)

	for _, candidate := range candidates {
		if install, ok := candidate(p, searched); ok {
			install.Found = true
			install.Searched = searched.paths
			return install
		}
	}
	return Install{Searched: searched.paths}
}

// searchLog guarda onde se procurou, sem repetir: um mesmo diretório é
// consultado por mais de um candidato, e listá-lo duas vezes na tela faria
// parecer que a busca se perdeu.
type searchLog struct {
	paths []string
}

func (s *searchLog) add(path string) {
	if path == "" || slices.Contains(s.paths, path) {
		return
	}
	s.paths = append(s.paths, path)
}

// cursorHome é o diretório onde o instalador do Cursor põe o CLI no Windows.
func cursorHome(p probe) string {
	base := strings.TrimSpace(p.getenv("LOCALAPPDATA"))
	if base == "" {
		return ""
	}
	return filepath.Join(base, "cursor-agent")
}

// cursorVersionedNode acha o par `node.exe` + `index.js` que o wrapper do
// Cursor executaria. Cobre os dois layouts que o próprio wrapper conhece: o
// diretório versionado mais recente e o caso em que os arquivos estão soltos na
// raiz da instalação.
func cursorVersionedNode(p probe, searched *searchLog) (Install, bool) {
	home := cursorHome(p)
	if home == "" {
		return Install{}, false
	}
	searched.add(home)

	if install, ok := nodePairIn(p, home, ""); ok {
		return install, true
	}

	versions := filepath.Join(home, "versions")
	searched.add(versions)
	entries, err := p.readDir(versions)
	if err != nil {
		return Install{}, false
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && cursorVersionPattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	// Da mais recente para a mais antiga, como o wrapper faz: a instalação
	// guarda versões antigas, e subir uma delas seria falar com um agente que a
	// pessoa já atualizou.
	slices.SortFunc(names, func(a, b string) int { return cmp.Compare(cursorVersionOrder(b), cursorVersionOrder(a)) })
	for _, name := range names {
		if install, ok := nodePairIn(p, filepath.Join(versions, name), name); ok {
			return install, true
		}
	}
	return Install{}, false
}

// nodePairIn devolve o comando para o par node/index de um diretório, quando os
// dois arquivos estão lá.
func nodePairIn(p probe, dir, version string) (Install, bool) {
	node := filepath.Join(dir, "node.exe")
	index := filepath.Join(dir, "index.js")
	if !p.isFile(node) || !p.isFile(index) {
		return Install{}, false
	}
	return Install{
		Command: node,
		Args:    []string{index, acpSubcommand},
		Version: version,
		Source:  index,
	}, true
}

// cursorVersionPattern aceita os dois formatos de nome de versão que o wrapper
// do Cursor reconhece: `AAAA.MM.DD-commit` e o mais novo
// `AAAA.MM.DD-hh-mm-ss-commit`.
var cursorVersionPattern = regexp.MustCompile(`^(\d{4})\.(\d{1,2})\.(\d{1,2})(?:-(\d{2})-(\d{2})-(\d{2}))?-[0-9a-f]+$`)

// cursorVersionOrder transforma o nome da versão em um número comparável. O
// carimbo de hora entra na conta porque, com ele, duas versões do mesmo dia
// deixam de empatar — e empate escolheria pela ordem do diretório, que não diz
// nada sobre qual é a mais nova.
//
// O resultado é int64 porque a concatenação de ano, mês, dia, hora, minuto e
// segundo passa de duas casas de bilhão, e `int` tem 32 bits nas arquiteturas
// de 32 bits: lá a conta viraria negativa e a ordenação escolheria a versão
// errada.
func cursorVersionOrder(name string) int64 {
	match := cursorVersionPattern.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	var order int64
	for _, group := range match[1:] {
		value := 0
		if group != "" {
			value, _ = strconv.Atoi(group)
		}
		order = order*100 + int64(value)
	}
	return order
}

// cursorScriptWrapper é a saída quando o layout da instalação não é o esperado:
// o wrapper PowerShell que o instalador escreveu sabe se achar sozinho, mesmo
// depois de uma atualização mudar o diretório versionado de nome.
//
// Não existe `agent.exe` para spawnar (AEP-0084, descobertas empíricas), e o
// `agent.cmd` que fica ao lado não serve: o Windows não sabe criar processo a
// partir de um arquivo de lote. Sobra chamar o PowerShell passando o script.
func cursorScriptWrapper(p probe, searched *searchLog) (Install, bool) {
	home := cursorHome(p)
	if home == "" {
		return Install{}, false
	}
	searched.add(home)

	shell, err := p.lookPath("powershell")
	if err != nil {
		// Sem PowerShell não há como executar o script. Dizer que encontrou
		// levaria a pessoa a salvar um provider que nunca sobe.
		return Install{}, false
	}
	for _, name := range []string{"agent.ps1", "cursor-agent.ps1"} {
		script := filepath.Join(home, name)
		if !p.isFile(script) {
			continue
		}
		return Install{
			Command: shell,
			// -NoProfile porque o perfil da pessoa não tem nada a ver com o
			// agente e pode escrever na saída, que aqui é o canal do protocolo.
			// -ExecutionPolicy Bypass porque o script é do instalador do Cursor
			// e uma política restritiva da máquina o bloquearia; o escopo é
			// esta chamada, e não a máquina.
			Args:   []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, acpSubcommand},
			Source: script,
		}, true
	}
	return Install{}, false
}

// cursorAgentNames são os nomes com que o CLI aparece no PATH.
var cursorAgentNames = []string{"cursor-agent", "agent"}

// cursorOnPath procura o executável no PATH, que é o caminho normal em Linux e
// macOS.
func cursorOnPath(p probe, _ *searchLog) (Install, bool) {
	for _, name := range cursorAgentNames {
		found, err := p.lookPath(name)
		if err != nil {
			continue
		}
		if !spawnable(p.goos, found) {
			// No Windows o PATH costuma resolver para `agent.cmd`, que não é
			// um executável: aceitá-lo daria um provider que falha no spawn.
			continue
		}
		return Install{Command: found, Args: []string{acpSubcommand}, Source: found}, true
	}
	return Install{}, false
}

// cursorInLocalBin cobre a instalação em Linux e macOS quando o diretório do
// instalador não está no PATH do processo do app — que herda o ambiente de quem
// abriu o app, e não o do shell de login.
func cursorInLocalBin(p probe, searched *searchLog) (Install, bool) {
	if p.goos == "windows" {
		return Install{}, false
	}
	home := strings.TrimSpace(p.getenv("HOME"))
	if home == "" {
		return Install{}, false
	}
	for _, name := range cursorAgentNames {
		candidate := filepath.Join(home, ".local", "bin", name)
		searched.add(candidate)
		if p.isFile(candidate) {
			return Install{Command: candidate, Args: []string{acpSubcommand}, Source: candidate}, true
		}
	}
	return Install{}, false
}

// spawnable diz se o app consegue criar processo a partir deste arquivo. Fora
// do Windows a pergunta não existe; nele, script de lote e script do PowerShell
// precisam de um intérprete e não sobem sozinhos.
func spawnable(goos, path string) bool {
	if goos != "windows" {
		return true
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".exe", ".com":
		return true
	default:
		return false
	}
}
