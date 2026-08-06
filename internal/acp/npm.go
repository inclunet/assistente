package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// NodeRuntime é o Node desta máquina, procurado sem executar nada — a mesma
// disciplina de `detect.go`, e o que o AEP-0086 D7 pede: o app procura e nomeia
// o runtime, e nunca o instala.
//
// Ele carrega o npm junto porque as duas coisas são pedidas juntas: instalar um
// agente distribuído por npm precisa do npm agora e do `node` depois, para o
// spawn do ponto de entrada (D6).
type NodeRuntime struct {
	// Found diz se há `node` para rodar. Falso não é erro: é o estado que a
	// tela explica, com o requisito em texto (D7).
	Found bool

	// Node é o executável do `node`, já conferido como spawnável.
	Node string

	// NPMScript é o `npm-cli.js` que acompanha a instalação do Node, quando ele
	// está no layout conhecido. É o caminho preferido para rodar o npm: no
	// Windows o `npm` do PATH é um arquivo de lote, e o app não cria processo a
	// partir de arquivo de lote (AEP-0084 D15).
	NPMScript string

	// NPM é o executável do npm no PATH, quando ele é spawnável. É a saída para
	// as instalações do Node que não põem o `npm-cli.js` onde se espera.
	NPM string

	// Version é a versão do Node quando o caminho a revela — o diretório
	// versionado do nvm, que é o único layout que a diz sem executar nada.
	// Descobri-la de outro jeito exigiria rodar `node -v`, e a procura de
	// runtime não executa nada (AEP-0086 D7), pelo mesmo princípio que rege
	// `detect.go`.
	Version string

	// Searched são os lugares consultados, na ordem, para a tela poder dizer
	// onde se procurou quando não se achou nada.
	Searched []string

	// Failures são os lugares que não deu para conferir, já com o motivo.
	// Permissão negada não é ausência, e mandar instalar o Node que já está lá
	// não resolveria nada.
	Failures []string
}

// FindNodeRuntime procura o Node e o npm nesta máquina.
func FindNodeRuntime() NodeRuntime {
	return findNodeRuntime(systemProbe())
}

// findNodeRuntime é o FindNodeRuntime com a máquina injetada, para o teste
// descrever um Windows com nvm ou um Unix sem Node nenhum.
func findNodeRuntime(p probe) NodeRuntime {
	searched := &searchLog{}
	node := nodeExecutable(p, searched)
	runtime := NodeRuntime{Searched: searched.paths, Failures: searched.failures}
	if node == "" {
		return runtime
	}
	runtime.Found = true
	runtime.Node = node
	runtime.Version = nodeVersionFromPath(node)
	runtime.NPMScript = npmScriptFor(p, searched, node)
	if found, err := p.lookPath("npm"); err == nil && spawnable(p.goos, found) {
		runtime.NPM = found
	}
	// Os registros crescem durante a procura; o valor só é fechado agora.
	runtime.Searched = searched.paths
	runtime.Failures = searched.failures
	return runtime
}

// nodeExecutable acha o `node` que o app consegue spawnar.
//
// O PATH vem primeiro porque é onde o instalador oficial põe o executável nos
// três sistemas, e é o mesmo `node` que a pessoa vê no terminal dela. Os
// prefixos conhecidos existem porque o app herda o ambiente de quem o abriu, e
// não o do shell de login: aberto pelo lançador do sistema, ele não vê o que o
// `.profile` ou o nvm acrescentaram.
func nodeExecutable(p probe, searched *searchLog) string {
	if found, err := p.lookPath("node"); err == nil && spawnable(p.goos, found) {
		return found
	}
	if p.goos == "windows" {
		// Os mesmos prefixos onde o npm global mora: quem instalou pacote
		// global tem o `node.exe` ao lado, e é ele o dono daqueles pacotes.
		for _, prefix := range npmGlobalPrefixes(p, searched) {
			candidate := filepath.Join(prefix, "node.exe")
			if exists(p, searched, candidate) {
				return candidate
			}
		}
		return ""
	}
	for _, candidate := range unixNodePaths(p) {
		searched.add(candidate)
		if exists(p, searched, candidate) {
			return candidate
		}
	}
	return ""
}

// unixNodePaths são os diretórios de binário que não estão no PATH de um app
// aberto pelo lançador do sistema. É a mesma lacuna que `cursorInLocalBin`
// cobre, e pelo mesmo motivo.
func unixNodePaths(p probe) []string {
	paths := []string{
		filepath.Join("/usr", "local", "bin", "node"),
		filepath.Join("/opt", "homebrew", "bin", "node"),
	}
	if home := strings.TrimSpace(p.getenv("HOME")); home != "" {
		paths = append(paths, filepath.Join(home, ".local", "bin", "node"))
	}
	return paths
}

