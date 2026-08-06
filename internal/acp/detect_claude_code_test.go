package acp

import (
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	adaptadorAtual = "@agentclientprotocol/claude-agent-acp"
	adaptadorAntes = "@zed-industries/claude-code-acp"
)

// windowsComAdaptador monta a máquina Windows como ela é depois de um
// `npm install -g`: o pacote dentro do `node_modules` do prefixo global, com o
// `node.exe` do instalador oficial ao lado.
func windowsComAdaptador(modulos ...string) (fakeMachine, string) {
	prefixo := filepath.Join(`C:\Program Files`, "nodejs")
	machine := fakeMachine{
		goos: "windows",
		env: map[string]string{
			"ProgramFiles": `C:\Program Files`,
			"APPDATA":      `C:\Users\alguem\AppData\Roaming`,
		},
		files:     []string{filepath.Join(prefixo, "node.exe")},
		conteudos: map[string]string{},
	}
	for _, modulo := range modulos {
		pkg := pacoteEm(prefixo, modulo)
		machine.files = append(machine.files, filepath.Join(pkg, "dist", "index.js"))
		machine.conteudos[filepath.Join(pkg, "package.json")] = `{"name":"` + modulo + `","version":"0.65.0"}`
	}
	return machine, prefixo
}

func pacoteEm(prefixo, modulo string) string {
	return filepath.Join(prefixo, "node_modules", filepath.FromSlash(modulo))
}

func TestDetectClaudeCodeWindowsUsaOParNodeIndexDoAdaptador(t *testing.T) {
	machine, prefixo := windowsComAdaptador(adaptadorAtual)

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("adaptador não encontrado numa máquina que o tem instalado: %+v", install)
	}
	entrada := filepath.Join(pacoteEm(prefixo, adaptadorAtual), "dist", "index.js")
	if install.Command != filepath.Join(prefixo, "node.exe") {
		t.Errorf("comando = %q, queria o node do prefixo", install.Command)
	}
	// Sem subcomando: diferente do Cursor, o adaptador já sobe falando ACP, e um
	// `acp` a mais seria argumento que ele não entende.
	if !slices.Equal(install.Args, []string{entrada}) {
		t.Errorf("argumentos = %q, queria só o ponto de entrada", install.Args)
	}
	if install.Source != entrada {
		t.Errorf("origem = %q, queria %q", install.Source, entrada)
	}
	if install.Version != "0.65.0" {
		t.Errorf("versão = %q, queria a do package.json do pacote", install.Version)
	}
}

func TestDetectClaudeCodeAceitaOPacoteAntigoQuandoESoOQueHa(t *testing.T) {
	// O pacote foi renomeado e o antigo avisa que está deprecado. Quem o
	// instalou antes disso tem um adaptador que funciona: recusá-lo mandaria
	// essa pessoa reinstalar sem precisar.
	machine, prefixo := windowsComAdaptador(adaptadorAntes)

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("recusou o adaptador anterior, que ainda funciona: %+v", install)
	}
	entrada := filepath.Join(pacoteEm(prefixo, adaptadorAntes), "dist", "index.js")
	if !slices.Equal(install.Args, []string{entrada}) {
		t.Errorf("argumentos = %q, queria %q", install.Args, entrada)
	}
}

func TestDetectClaudeCodeComOsDoisPacotesPrefereOAtual(t *testing.T) {
	machine, prefixo := windowsComAdaptador(adaptadorAntes, adaptadorAtual)

	install := detectClaudeCode(machine.probe())

	entrada := filepath.Join(pacoteEm(prefixo, adaptadorAtual), "dist", "index.js")
	if install.Source != entrada {
		t.Fatalf("origem = %q, queria o pacote atual (%q)", install.Source, entrada)
	}
}

