package acp

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// pacoteFalso monta no disco um pacote npm como o `npm install --prefix` o
// deixaria: o manifesto e o arquivo que o `bin` aponta. É o que permite testar a
// resolução do ponto de entrada sem baixar nada do registro npm.
func pacoteFalso(t *testing.T, prefixo, nome, manifesto string, arquivos ...string) {
	t.Helper()
	dir := filepath.Join(prefixo, "node_modules", filepath.FromSlash(nome))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("não deu para criar o pacote falso: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte(manifesto), 0o644); err != nil {
		t.Fatalf("não deu para gravar o manifesto: %v", err)
	}
	for _, arquivo := range arquivos {
		caminho := filepath.Join(dir, filepath.FromSlash(arquivo))
		if err := os.MkdirAll(filepath.Dir(caminho), 0o755); err != nil {
			t.Fatalf("não deu para criar o diretório de %s: %v", arquivo, err)
		}
		if err := os.WriteFile(caminho, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
			t.Fatalf("não deu para gravar %s: %v", arquivo, err)
		}
	}
}

func TestNPMEntryPointLeOBinDoManifesto(t *testing.T) {
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "@agentclientprotocol/codex-acp",
		`{"name":"@agentclientprotocol/codex-acp","version":"1.1.9","bin":{"codex-acp":"dist/index.js"}}`,
		"dist/index.js")

	pkg, err := NPMEntryPoint(prefixo, "@agentclientprotocol/codex-acp")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada de um pacote instalado: %v", err)
	}
	esperado := filepath.Join(prefixo, "node_modules", filepath.FromSlash("@agentclientprotocol/codex-acp"), "dist", "index.js")
	if pkg.EntryPoint != esperado {
		t.Errorf("ponto de entrada = %q, queria %q", pkg.EntryPoint, esperado)
	}
	if pkg.Version != "1.1.9" {
		t.Errorf("versão = %q, queria a do manifesto", pkg.Version)
	}
}

func TestNPMEntryPointAceitaBinComoTexto(t *testing.T) {
	// O npm aceita `bin` como texto quando o pacote tem um executável só, e
	// vários dos 21 pacotes do catálogo o declaram assim.
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"cli.js"}`, "cli.js")

	pkg, err := NPMEntryPoint(prefixo, "agente")
	if err != nil {
		t.Fatalf("recusou um `bin` de texto, que é formato válido: %v", err)
	}
	if filepath.Base(pkg.EntryPoint) != "cli.js" {
		t.Errorf("ponto de entrada = %q, queria o cli.js", pkg.EntryPoint)
	}
}

func TestNPMEntryPointPrefereOBinComONomeDoPacote(t *testing.T) {
	// Com mais de um executável, o que leva o nome do pacote é o principal — é a
	// convenção do npm e é o que o `npx <pacote>` roda. Escolher outro subiria
	// uma ferramenta auxiliar em vez do agente.
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "@escopo/codex-acp",
		`{"name":"@escopo/codex-acp","bin":{"aaa-ferramenta":"tools/aux.js","codex-acp":"dist/main.js"}}`,
		"tools/aux.js", "dist/main.js")

	pkg, err := NPMEntryPoint(prefixo, "@escopo/codex-acp")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada: %v", err)
	}
	if filepath.Base(pkg.EntryPoint) != "main.js" {
		t.Errorf("ponto de entrada = %q, queria o bin com o nome do pacote", pkg.EntryPoint)
	}
}

func TestNPMEntryPointIgnoraOBinPreferidoQueEstaEmBranco(t *testing.T) {
	// A entrada com o nome do pacote ganha por ser a convenção do npm, mas
	// ganhar em branco não é ganhar: o pacote tem outro executável declarado, e
	// o desfecho tem de ser ele, e não um ponto de entrada vazio.
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente",
		`{"name":"agente","bin":{"agente":"   ","auxiliar":"tools/aux.js"}}`,
		"tools/aux.js")

	pkg, err := NPMEntryPoint(prefixo, "agente")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada tendo uma entrada válida: %v", err)
	}
	if filepath.Base(pkg.EntryPoint) != "aux.js" {
		t.Errorf("ponto de entrada = %q, queria a entrada que não está em branco", pkg.EntryPoint)
	}
}

func TestNPMEntryPointAparaOsEspacosDoBinDeclarado(t *testing.T) {
	// Manifesto escrito à mão traz espaço em volta do valor. Levá-lo adiante
	// trocaria "o `bin` está vazio" por um "arquivo não existe" alguns passos
	// depois, apontando para um caminho que parece certo na mensagem.
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"  cli.js  "}`, "cli.js")

	pkg, err := NPMEntryPoint(prefixo, "agente")
	if err != nil {
		t.Fatalf("recusou um `bin` que só tinha espaço em volta: %v", err)
	}
	if filepath.Base(pkg.EntryPoint) != "cli.js" {
		t.Errorf("ponto de entrada = %q, queria o cli.js sem os espaços", pkg.EntryPoint)
	}
}

