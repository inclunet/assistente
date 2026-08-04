package acp

import (
	"errors"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeMachine descreve uma máquina para a detecção: quais arquivos existem,
// quais variáveis de ambiente valem e o que o PATH resolve. Sem isso o teste
// dependeria de o Cursor estar instalado em quem roda o teste — e ele passaria
// ou falharia por um motivo que não tem nada a ver com o código.
type fakeMachine struct {
	goos  string
	env   map[string]string
	files []string
	dirs  map[string][]string
	path  map[string]string

	// recusas são os caminhos que a máquina se nega a conferir, como um
	// diretório sem permissão de leitura. É o que separa "não existe" de "não
	// deu para olhar" nos testes.
	recusas map[string]error
}

func (m fakeMachine) probe() probe {
	return probe{
		goos:   m.goos,
		getenv: func(key string) string { return m.env[key] },
		lookPath: func(name string) (string, error) {
			if found, ok := m.path[name]; ok {
				return found, nil
			}
			return "", errors.New("não está no PATH")
		},
		isFile: func(path string) (bool, error) {
			if err, ok := m.recusas[path]; ok {
				return false, err
			}
			return slices.Contains(m.files, path), nil
		},
		readDir: func(dir string) ([]fs.DirEntry, error) {
			if err, ok := m.recusas[dir]; ok {
				return nil, err
			}
			names, ok := m.dirs[dir]
			if !ok {
				// O mesmo erro que o sistema devolve: ausência é resposta, e a
				// detecção só guarda o que não deu para conferir.
				return nil, fs.ErrNotExist
			}
			entries := make([]fs.DirEntry, 0, len(names))
			for _, name := range names {
				entries = append(entries, fakeDirEntry(name))
			}
			return entries, nil
		},
	}
}

type fakeDirEntry string

func (e fakeDirEntry) Name() string               { return string(e) }
func (e fakeDirEntry) IsDir() bool                { return true }
func (e fakeDirEntry) Type() fs.FileMode          { return fs.ModeDir }
func (e fakeDirEntry) Info() (fs.FileInfo, error) { return nil, errors.New("sem info") }

// windowsInstall monta a instalação do Cursor no Windows como ela é de fato:
// wrapper na raiz e o par node/index dentro do diretório da versão.
func windowsInstall(versions ...string) fakeMachine {
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")
	machine := fakeMachine{
		goos:  "windows",
		env:   map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
		files: []string{filepath.Join(home, "agent.ps1"), filepath.Join(home, "agent.cmd")},
		dirs:  map[string][]string{filepath.Join(home, "versions"): versions},
		path:  map[string]string{"powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`},
	}
	for _, version := range versions {
		dir := filepath.Join(home, "versions", version)
		machine.files = append(machine.files, filepath.Join(dir, "node.exe"), filepath.Join(dir, "index.js"))
	}
	return machine
}

func TestDetectCursorWindowsUsaOParNodeIndexDaVersaoInstalada(t *testing.T) {
	machine := windowsInstall("2026.07.23-e383d2b")
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")

	install := detectCursor(machine.probe())

	if !install.Found {
		t.Fatalf("agente não encontrado numa máquina com o Cursor instalado: %+v", install)
	}
	wantNode := filepath.Join(home, "versions", "2026.07.23-e383d2b", "node.exe")
	wantIndex := filepath.Join(home, "versions", "2026.07.23-e383d2b", "index.js")
	if install.Command != wantNode {
		t.Errorf("comando = %q, queria %q", install.Command, wantNode)
	}
	if !slices.Equal(install.Args, []string{wantIndex, "acp"}) {
		t.Errorf("argumentos = %q", install.Args)
	}
	if install.Version != "2026.07.23-e383d2b" {
		t.Errorf("versão = %q", install.Version)
	}
	if install.Source != wantIndex {
		t.Errorf("origem = %q, queria %q", install.Source, wantIndex)
	}
}

func TestDetectCursorWindowsEscolheAVersaoMaisRecente(t *testing.T) {
	// Fora de ordem de propósito: quem escolhe é a versão, não a ordem em que o
	// diretório devolveu os nomes.
	machine := windowsInstall("2026.7.23-e383d2b", "2026.08.01-12-30-45-aaaa111", "2025.12.31-ffff000")

	install := detectCursor(machine.probe())

	if !strings.Contains(install.Command, filepath.Join("versions", "2026.08.01-12-30-45-aaaa111")) {
		t.Fatalf("escolheu a versão errada: %q", install.Command)
	}
	if install.Version != "2026.08.01-12-30-45-aaaa111" {
		t.Errorf("versão = %q", install.Version)
	}
}

func TestDetectCursorWindowsIgnoraDiretorioQueNaoEVersao(t *testing.T) {
	machine := windowsInstall("2026.07.23-e383d2b")
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")
	// Um diretório com nome fora do padrão, e com o par completo dentro: nem
	// assim ele pode ganhar da versão de verdade.
	machine.dirs[filepath.Join(home, "versions")] = append(machine.dirs[filepath.Join(home, "versions")], "rascunho")
	rascunho := filepath.Join(home, "versions", "rascunho")
	machine.files = append(machine.files, filepath.Join(rascunho, "node.exe"), filepath.Join(rascunho, "index.js"))

	install := detectCursor(machine.probe())

	if install.Version != "2026.07.23-e383d2b" {
		t.Fatalf("versão = %q, queria a versão nomeada no padrão", install.Version)
	}
}

func TestDetectCursorWindowsCaiNoWrapperQuandoNaoHaParNodeIndex(t *testing.T) {
	machine := windowsInstall()
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")
	script := filepath.Join(home, "agent.ps1")

	install := detectCursor(machine.probe())

	if !install.Found {
		t.Fatalf("wrapper não encontrado: %+v", install)
	}
	if !strings.EqualFold(filepath.Base(install.Command), "powershell.exe") {
		t.Errorf("comando = %q, queria o PowerShell", install.Command)
	}
	want := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script, "acp"}
	if !slices.Equal(install.Args, want) {
		t.Errorf("argumentos = %q, queria %q", install.Args, want)
	}
	if install.Source != script {
		t.Errorf("origem = %q, queria %q", install.Source, script)
	}
}

func TestDetectCursorWindowsSemPowerShellNaoOfereceOWrapper(t *testing.T) {
	machine := windowsInstall()
	// Sem PowerShell o script não roda; dizer que encontrou faria a pessoa
	// salvar um provider que nunca sobe.
	machine.path = map[string]string{}

	install := detectCursor(machine.probe())

	if install.Found {
		t.Fatalf("ofereceu um wrapper que não tem como executar: %+v", install)
	}
}

func TestDetectCursorWindowsNaoAceitaArquivoDeLoteNoPath(t *testing.T) {
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
		// O PATH resolve para o .cmd que fica ao lado do wrapper, e o Windows
		// não cria processo a partir de arquivo de lote.
		path: map[string]string{"cursor-agent": `C:\Users\alguem\AppData\Local\cursor-agent\cursor-agent.cmd`},
	}

	install := detectCursor(machine.probe())

	if install.Found {
		t.Fatalf("aceitou um arquivo de lote como comando do agente: %+v", install)
	}
}

