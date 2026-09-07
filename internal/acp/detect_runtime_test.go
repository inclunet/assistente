package acp

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// A procura pelo runtime existe para o catálogo poder nomear o que falta (AEP-0086
// D7). Ela não executa nada, então o que os testes descrevem é sempre um disco e
// um PATH — nunca a resposta de um programa.

func TestDetectRuntimeAchaNodeNoPATH(t *testing.T) {
	machine := fakeMachine{
		goos: "linux",
		path: map[string]string{"node": "/usr/bin/node"},
	}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !install.Found {
		t.Fatal("o Node está no PATH e a procura não o encontrou")
	}
	if install.Path != "/usr/bin/node" {
		t.Errorf("caminho = %q, quer %q", install.Path, "/usr/bin/node")
	}
}

func TestDetectRuntimeIgnoraNodeNaoSpawnavelNoWindows(t *testing.T) {
	// No Windows o PATH pode resolver para um `.cmd` — um lançador, e não o
	// executável. Aceitá-lo faria a tela dizer que o Node está lá e a instalação
	// morrer no primeiro comando.
	prefixo := filepath.Join(`C:\Program Files`, "nodejs")
	machine := fakeMachine{
		goos:  "windows",
		env:   map[string]string{"ProgramFiles": `C:\Program Files`},
		path:  map[string]string{"node": filepath.Join(prefixo, "node.cmd")},
		files: []string{filepath.Join(prefixo, "node.exe")},
	}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if install.Path != filepath.Join(prefixo, "node.exe") {
		t.Errorf("caminho = %q, quer o node.exe do prefixo conhecido", install.Path)
	}
}

func TestDetectRuntimeAchaNodeDoNvmMaisRecenteNoWindows(t *testing.T) {
	// Cada versão do nvm tem o próprio `node_modules` global, e a mais antiga que
	// ficou no disco guarda os pacotes de outra época: escolher por ordem de
	// diretório apontaria para o Node que a pessoa deixou para trás.
	appdata := `C:\Users\alguem\AppData\Roaming`
	nvm := filepath.Join(appdata, "nvm")
	machine := fakeMachine{
		goos: "windows",
		env:  map[string]string{"APPDATA": appdata},
		dirs: map[string][]string{nvm: {"v18.20.4", "v22.11.0", "v9.11.2"}},
		files: []string{
			filepath.Join(nvm, "v18.20.4", "node.exe"),
			filepath.Join(nvm, "v22.11.0", "node.exe"),
			filepath.Join(nvm, "v9.11.2", "node.exe"),
		},
	}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	quer := filepath.Join(nvm, "v22.11.0", "node.exe")
	if install.Path != quer {
		t.Errorf("caminho = %q, quer %q", install.Path, quer)
	}
}

func TestDetectRuntimeAchaNodeDoNvmMaisRecenteEmUnix(t *testing.T) {
	home := "/home/alguem"
	versoes := filepath.Join(home, ".nvm", "versions", "node")
	machine := fakeMachine{
		goos: "linux",
		env:  map[string]string{"HOME": home},
		dirs: map[string][]string{versoes: {"v20.11.1", "v24.3.0"}},
		files: []string{
			filepath.Join(versoes, "v20.11.1", "bin", "node"),
			filepath.Join(versoes, "v24.3.0", "bin", "node"),
		},
	}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	quer := filepath.Join(versoes, "v24.3.0", "bin", "node")
	if install.Path != quer {
		t.Errorf("caminho = %q, quer %q", install.Path, quer)
	}
}

func TestDetectRuntimeSemNodeNaoAchaEContaOndeOlhou(t *testing.T) {
	machine := fakeMachine{goos: "linux", env: map[string]string{"HOME": "/home/alguem"}}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err != nil {
		t.Fatalf("não encontrar não é erro, mas veio erro: %v", err)
	}
	if install.Found {
		t.Fatal("máquina sem Node e a procura diz que achou")
	}
	if len(install.Searched) == 0 {
		t.Error("sem os lugares consultados, \"não encontrado\" não é verificável")
	}
}

func TestDetectRuntimeAchaUVNoDiretorioDoInstalador(t *testing.T) {
	home := "/home/alguem"
	machine := fakeMachine{
		goos:  "darwin",
		env:   map[string]string{"HOME": home},
		files: []string{filepath.Join(home, ".local", "bin", "uv")},
	}

	install, err := detectRuntime(RuntimeUV, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !install.Found {
		t.Fatal("o uv está em ~/.local/bin e a procura não o encontrou")
	}
}

func TestDetectRuntimeAchaUVNoWindowsPeloPerfilDoUsuario(t *testing.T) {
	// O Windows não define HOME, e a procura por diretório é justamente onde o
	// PATH do processo é mais capenga: sem USERPROFILE ela não olharia lugar
	// nenhum.
	perfil := `C:\Users\alguem`
	machine := fakeMachine{
		goos:  "windows",
		env:   map[string]string{"USERPROFILE": perfil},
		files: []string{filepath.Join(perfil, ".local", "bin", "uv.exe")},
	}

	install, err := detectRuntime(RuntimeUV, machine.probe())
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if !install.Found {
		t.Fatal("o uv está no perfil do usuário e a procura não o encontrou")
	}
}

func TestDetectRuntimeRecusaRuntimeDesconhecido(t *testing.T) {
	// O nome vem de quem monta o catálogo, e um runtime que ninguém sabe procurar
	// não pode virar "não encontrado": isso mandaria instalar algo sem nome.
	_, err := detectRuntime(Runtime("bun\nrm -rf"), fakeMachine{goos: "linux"}.probe())
	if err == nil {
		t.Fatal("runtime desconhecido foi aceito")
	}
	if strings.Contains(err.Error(), "\n") {
		t.Errorf("o nome saiu com quebra de linha na mensagem: %q", err.Error())
	}
}

func TestDetectRuntimeSeparaProcuraFalhaDeAusencia(t *testing.T) {
	// Permissão negada é pergunta sem resposta, e tratá-la como ausência mandaria
	// a pessoa instalar o que talvez já esteja lá.
	home := "/home/alguem"
	versoes := filepath.Join(home, ".nvm", "versions", "node")
	machine := fakeMachine{
		goos:    "linux",
		env:     map[string]string{"HOME": home},
		recusas: map[string]error{versoes: errors.New("permissão negada")},
	}

	install, err := detectRuntime(RuntimeNode, machine.probe())
	if err == nil {
		t.Fatal("a procura não pôde ser concluída e nenhum erro voltou")
	}
	if install.Found {
		t.Error("procura que falhou não encontrou nada")
	}
	if len(install.Failures) == 0 {
		t.Error("sem o motivo guardado não há o que dizer a quem vai corrigir")
	}
}