func TestNPMEntryPointRecusaNomeQueNaoEDeUmPacoteInstalado(t *testing.T) {
	// `..` colapsa para o próprio prefixo e `.` para o `node_modules`: nenhum
	// dos dois sai dali, e por isso uma guarda contra o prefixo os deixaria
	// passar — para ler um manifesto que não é o de um pacote instalado, e
	// resolver o `bin` a partir de um diretório qualquer.
	prefixo := t.TempDir()
	manifesto := `{"name":"nao-e-pacote","bin":"cli.js"}`
	if err := os.WriteFile(filepath.Join(prefixo, "package.json"), []byte(manifesto), 0o644); err != nil {
		t.Fatalf("não deu para gravar o manifesto de fora: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(prefixo, "node_modules"), 0o755); err != nil {
		t.Fatalf("não deu para criar o node_modules: %v", err)
	}

	for _, nome := range []string{"..", ".", "../..", "../outro"} {
		if _, err := NPMEntryPoint(prefixo, nome); err == nil {
			t.Errorf("aceitou %q como nome de pacote instalado", nome)
		}
	}
}

func TestNPMEntryPointRecusaBinQueSaiDaInstalacao(t *testing.T) {
	// Nada vindo do manifesto vira caminho de execução fora do diretório do
	// agente (AEP-0086 D9): um `bin` apontando para fora é o pacote pedindo que
	// o app execute outra coisa.
	prefixo := t.TempDir()
	fora := filepath.Join(prefixo, "fora.js")
	if err := os.WriteFile(fora, []byte("//"), 0o644); err != nil {
		t.Fatalf("não deu para gravar o arquivo de fora: %v", err)
	}
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"../../../fora.js"}`)

	if _, err := NPMEntryPoint(filepath.Join(prefixo, "node_modules", "agente"), "agente"); err == nil {
		t.Fatal("aceitou um `bin` que sai da instalação")
	}
}

func TestNPMEntryPointRecusaBinQueSaiDaInstalacaoPorUmLink(t *testing.T) {
	// Caminho de dentro do prefixo que leva para fora dele: o texto passa na
	// guarda, o destino não. Quem escolhe o destino é o pacote, então a
	// verificação tem de ser sobre o que se abre, não sobre o que se lê.
	prefixo := t.TempDir()
	fora := filepath.Join(t.TempDir(), "fora.js")
	if err := os.WriteFile(fora, []byte("//"), 0o644); err != nil {
		t.Fatalf("não deu para gravar o arquivo de fora: %v", err)
	}
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"dist/index.js"}`)
	link := filepath.Join(prefixo, "node_modules", "agente", "dist", "index.js")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatalf("não deu para criar o diretório do link: %v", err)
	}
	if err := os.Symlink(fora, link); err != nil {
		// No Windows criar link exige privilégio, e não é a máquina que está
		// sendo testada aqui.
		t.Skipf("não deu para criar o link neste sistema: %v", err)
	}

	if _, err := NPMEntryPoint(prefixo, "agente"); err == nil {
		t.Fatal("aceitou um `bin` que sai da instalação por um link")
	}
}