func TestDetectCursorWindowsAceitaExecutavelNoPath(t *testing.T) {
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
		path: map[string]string{"cursor-agent": `C:\ferramentas\cursor-agent.exe`},
	}

	install := detectCursor(machine.probe())

	if !install.Found || install.Command != `C:\ferramentas\cursor-agent.exe` {
		t.Fatalf("não aceitou o executável do PATH: %+v", install)
	}
	if !slices.Equal(install.Args, []string{"acp"}) {
		t.Errorf("argumentos = %q", install.Args)
	}
}

func TestDetectCursorNoPathEmLinux(t *testing.T) {
	machine := fakeMachine{
		goos: "linux",
		env:  map[string]string{"HOME": "/home/alguem"},
		path: map[string]string{"cursor-agent": "/usr/local/bin/cursor-agent"},
	}

	install := detectCursor(machine.probe())

	if !install.Found {
		t.Fatalf("agente do PATH não encontrado: %+v", install)
	}
	if install.Command != "/usr/local/bin/cursor-agent" {
		t.Errorf("comando = %q", install.Command)
	}
	if !slices.Equal(install.Args, []string{"acp"}) {
		t.Errorf("argumentos = %q", install.Args)
	}
	if install.Version != "" {
		t.Errorf("versão = %q; pelo PATH não há como saber sem executar o agente", install.Version)
	}
}

func TestDetectCursorAchaNoLocalBinQuandoNaoEstaNoPath(t *testing.T) {
	// O app herda o ambiente de quem o abriu, que pode não ter ~/.local/bin no
	// PATH mesmo com o CLI instalado ali.
	//
	// O caminho esperado é montado com filepath.Join como o código monta: este
	// teste roda também no Windows, onde o separador é outro, e escrever a
	// string à mão faria ele falhar pelo sistema de quem executa.
	esperado := filepath.Join("/Users/alguem", ".local", "bin", "cursor-agent")
	machine := fakeMachine{
		goos:  "darwin",
		env:   map[string]string{"HOME": "/Users/alguem"},
		files: []string{esperado},
	}

	install := detectCursor(machine.probe())

	if !install.Found || install.Command != esperado {
		t.Fatalf("não achou o agente em ~/.local/bin: %+v", install)
	}
}

func TestDetectCursorNaoEncontradoDizOndeProcurou(t *testing.T) {
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
	}
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")

	install := detectCursor(machine.probe())

	if install.Found {
		t.Fatalf("encontrou agente numa máquina sem Cursor: %+v", install)
	}
	if install.Command != "" || len(install.Args) != 0 {
		t.Errorf("sugeriu comando sem ter encontrado nada: %+v", install)
	}
	// Sem dizer onde se olhou, "não encontrado" não ajuda quem vai corrigir.
	if !slices.Contains(install.Searched, home) {
		t.Errorf("lugares consultados = %q, queria conter %q", install.Searched, home)
	}
	if !slices.Contains(install.Searched, filepath.Join(home, "versions")) {
		t.Errorf("lugares consultados = %q, queria conter o diretório de versões", install.Searched)
	}
}