func TestDetectClaudeCodeOAtualVenceOAntigoDeOutroPrefixo(t *testing.T) {
	// Quem tem os dois instalados atualizou em algum momento; falar com o
	// deprecado seria conversar com o que ficou para trás, mesmo que ele esteja
	// no prefixo que a procura consulta primeiro.
	machine, prefixoDoSistema := windowsComAdaptador(adaptadorAntes)
	prefixoDoUsuario := filepath.Join(`C:\Users\alguem\AppData\Roaming`, "npm")
	atual := pacoteEm(prefixoDoUsuario, adaptadorAtual)
	machine.files = append(machine.files, filepath.Join(atual, "dist", "index.js"))
	// O prefixo de usuário guarda pacotes, não interpretador: o node do pacote
	// dele é o do PATH.
	machine.path = map[string]string{"node": filepath.Join(prefixoDoSistema, "node.exe")}

	install := detectClaudeCode(machine.probe())

	if !strings.Contains(install.Source, filepath.FromSlash(adaptadorAtual)) {
		t.Fatalf("origem = %q, queria o pacote atual do prefixo de usuário", install.Source)
	}
	if install.Command != filepath.Join(prefixoDoSistema, "node.exe") {
		t.Errorf("comando = %q", install.Command)
	}
}

func TestDetectClaudeCodeAchaOAdaptadorInstaladoPeloNvm(t *testing.T) {
	appdata := `C:\Users\alguem\AppData\Roaming`
	nvm := filepath.Join(appdata, "nvm")
	// Fora de ordem de propósito, e com uma versão que empataria na comparação
	// por texto: a 22 é mais nova que a 9.
	versoes := []string{"v9.9.9", "v22.11.0", "v20.1.3"}
	machine := fakeMachine{
		goos:      "windows",
		env:       map[string]string{"APPDATA": appdata},
		dirs:      map[string][]string{nvm: versoes},
		conteudos: map[string]string{},
	}
	for _, versao := range versoes {
		prefixo := filepath.Join(nvm, versao)
		machine.files = append(machine.files,
			filepath.Join(prefixo, "node.exe"),
			filepath.Join(pacoteEm(prefixo, adaptadorAtual), "dist", "index.js"))
	}

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("não achou o adaptador no layout do nvm: %+v", install)
	}
	if !strings.Contains(install.Command, filepath.Join("nvm", "v22.11.0")) {
		t.Errorf("comando = %q, queria o da versão mais recente", install.Command)
	}
}

func TestOrdemDeVersaoDoNvmNaoEstouraNoMaiorNumeroAceito(t *testing.T) {
	// O maior nome aceito tem de continuar cabendo no `int` de 32 bits, senão a
	// ordenação daria resultado diferente por arquitetura.
	const maiorInt32 = 1<<31 - 1
	if ordem := nvmVersionOrder("v999.999.999"); ordem <= 0 || ordem > maiorInt32 {
		t.Fatalf("ordem da maior versão = %d, queria positiva e dentro de int32", ordem)
	}
	if nvmVersionOrder("v999.999.999") <= nvmVersionOrder("v22.11.0") {
		t.Error("a versão maior tem de vir na frente")
	}
	// Acima do teto o nome deixa de ser reconhecido, em vez de virar uma ordem
	// truncada que poria essa versão no lugar errado da fila.
	if ordem := nvmVersionOrder("v1000.0.0"); ordem != 0 {
		t.Errorf("ordem = %d, queria 0 para nome fora do padrão", ordem)
	}
}

func TestDetectClaudeCodeNoPathEmLinux(t *testing.T) {
	machine := fakeMachine{
		goos: "linux",
		env:  map[string]string{"HOME": "/home/alguem"},
		path: map[string]string{"claude-agent-acp": "/usr/local/bin/claude-agent-acp"},
	}

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("adaptador do PATH não encontrado: %+v", install)
	}
	if install.Command != "/usr/local/bin/claude-agent-acp" {
		t.Errorf("comando = %q", install.Command)
	}
	if len(install.Args) != 0 {
		t.Errorf("argumentos = %q; o adaptador sobe em ACP sem subcomando", install.Args)
	}
	if install.Version != "" {
		t.Errorf("versão = %q; pelo PATH não há como saber sem executar o adaptador", install.Version)
	}
}