func TestNPMEntryPointAceitaLinkQueFicaDentroDaInstalacao(t *testing.T) {
	// O npm liga arquivos dentro da própria instalação o tempo todo; recusar
	// link por ser link quebraria instalação legítima.
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"dist/index.js"}`, "dist/real.js")
	dist := filepath.Join(prefixo, "node_modules", "agente", "dist")
	if err := os.Symlink(filepath.Join(dist, "real.js"), filepath.Join(dist, "index.js")); err != nil {
		t.Skipf("não deu para criar o link neste sistema: %v", err)
	}

	pkg, err := NPMEntryPoint(prefixo, "agente")
	if err != nil {
		t.Fatalf("recusou um link que fica dentro da instalação: %v", err)
	}
	if filepath.Base(pkg.EntryPoint) != "index.js" {
		t.Errorf("ponto de entrada = %q, queria o caminho declarado no manifesto", pkg.EntryPoint)
	}
}

func TestNPMEntryPointRecusaManifestoSemBin(t *testing.T) {
	// Sem `bin` não há o que rodar, e adivinhar um `dist/index.js` daria um
	// provider que falha na primeira conversa (D8).
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","main":"index.js"}`, "index.js")

	_, err := NPMEntryPoint(prefixo, "agente")
	if err == nil {
		t.Fatal("aceitou um manifesto sem `bin`")
	}
	if !strings.Contains(err.Error(), "bin") {
		t.Errorf("erro = %q, queria que ele nomeasse o `bin` que falta", err)
	}
}

func TestNPMEntryPointRecusaBinQueNaoEstaNoDisco(t *testing.T) {
	prefixo := t.TempDir()
	pacoteFalso(t, prefixo, "agente", `{"name":"agente","bin":"dist/index.js"}`)

	if _, err := NPMEntryPoint(prefixo, "agente"); err == nil {
		t.Fatal("aceitou um `bin` que não existe no disco")
	}
}

func TestNPMEntryPointRecusaPacoteNaoInstalado(t *testing.T) {
	if _, err := NPMEntryPoint(t.TempDir(), "agente"); err == nil {
		t.Fatal("aceitou um prefixo sem o pacote")
	}
}

// windowsComNode monta um Windows com o Node do instalador oficial e o npm ao
// lado dele, sem nada no PATH — que é o que o app vê quando foi aberto pelo
// lançador do sistema.
func windowsComNode() fakeMachine {
	prefixo := filepath.Join(`C:\Program Files`, "nodejs")
	return fakeMachine{
		goos: "windows",
		env: map[string]string{
			"ProgramFiles": `C:\Program Files`,
			"APPDATA":      `C:\Users\alguem\AppData\Roaming`,
		},
		files: []string{
			filepath.Join(prefixo, "node.exe"),
			filepath.Join(prefixo, "node_modules", "npm", "bin", "npm-cli.js"),
		},
	}
}

func TestFindNodeRuntimeNoWindowsAchaOParNodeENpmSemOPath(t *testing.T) {
	machine := windowsComNode()

	runtime := findNodeRuntime(machine.probe())

	if !runtime.Found {
		t.Fatalf("não achou o Node numa máquina que o tem: %+v", runtime)
	}
	prefixo := filepath.Join(`C:\Program Files`, "nodejs")
	if runtime.Node != filepath.Join(prefixo, "node.exe") {
		t.Errorf("node = %q, queria o do prefixo", runtime.Node)
	}
	if runtime.NPMScript != filepath.Join(prefixo, "node_modules", "npm", "bin", "npm-cli.js") {
		t.Errorf("npm-cli.js = %q, queria o que fica ao lado do node", runtime.NPMScript)
	}
}