func TestDetectCursorNaoRepeteLugarConsultado(t *testing.T) {
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
	}

	install := detectCursor(machine.probe())

	visto := map[string]bool{}
	for _, path := range install.Searched {
		if visto[path] {
			t.Fatalf("lugar repetido na lista de consultados: %q em %q", path, install.Searched)
		}
		visto[path] = true
	}
}

func TestDetectCursorSemLocalAppDataNaoQuebra(t *testing.T) {
	machine := fakeMachine{goos: "windows", env: map[string]string{}}

	install := detectCursor(machine.probe())

	if install.Found {
		t.Fatalf("encontrou agente sem LOCALAPPDATA: %+v", install)
	}
}

func TestDetectCursorGuardaOLugarQueNaoDeuParaConferir(t *testing.T) {
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")
	machine := fakeMachine{
		goos:    "windows",
		env:     map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
		recusas: map[string]error{filepath.Join(home, "versions"): fs.ErrPermission},
	}

	install := detectCursor(machine.probe())

	if install.Found {
		t.Fatalf("encontrou agente onde não deu nem para ler: %+v", install)
	}
	if len(install.Failures) == 0 {
		t.Fatalf("permissão negada não ficou registrada: %+v", install)
	}
	if !strings.Contains(install.Failures[0], filepath.Join(home, "versions")) {
		t.Errorf("falha registrada não diz o lugar: %q", install.Failures[0])
	}
}

func TestDetectCursorNaoTrataAusenciaComoFalha(t *testing.T) {
	// Máquina sem Cursor: nada existe, e nada disso é falha de procura — senão a
	// tela deixaria de dar a instrução de instalar, que é a certa aqui.
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
	}

	install := detectCursor(machine.probe())

	if len(install.Failures) != 0 {
		t.Fatalf("ausência virou falha de procura: %q", install.Failures)
	}
}

func TestDetectAgentNaoConfundeProcuraFalhadaComAgenteAusente(t *testing.T) {
	home := filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent")
	machine := fakeMachine{
		goos:    "windows",
		env:     map[string]string{"LOCALAPPDATA": `C:\Users\alguem\AppData\Local`},
		path:    map[string]string{"powershell": `C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`},
		recusas: map[string]error{filepath.Join(home, "agent.ps1"): fs.ErrPermission},
	}

	_, err := detectAgent(AgentKindCursor, machine.probe())

	if err == nil {
		t.Fatal("procura interrompida por permissão passou como agente não instalado")
	}
	if !strings.Contains(err.Error(), "agent.ps1") {
		t.Errorf("erro não diz o que não deu para conferir: %v", err)
	}
}

func TestDetectAgentSemInstalacaoNaoEErro(t *testing.T) {
	machine := fakeMachine{goos: "linux", env: map[string]string{"HOME": "/home/alguem"}}

	install, err := detectAgent(AgentKindCursor, machine.probe())

	if err != nil {
		t.Fatalf("não achar o CLI virou erro: %v", err)
	}
	if install.Found {
		t.Errorf("encontrou agente numa máquina sem nada: %+v", install)
	}
}

func TestDetectAgentComAgenteEncontradoIgnoraFalhaEmOutroLugar(t *testing.T) {
	// Uma pasta ilegível não importa quando o agente foi achado em outra: já há
	// comando para subir, e reclamar disso seria assustar sem motivo.
	machine := windowsInstall("2026.07.23-e383d2b")
	machine.recusas = map[string]error{filepath.Join(`C:\Users\alguem\AppData\Local`, "cursor-agent", "cursor-agent.ps1"): fs.ErrPermission}

	install, err := detectAgent(AgentKindCursor, machine.probe())

	if err != nil {
		t.Fatalf("falha em lugar irrelevante virou erro: %v", err)
	}
	if !install.Found {
		t.Errorf("não achou o agente instalado: %+v", install)
	}
}

func TestDetectAgentRecusaAgenteDesconhecido(t *testing.T) {
	_, err := DetectAgent(AgentKind("claude-code"))
	if err == nil {
		t.Fatal("aceitou um agente que não tem detecção")
	}
	if !strings.Contains(err.Error(), "claude-code") {
		t.Errorf("erro não nomeia o agente pedido: %v", err)
	}
}

func TestDetectAgentCursorNaMaquinaDoTesteNaoFalha(t *testing.T) {
	// Não afirma nada sobre estar instalado — isso varia por máquina. O que
	// precisa valer sempre é: a detecção do Cursor não é erro, e quando ela diz
	// que encontrou, há comando para subir.
	install, err := DetectAgent(AgentKindCursor)
	if err != nil {
		t.Fatalf("detecção do Cursor devolveu erro: %v", err)
	}
	if install.Found && strings.TrimSpace(install.Command) == "" {
		t.Errorf("disse que encontrou sem comando: %+v", install)
	}
	// Registrado para quem for depurar a detecção nesta máquina: é o que a tela
	// vai mostrar, e a saída do teste é onde isso fica visível.
	t.Logf("detecção nesta máquina: %+v", install)
}