func TestDetectClaudeCodeNoPathAceitaOBinarioAnterior(t *testing.T) {
	machine := fakeMachine{
		goos: "linux",
		env:  map[string]string{"HOME": "/home/alguem"},
		path: map[string]string{"claude-code-acp": "/usr/local/bin/claude-code-acp"},
	}

	install := detectClaudeCode(machine.probe())

	if !install.Found || install.Command != "/usr/local/bin/claude-code-acp" {
		t.Fatalf("não aceitou o binário do pacote anterior: %+v", install)
	}
}

func TestDetectClaudeCodeWindowsNaoAceitaOAtalhoDeLoteDoNpm(t *testing.T) {
	// No Windows o npm liga o adaptador como `.cmd`, e o Windows não cria
	// processo a partir de arquivo de lote: aceitá-lo daria um provider que
	// falha no spawn.
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"APPDATA": `C:\Users\alguem\AppData\Roaming`},
		path: map[string]string{"claude-agent-acp": `C:\Users\alguem\AppData\Roaming\npm\claude-agent-acp.cmd`},
	}

	install := detectClaudeCode(machine.probe())

	if install.Found {
		t.Fatalf("aceitou um arquivo de lote como comando do agente: %+v", install)
	}
}

func TestDetectClaudeCodeSemNodeNaoOfereceOPacote(t *testing.T) {
	machine, prefixo := windowsComAdaptador(adaptadorAtual)
	// Pacote instalado, interpretador nenhum: nem no prefixo, nem no PATH.
	machine.files = slices.DeleteFunc(machine.files, func(caminho string) bool {
		return caminho == filepath.Join(prefixo, "node.exe")
	})

	install := detectClaudeCode(machine.probe())

	if install.Found {
		t.Fatalf("ofereceu um pacote que não tem como executar: %+v", install)
	}
}

func TestDetectClaudeCodeUsaONodeDoPathQuandoOPrefixoNaoTemUm(t *testing.T) {
	appdata := `C:\Users\alguem\AppData\Roaming`
	prefixo := filepath.Join(appdata, "npm")
	machine := fakeMachine{
		goos:  "windows",
		env:   map[string]string{"APPDATA": appdata},
		files: []string{filepath.Join(pacoteEm(prefixo, adaptadorAtual), "dist", "index.js")},
		path:  map[string]string{"node": `C:\Program Files\nodejs\node.exe`},
	}

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("não achou o adaptador do prefixo de usuário: %+v", install)
	}
	if install.Command != `C:\Program Files\nodejs\node.exe` {
		t.Errorf("comando = %q, queria o node do PATH", install.Command)
	}
}

func TestDetectClaudeCodeSemPackageJsonSegueValendo(t *testing.T) {
	// A versão é enfeite: o que faz o provider funcionar é o par node/entrada.
	machine, prefixo := windowsComAdaptador(adaptadorAtual)
	delete(machine.conteudos, filepath.Join(pacoteEm(prefixo, adaptadorAtual), "package.json"))

	install := detectClaudeCode(machine.probe())

	if !install.Found {
		t.Fatalf("descartou o adaptador por causa da versão: %+v", install)
	}
	if install.Version != "" {
		t.Errorf("versão = %q, queria vazia", install.Version)
	}
}

func TestDetectClaudeCodeVersaoIlegivelNaoViraRotulo(t *testing.T) {
	machine, prefixo := windowsComAdaptador(adaptadorAtual)
	manifesto := filepath.Join(pacoteEm(prefixo, adaptadorAtual), "package.json")
	// A versão vem de arquivo de terceiro e vai para a tela: escape de terminal
	// e quebra de linha não podem chegar inteiros ao leitor de telas.
	machine.conteudos[manifesto] = "{\"version\":\"\x1b[31m0.65.0\x1b[0m\\nlinha\"}"

	install := detectClaudeCode(machine.probe())

	if strings.ContainsAny(install.Version, "\x1b\n") {
		t.Fatalf("versão saiu crua: %q", install.Version)
	}
}

