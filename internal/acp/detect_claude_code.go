package acp

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// claudeAdapter é um pacote npm que põe o Claude Code em ACP.
type claudeAdapter struct {
	// module é o nome do pacote, que também é o caminho dele dentro de
	// `node_modules`.
	module string

	// bin é o nome que o npm liga no PATH.
	bin string
}

// claudeAdapters são os adaptadores conhecidos, do preferido ao tolerado.
//
// O segundo é o nome anterior do primeiro: o pacote foi renomeado e o antigo
// avisa na instalação que está deprecado. Quem instalou antes da renomeação
// continua com um adaptador que funciona, e recusá-lo mandaria essa pessoa
// reinstalar sem precisar.
var claudeAdapters = []claudeAdapter{
	{module: "@agentclientprotocol/claude-agent-acp", bin: "claude-agent-acp"},
	{module: "@zed-industries/claude-code-acp", bin: "claude-code-acp"},
}

// claudeLoginCommand é o comando que autentica o Claude Code.
//
// Ele não é derivável do comando do agente, como no Cursor: o que sobe o ACP
// aqui é o adaptador npm, e o adaptador não tem login nenhum — quem guarda a
// credencial é o CLI `claude`, que conduz a autenticação na primeira execução.
// Trocar um subcomando do adaptador por `login` mandaria a pessoa a um comando
// que não existe.
const claudeLoginCommand = "claude"

// detectClaudeCode procura o adaptador ACP do Claude Code nesta máquina.
//
// No Windows a busca vai ao pacote instalado, e não ao PATH, pelo mesmo motivo
// do Cursor: o npm liga o adaptador como `.cmd` e `.ps1`, e o Windows não cria
// processo a partir de nenhum dos dois. Sobra apontar para o par `node.exe` +
// `dist/index.js`, que ainda tem a vantagem de ser um processo que o app
// consegue encerrar sozinho.
func detectClaudeCode(p probe) Install {
	candidates := []func(probe, *searchLog) (Install, bool){}
	if p.goos == "windows" {
		candidates = append(candidates, claudeAdapterInNodeModules)
	}
	candidates = append(candidates, claudeAdapterOnPath)

	install := firstInstall(p, candidates)
	install.LoginCommand = claudeLoginCommand
	return install
}

// claudeAdapterOnPath procura o executável que o npm liga no PATH, que é o
// caminho normal em Linux e macOS.
func claudeAdapterOnPath(p probe, _ *searchLog) (Install, bool) {
	for _, adapter := range claudeAdapters {
		found, err := p.lookPath(adapter.bin)
		if err != nil {
			continue
		}
		if !spawnable(p.goos, found) {
			// No Windows o PATH resolve para o `.cmd` que o npm escreveu, e ele
			// não é executável: aceitá-lo daria um provider que falha no spawn.
			continue
		}
		// Sem subcomando: o adaptador já sobe falando ACP.
		return Install{Command: found, Source: found}, true
	}
	return Install{}, false
}

// claudeAdapterInNodeModules acha o ponto de entrada do adaptador na instalação
// global do npm.
//
// O pacote atual vence o antigo mesmo quando o antigo aparece num prefixo mais
// prioritário: quem tem os dois instalados atualizou em algum momento, e falar
// com o deprecado seria conversar com o que ficou para trás.
func claudeAdapterInNodeModules(p probe, searched *searchLog) (Install, bool) {
	prefixes := npmGlobalPrefixes(p, searched)
	for _, adapter := range claudeAdapters {
		for _, prefix := range prefixes {
			pkg := filepath.Join(prefix, "node_modules", filepath.FromSlash(adapter.module))
			// `dist/index.js` é o ponto de entrada declarado pelos dois pacotes.
			entry := filepath.Join(pkg, "dist", "index.js")
			if !exists(p, searched, entry) {
				continue
			}
			node := nodeExecutableFor(p, searched, prefix)
			if node == "" {
				// Pacote instalado e nenhum node para rodá-lo: dizer que
				// encontrou faria a pessoa salvar um provider que nunca sobe.
				continue
			}
			return Install{
				Command: node,
				Args:    []string{entry},
				Version: claudeAdapterVersion(p, searched, pkg),
				Source:  entry,
			}, true
		}
	}
	return Install{}, false
}