// nodeVersionFromPath lê a versão no nome do diretório versionado, que é o
// layout do nvm — `%APPDATA%\nvm\v24.4.1\node.exe` e
// `~/.nvm/versions/node/v24.4.1/bin/node`. É o mesmo recurso que a detecção do
// Cursor usa para saber a versão sem executar o agente.
//
// Vazio é resposta legítima: o instalador oficial não versiona o diretório, e a
// tela então mostra o caminho em vez da versão.
func nodeVersionFromPath(node string) string {
	dir := filepath.Dir(node)
	for i := 0; i < 2 && dir != ""; i++ {
		if match := nvmVersionPattern.FindStringSubmatch(filepath.Base(dir)); match != nil {
			return strings.Join(match[1:], ".")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// npmScriptFor acha o `npm-cli.js` da instalação do Node que respondeu.
//
// São dois layouts, e os dois são do instalador, não nossos: no Windows o
// `node_modules` fica ao lado do `node.exe`; em Unix o `node` fica em `bin/` e
// os pacotes em `lib/node_modules/` do mesmo prefixo.
func npmScriptFor(p probe, searched *searchLog, node string) string {
	dir := filepath.Dir(node)
	candidates := []string{
		filepath.Join(dir, "node_modules", "npm", "bin", "npm-cli.js"),
		filepath.Join(filepath.Dir(dir), "lib", "node_modules", "npm", "bin", "npm-cli.js"),
	}
	for _, candidate := range candidates {
		searched.add(candidate)
		if exists(p, searched, candidate) {
			return candidate
		}
	}
	return ""
}

// NPMCommand devolve o par comando/argumentos iniciais para rodar o npm de um
// jeito que o app consegue spawnar e encerrar.
//
// O `node npm-cli.js` vem primeiro de propósito: no Windows o `npm` do PATH é
// `npm.cmd`, e passar por um intérprete de lote deixaria o npm como processo
// neto — cancelar a instalação não encerraria quem está escrevendo no disco.
func (r NodeRuntime) NPMCommand() (string, []string, bool) {
	if !r.Found {
		return "", nil, false
	}
	if r.NPMScript != "" {
		return r.Node, []string{r.NPMScript}, true
	}
	if r.NPM != "" {
		return r.NPM, nil, true
	}
	return "", nil, false
}

// NPMPackage é o que o manifesto de um pacote npm instalado diz sobre como
// rodá-lo.
type NPMPackage struct {
	// Dir é o diretório do pacote dentro do `node_modules` do prefixo.
	Dir string

	// EntryPoint é o arquivo que o `bin` do manifesto aponta, já absoluto e já
	// conferido como estando dentro do prefixo.
	EntryPoint string

	// Version é a versão que o manifesto declara, saneada porque é texto de
	// terceiro que chega à tela (AEP-0084 D11). Vazia quando o manifesto não a
	// declara — o que não invalida nada: o ponto de entrada é o que importa.
	Version string
}

// ErrNPMEntryPoint diz que não deu para saber o que rodar de um pacote npm
// instalado. Quem recebe este erro não tem instalação: um provider salvo com
// comando adivinhado falharia na primeira conversa, longe de quem poderia
// consertá-lo (D8).
var ErrNPMEntryPoint = errors.New("ponto de entrada do pacote npm não resolvido")

// maxManifestBytes é o teto de leitura do `package.json` do pacote. Manifesto
// honesto tem alguns kilobytes; o teto existe para um pacote hostil não virar
// memória gasta antes de qualquer validação.
const maxManifestBytes = 1 << 20

// NPMEntryPoint resolve o que executar de um pacote instalado em `prefix` pelo
// `npm install --prefix` (AEP-0086 D6).
//
// O ponto de entrada sai do `bin` do manifesto, e não de um caminho adivinhado:
// o `dist/index.js` que a detecção do adaptador do Claude Code conhece é
// conhecimento escrito à mão sobre um pacote específico, e ler o manifesto vale
// para os 21 pacotes do catálogo e para os que entrarem depois.
//
// O caminho resolvido é obrigatoriamente dentro de `prefix` (D9): um `bin`
// apontando para fora é o pacote pedindo que o app execute outra coisa.
func NPMEntryPoint(prefix, name string) (NPMPackage, error) {
	root, err := filepath.Abs(prefix)
	if err != nil {
		return NPMPackage{}, fmt.Errorf("%w: prefixo %q ilegível: %v", ErrNPMEntryPoint, prefix, err)
	}
	dir := filepath.Join(root, "node_modules", filepath.FromSlash(name))
	if !withinDir(root, dir) {
		// Nome de pacote com `..` não chega aqui pelo catálogo, que o recusa na
		// fronteira; a guarda vale para quem chamar de outro lugar.
		return NPMPackage{}, fmt.Errorf("%w: o nome %q sai do prefixo da instalação", ErrNPMEntryPoint, name)
	}

	manifestPath := filepath.Join(dir, "package.json")
	data, err := readFileAtMost(manifestPath, maxManifestBytes)
	if err != nil {
		return NPMPackage{}, fmt.Errorf("%w: não foi possível ler %s: %v", ErrNPMEntryPoint, manifestPath, err)
	}
	var manifest struct {
		Name    string          `json:"name"`
		Version string          `json:"version"`
		Bin     json.RawMessage `json:"bin"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return NPMPackage{}, fmt.Errorf("%w: %s não é um manifesto legível: %v", ErrNPMEntryPoint, manifestPath, err)
	}

	relative, err := binFromManifest(manifest.Bin, name)
	if err != nil {
		return NPMPackage{}, fmt.Errorf("%w: %s: %v", ErrNPMEntryPoint, manifestPath, err)
	}
	entry := filepath.Join(dir, filepath.FromSlash(relative))
	if !withinDir(root, entry) {
		return NPMPackage{}, fmt.Errorf("%w: o `bin` de %s aponta para fora da instalação", ErrNPMEntryPoint, manifestPath)
	}
	// O caminho textual estar dentro do prefixo não basta: um link dentro da
	// instalação pode levar para fora dela, e quem escolhe o destino do link é o
	// mesmo pacote que declarou o `bin`. A guarda do D9 vale sobre o destino
	// real; o que se executa continua sendo o caminho legível.
	real, err := filepath.EvalSymlinks(entry)
	if err != nil {
		return NPMPackage{}, fmt.Errorf("%w: o `bin` de %s aponta para %s, que não é um arquivo", ErrNPMEntryPoint, manifestPath, entry)
	}
	// O prefixo também é resolvido antes da comparação: em macOS `/var` é link
	// para `/private/var`, e comparar um lado resolvido com o outro cru recusaria
	// instalação legítima.
	realRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		realRoot = resolved
	}
	if !withinDir(realRoot, real) {
		return NPMPackage{}, fmt.Errorf("%w: o `bin` de %s aponta para fora da instalação", ErrNPMEntryPoint, manifestPath)
	}
	if ok, err := isRegularFile(real); err != nil || !ok {
		return NPMPackage{}, fmt.Errorf("%w: o `bin` de %s aponta para %s, que não é um arquivo", ErrNPMEntryPoint, manifestPath, entry)
	}

	return NPMPackage{Dir: dir, EntryPoint: entry, Version: SanitizeLabel(manifest.Version)}, nil
}

// binFromManifest lê o `bin` do manifesto, que o npm aceita como texto ou como
// mapa de nome para caminho.
//
// Com mais de uma entrada, a que leva o nome do pacote ganha: é a convenção do
// npm para o executável principal, e é o que o `npx <pacote>` roda. Sem ela, a
// escolha é a primeira em ordem alfabética — arbitrária, mas estável: escolher
// pela ordem do JSON daria comandos diferentes para o mesmo pacote conforme
// quem serializou o manifesto.
func binFromManifest(raw json.RawMessage, name string) (string, error) {
	if len(raw) == 0 {
		return "", errors.New("o manifesto não declara `bin`, e é dele que sai o ponto de entrada")
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if strings.TrimSpace(single) == "" {
			return "", errors.New("o `bin` do manifesto está vazio")
		}
		return single, nil
	}

	var entries map[string]string
	if err := json.Unmarshal(raw, &entries); err != nil {
		return "", errors.New("o `bin` do manifesto não é texto nem mapa")
	}
	names := make([]string, 0, len(entries))
	for key, value := range entries {
		if strings.TrimSpace(value) != "" {
			names = append(names, key)
		}
	}
	if len(names) == 0 {
		return "", errors.New("o `bin` do manifesto não tem entrada aproveitável")
	}
	preferred := name
	if i := strings.LastIndex(name, "/"); i >= 0 {
		preferred = name[i+1:]
	}
	if _, ok := entries[preferred]; ok {
		return entries[preferred], nil
	}
	slices.Sort(names)
	return entries[names[0]], nil
}

// withinDir diz se path está dentro de root. É a guarda de caminho do D9, e ela
// compara texto já limpo em vez de confiar em quem montou o caminho.
func withinDir(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if path == root {
		return true
	}
	return strings.HasPrefix(path, root+string(os.PathSeparator))
}

// readFileAtMost lê o arquivo recusando o que passar do teto. O byte de folga
// do LimitReader é o que permite saber que passou sem ter lido o excesso.
func readFileAtMost(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("o arquivo passou do limite de %d bytes", limit)
	}
	return data, nil
}