func TestNPMCommandRodaONpmPeloNodeENaoPeloArquivoDeLote(t *testing.T) {
	// No Windows o `npm` do PATH é `npm.cmd`, e o app não cria processo a partir
	// de arquivo de lote (AEP-0084 D15): passar por um intérprete deixaria o npm
	// como processo neto, e cancelar não encerraria quem escreve no disco.
	machine := windowsComNode()
	machine.path = map[string]string{"npm": `C:\Program Files\nodejs\npm.cmd`}

	runtime := findNodeRuntime(machine.probe())
	command, args, ok := runtime.NPMCommand()

	if !ok {
		t.Fatal("não montou o comando do npm com Node e npm-cli.js presentes")
	}
	if command != runtime.Node {
		t.Errorf("comando = %q, queria o node", command)
	}
	if !slices.Equal(args, []string{runtime.NPMScript}) {
		t.Errorf("argumentos = %q, queriam o npm-cli.js", args)
	}
	if runtime.NPM != "" {
		t.Errorf("npm = %q, queria vazio: o `.cmd` não é spawnável", runtime.NPM)
	}
}

func TestFindNodeRuntimeSemNodeDizOndeProcurou(t *testing.T) {
	// Sem Node não se oferece instalação, e o motivo precisa ser verificável
	// (AEP-0086 D7): "não encontrei" sem dizer onde se olhou não ajuda quem vai
	// instalar o Node.
	machine := fakeMachine{
		goos: "windows",
		env: map[string]string{
			"ProgramFiles": `C:\Program Files`,
			"APPDATA":      `C:\Users\alguem\AppData\Roaming`,
		},
	}

	runtime := findNodeRuntime(machine.probe())

	if runtime.Found {
		t.Fatalf("achou Node numa máquina sem Node: %+v", runtime)
	}
	if len(runtime.Searched) == 0 {
		t.Error("não registrou onde procurou")
	}
	if _, _, ok := runtime.NPMCommand(); ok {
		t.Error("montou comando de npm sem Node")
	}
}

func TestFindNodeRuntimeUnixUsaOPathEAchaONpmDoPrefixo(t *testing.T) {
	// Em Unix o `node` fica em `bin/` e os pacotes em `lib/node_modules/` do
	// mesmo prefixo — layout do instalador, não nosso. Os caminhos são montados
	// com filepath para o teste valer na máquina que o roda, que pode ser a que
	// usa a outra barra.
	node := filepath.Join("/usr", "bin", "node")
	npmCLI := filepath.Join("/usr", "lib", "node_modules", "npm", "bin", "npm-cli.js")
	machine := fakeMachine{
		goos:  "linux",
		env:   map[string]string{"HOME": filepath.Join("/home", "alguem")},
		path:  map[string]string{"node": node},
		files: []string{npmCLI},
	}

	runtime := findNodeRuntime(machine.probe())

	if !runtime.Found || runtime.Node != node {
		t.Fatalf("não usou o node do PATH: %+v", runtime)
	}
	if runtime.NPMScript != npmCLI {
		t.Errorf("npm-cli.js = %q, queria o do prefixo do node", runtime.NPMScript)
	}
}

func TestFindNodeRuntimeLeAVersaoDoDiretorioDoNvm(t *testing.T) {
	// A versão do runtime é exibida quando o caminho a revela (D7). Descobri-la
	// de outro jeito exigiria executar o `node`, e a procura não executa nada.
	machine := fakeMachine{
		goos:  "windows",
		env:   map[string]string{"APPDATA": `C:\Users\alguem\AppData\Roaming`},
		files: []string{filepath.Join(`C:\Users\alguem\AppData\Roaming`, "nvm", "v24.4.1", "node.exe")},
		dirs: map[string][]string{
			filepath.Join(`C:\Users\alguem\AppData\Roaming`, "nvm"): {"v24.4.1"},
		},
	}

	runtime := findNodeRuntime(machine.probe())

	if !runtime.Found {
		t.Fatalf("não achou o node do nvm: %+v", runtime)
	}
	if runtime.Version != "24.4.1" {
		t.Errorf("versão = %q, queria a do diretório versionado", runtime.Version)
	}
}