// npmGlobalPrefixes são os diretórios onde a instalação global do npm põe o
// `node_modules` no Windows, na ordem em que valem.
//
// A lista é do disco, e não do `npm prefix -g`: perguntar ao npm seria executar
// um programa, e a detecção não executa nada — ela olha onde os instaladores
// põem as coisas.
func npmGlobalPrefixes(p probe, searched *searchLog) []string {
	var prefixes []string
	add := func(dir string) {
		if dir == "" || slices.Contains(prefixes, dir) {
			return
		}
		prefixes = append(prefixes, dir)
		searched.add(dir)
	}

	// O instalador oficial do Node põe o global junto do `node.exe`.
	if base := strings.TrimSpace(p.getenv("ProgramFiles")); base != "" {
		add(filepath.Join(base, "nodejs"))
	}
	// Prefixo de usuário: o padrão de quem instala pacotes sem ser
	// administrador, e o que o npm usa quando alguém configura `prefix`.
	appdata := strings.TrimSpace(p.getenv("APPDATA"))
	if appdata != "" {
		add(filepath.Join(appdata, "npm"))
	}
	for _, dir := range nvmPrefixes(p, searched, appdata) {
		add(dir)
	}
	return prefixes
}

// nvmPrefixes lista as versões do Node instaladas pelo nvm-windows, da mais
// recente para a mais antiga.
//
// A ordem importa porque cada versão tem o próprio global: uma versão antiga
// que ficou no disco guarda os pacotes daquela época, e subir o adaptador dela
// seria falar com um agente que a pessoa já atualizou.
func nvmPrefixes(p probe, searched *searchLog, appdata string) []string {
	if appdata == "" {
		return nil
	}
	root := filepath.Join(appdata, "nvm")
	searched.add(root)
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
	slices.SortFunc(names, func(a, b string) int { return nvmVersionOrder(b) - nvmVersionOrder(a) })

	dirs := make([]string, 0, len(names))
	for _, name := range names {
		dirs = append(dirs, filepath.Join(root, name))
	}
	return dirs
}

// nvmVersionPattern aceita o nome de diretório que o nvm-windows usa: a versão
// do Node, com ou sem o `v` na frente. Três dígitos por componente é o teto
// porque é o que o peso de `nvmVersionOrder` comporta.
var nvmVersionPattern = regexp.MustCompile(`^v?(\d{1,3})\.(\d{1,3})\.(\d{1,3})$`)

// nvmVersionOrder transforma a versão em número comparável — comparar como
// texto poria a 9 na frente da 22. O peso de mil por componente mantém o maior
// valor possível (999999999) dentro do `int` de 32 bits, para a ordenação valer
// o mesmo em qualquer arquitetura.
func nvmVersionOrder(name string) int {
	match := nvmVersionPattern.FindStringSubmatch(name)
	if match == nil {
		return 0
	}
	order := 0
	for _, group := range match[1:] {
		value, _ := strconv.Atoi(group)
		order = order*1000 + value
	}
	return order
}

// nodeExecutableFor acha o `node.exe` que roda o adaptador daquele prefixo.
//
// O node vizinho vem primeiro porque é o dono daqueles pacotes; na falta dele
// vale o do PATH, que é o caso do prefixo de usuário (`%APPDATA%\npm`), onde
// só ficam os pacotes.
func nodeExecutableFor(p probe, searched *searchLog, prefix string) string {
	local := filepath.Join(prefix, "node.exe")
	if exists(p, searched, local) {
		return local
	}
	if found, err := p.lookPath("node"); err == nil && spawnable(p.goos, found) {
		return found
	}
	return ""
}

// claudeAdapterVersion lê a versão no `package.json` do adaptador. É o único
// lugar onde ela está: diferente do Cursor, cujo diretório versionado a revela
// pelo nome, aqui o caminho não diz nada sobre o que está instalado.
//
// Não conseguir ler não invalida a detecção: o comando encontrado continua
// valendo, e a tela só deixa de exibir um dado.
func claudeAdapterVersion(p probe, searched *searchLog, pkg string) string {
	manifest := filepath.Join(pkg, "package.json")
	data, err := p.readFile(manifest)
	if err != nil {
		searched.fail(manifest, err)
		return ""
	}
	var manifesto struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifesto); err != nil {
		return ""
	}
	// A versão sai de um arquivo de terceiro e vai para a tela: entra saneada,
	// como todo texto de fora (AEP-0084 D11).
	return SanitizeLabel(manifesto.Version)
}