func TestDetectClaudeCodeSempreDizComoFazerLogin(t *testing.T) {
	// O login não é o adaptador com outro subcomando: quem autentica é o CLI
	// `claude`. E a instrução vale mesmo sem adaptador instalado — quem digitou
	// o comando na mão também precisa dela.
	comAdaptador, _ := windowsComAdaptador(adaptadorAtual)
	semNada := fakeMachine{goos: "linux", env: map[string]string{"HOME": "/home/alguem"}}

	for nome, machine := range map[string]fakeMachine{"com adaptador": comAdaptador, "sem nada": semNada} {
		install := detectClaudeCode(machine.probe())
		if install.LoginCommand != "claude" {
			t.Errorf("%s: comando de login = %q, queria o CLI do Claude Code", nome, install.LoginCommand)
		}
	}
}

func TestDetectCursorNaoInventaComandoDeLogin(t *testing.T) {
	// No Cursor o login é o mesmo programa com outro subcomando, e quem o monta
	// é a tela, a partir do comando configurado — que a pessoa pode ter
	// editado. Um comando fixo daqui passaria por cima dessa escolha.
	machine := windowsInstall("2026.07.23-e383d2b")

	install := detectCursor(machine.probe())

	if install.LoginCommand != "" {
		t.Fatalf("comando de login = %q, queria vazio", install.LoginCommand)
	}
}

func TestDetectClaudeCodeNaoEncontradoDizOndeProcurou(t *testing.T) {
	machine := fakeMachine{
		goos: "windows",
		env: map[string]string{
			"ProgramFiles": `C:\Program Files`,
			"APPDATA":      `C:\Users\alguem\AppData\Roaming`,
		},
	}

	install := detectClaudeCode(machine.probe())

	if install.Found {
		t.Fatalf("encontrou adaptador numa máquina sem ele: %+v", install)
	}
	if install.Command != "" || len(install.Args) != 0 {
		t.Errorf("sugeriu comando sem ter encontrado nada: %+v", install)
	}
	for _, esperado := range []string{
		filepath.Join(`C:\Program Files`, "nodejs"),
		filepath.Join(`C:\Users\alguem\AppData\Roaming`, "npm"),
	} {
		if !slices.Contains(install.Searched, esperado) {
			t.Errorf("lugares consultados = %q, queria conter %q", install.Searched, esperado)
		}
	}
	if len(install.Failures) != 0 {
		t.Errorf("ausência virou falha de procura: %q", install.Failures)
	}
}

func TestDetectAgentClaudeCodeNaoConfundeProcuraFalhadaComAgenteAusente(t *testing.T) {
	machine, prefixo := windowsComAdaptador()
	entrada := filepath.Join(pacoteEm(prefixo, adaptadorAtual), "dist", "index.js")
	machine.recusas = map[string]error{entrada: fs.ErrPermission}

	_, err := detectAgent(AgentKindClaudeCode, machine.probe())

	if err == nil {
		t.Fatal("procura interrompida por permissão passou como adaptador não instalado")
	}
	if !strings.Contains(err.Error(), "claude-agent-acp") {
		t.Errorf("erro não diz o que não deu para conferir: %v", err)
	}
}

func TestDetectAgentClaudeCodeSemInstalacaoNaoEErro(t *testing.T) {
	machine := fakeMachine{goos: "linux", env: map[string]string{"HOME": "/home/alguem"}}

	install, err := detectAgent(AgentKindClaudeCode, machine.probe())

	if err != nil {
		t.Fatalf("não achar o adaptador virou erro: %v", err)
	}
	if install.Found {
		t.Errorf("encontrou adaptador numa máquina sem nada: %+v", install)
	}
}

func TestDetectAgentClaudeCodeNaMaquinaDoTesteNaoFalha(t *testing.T) {
	// Não afirma nada sobre estar instalado — isso varia por máquina. O que
	// precisa valer sempre é: a detecção não é erro, e quando ela diz que
	// encontrou, há comando para subir.
	install, err := DetectAgent(AgentKindClaudeCode)
	if err != nil {
		t.Fatalf("detecção do Claude Code devolveu erro: %v", err)
	}
	if install.Found && strings.TrimSpace(install.Command) == "" {
		t.Errorf("disse que encontrou sem comando: %+v", install)
	}
	t.Logf("detecção nesta máquina: %+v", install)
}
