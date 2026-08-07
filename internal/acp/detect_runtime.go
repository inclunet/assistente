package acp

import (
	"cmp"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// Runtime é um interpretador de terceiro que alguns agentes do catálogo exigem
// para existir na máquina. O app procura e nomeia o que falta, e nunca instala
// (AEP-0086 D7): gerenciar runtime alheio quebra o que já estava lá.
type Runtime string

const (
	// RuntimeNode é o Node.js, exigido pelos agentes distribuídos por npm.
	RuntimeNode Runtime = "node"

	// RuntimeUV é o `uv`, exigido pelos agentes distribuídos por `uvx`.
	RuntimeUV Runtime = "uv"
)

// RuntimeInstall é o que a procura pelo runtime encontrou. Não achar não é
// erro: é o estado que a tela explica, com o nome do que falta (D7).
type RuntimeInstall struct {
	// Found diz se há runtime nesta máquina.
	Found bool

	// Path é o executável encontrado. Ele fica visível porque é a prova de qual
	// instalação atende — quem tem duas versões de Node precisa saber qual o
	// app achou antes de investigar por que o pacote não instala.
	Path string

	// Searched são os lugares específicos desta máquina que foram consultados,
	// na ordem. A procura no PATH acontece sempre e é a tela que a menciona, no
	// idioma de quem lê — a mesma divisão do Install.
	Searched []string

	// Failures são os lugares que não deu para conferir, já com o motivo. Vale
	// o mesmo do Install: ausência não entra, porque não existir é a resposta
	// esperada de quem não instalou o runtime.
	Failures []string
}

// DetectRuntime procura o runtime nesta máquina.
//
// Ela não executa nada, pelo mesmo princípio que rege a detecção de agente:
// perguntar a versão ao próprio programa custaria criar processo a cada abertura
// da tela do catálogo, e um runtime quebrado penduraria a tela em vez de
// aparecer como ausente.
func DetectRuntime(rt Runtime) (RuntimeInstall, error) {
	return detectRuntime(rt, systemProbe())
}

// detectRuntime é o DetectRuntime com a máquina injetada, para o teste poder
// descrever um Windows com nvm e um Linux sem Node nenhum.
func detectRuntime(rt Runtime, p probe) (RuntimeInstall, error) {
	var candidates []func(probe, *searchLog) (string, bool)
	switch rt {
	case RuntimeNode:
		candidates = []func(probe, *searchLog) (string, bool){nodeOnPath, nodeInKnownPrefixes}
	case RuntimeUV:
		candidates = []func(probe, *searchLog) (string, bool){uvOnPath, uvInKnownDirs}
	default:
		// O nome pode chegar de qualquer lugar, como o do agente: sai citado e
		// achatado (AEP-0084 D11).
		return RuntimeInstall{}, fmt.Errorf("runtime desconhecido: %q", singleLine(string(rt)))
	}

	searched := &searchLog{}
	for _, candidate := range candidates {
		if path, ok := candidate(p, searched); ok {
			return RuntimeInstall{Found: true, Path: path, Searched: searched.paths}, nil
		}
	}
	install := RuntimeInstall{Searched: searched.paths, Failures: searched.failures}
	if len(install.Failures) > 0 {
		return install, fmt.Errorf("a procura pelo runtime não pôde ser concluída: %s",
			strings.Join(install.Failures, "; "))
	}
	return install, nil
}

// nodeOnPath é o caminho normal em qualquer sistema: o instalador do Node põe o
// executável no PATH.
func nodeOnPath(p probe, _ *searchLog) (string, bool) {
	found, err := p.lookPath("node")
	if err != nil || !spawnable(p.goos, found) {
		return "", false
	}
	return found, true
}

// nodeInKnownPrefixes cobre o app aberto pelo lançador do sistema, que herda o
// ambiente de quem o abriu e não o do shell de login: no Windows o instalador do
// Node e o nvm-windows põem o executável em diretórios que o PATH do processo
// pode não ter, e em Linux e macOS o `.profile` é quem acrescenta o do nvm.
//
// A lista de prefixos do Windows é a mesma que a detecção do adaptador do Claude
// Code já usa (`npmGlobalPrefixes`), e é de propósito: o Node que interessa é o
// dono daqueles pacotes globais.
func nodeInKnownPrefixes(p probe, searched *searchLog) (string, bool) {
	if p.goos == "windows" {
		for _, prefix := range npmGlobalPrefixes(p, searched) {
			candidate := filepath.Join(prefix, "node.exe")
			searched.add(candidate)
			if exists(p, searched, candidate) {
				return candidate, true
			}
		}
		return "", false
	}
	for _, candidate := range unixNodeCandidates(p, searched) {
		searched.add(candidate)
		if exists(p, searched, candidate) {
			return candidate, true
		}
	}
	return "", false
}

// unixNodeCandidates são os lugares onde o Node fica em Linux e macOS quando o
// PATH do processo não o alcança. As versões do nvm entram da mais recente para
// a mais antiga, pelo mesmo motivo do nvm-windows: uma versão antiga que ficou
// no disco não é a que a pessoa usa.
func unixNodeCandidates(p probe, searched *searchLog) []string {
	var candidates []string
	home := strings.TrimSpace(p.getenv("HOME"))
	if home != "" {
		nvm := filepath.Join(home, ".nvm", "versions", "node")
		searched.add(nvm)
		for _, version := range sortedVersionDirs(p, searched, nvm) {
			candidates = append(candidates, filepath.Join(version, "bin", "node"))
		}
		candidates = append(candidates,
			filepath.Join(home, ".volta", "bin", "node"),
			filepath.Join(home, ".local", "bin", "node"),
		)
	}
	// `/opt/homebrew` é o prefixo do Homebrew em macOS ARM, onde `/usr/local`
	// deixou de ser o padrão.
	return append(candidates,
		filepath.Join("/opt", "homebrew", "bin", "node"),
		filepath.Join("/usr", "local", "bin", "node"),
		filepath.Join("/usr", "bin", "node"),
	)
}

// sortedVersionDirs lista os subdiretórios que têm nome de versão, do mais novo
// para o mais antigo. Reaproveita a régua do nvm-windows: o layout do nvm de
// Unix nomeia os diretórios do mesmo jeito, com o `v` na frente.
func sortedVersionDirs(p probe, searched *searchLog, root string) []string {
	entries, err := p.readDir(root)
	if err != nil {
		searched.fail(root, err)
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && nvmVersionPattern.MatchString(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sortVersionNamesDesc(names)
	dirs := make([]string, 0, len(names))
	for _, name := range names {
		dirs = append(dirs, filepath.Join(root, name))
	}
	return dirs
}

// sortVersionNamesDesc ordena nomes de diretório de versão do nvm da mais
// recente para a mais antiga, comparando por número: como texto, a 9 ficaria na
// frente da 22.
func sortVersionNamesDesc(names []string) {
	// Comparação, e não subtração: a diferença de dois inteiros pode estourar, e
	// um comparador que estoura devolve a ordem trocada em vez de um erro.
	slices.SortFunc(names, func(a, b string) int {
		return cmp.Compare(nvmVersionOrder(b), nvmVersionOrder(a))
	})
}

// uvOnPath é o caminho normal: o instalador do `uv` acrescenta o diretório dele
// ao PATH do shell.
func uvOnPath(p probe, _ *searchLog) (string, bool) {
	found, err := p.lookPath("uv")
	if err != nil || !spawnable(p.goos, found) {
		return "", false
	}
	return found, true
}

// uvInKnownDirs cobre o PATH que o app não herdou. O instalador oficial do `uv`
// grava em `~/.local/bin`; quem instalou pelo cargo tem o binário em
// `~/.cargo/bin`.
func uvInKnownDirs(p probe, searched *searchLog) (string, bool) {
	home := runtimeHomeDir(p)
	if home == "" {
		return "", false
	}
	name := "uv"
	if p.goos == "windows" {
		name = "uv.exe"
	}
	for _, dir := range []string{
		filepath.Join(home, ".local", "bin"),
		filepath.Join(home, ".cargo", "bin"),
	} {
		candidate := filepath.Join(dir, name)
		searched.add(candidate)
		if exists(p, searched, candidate) {
			return candidate, true
		}
	}
	return "", false
}

// runtimeHomeDir é o diretório da pessoa, com o nome que cada sistema dá à
// variável. O Windows não define HOME, e usar só ele deixaria a procura por
// diretório de fora justamente onde o PATH do processo é mais capenga.
func runtimeHomeDir(p probe) string {
	if p.goos == "windows" {
		return strings.TrimSpace(p.getenv("USERPROFILE"))
	}
	return strings.TrimSpace(p.getenv("HOME"))
}
