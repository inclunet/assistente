package acp

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// venvFalso monta no disco um venv como o `uv venv` + `uv pip install` o
// deixaria: Python, Scripts/bin e o dist-info com console_scripts. É o que
// permite testar a resolução do ponto de entrada sem baixar nada do PyPI.
func venvFalso(t *testing.T, venvDir, packageName, scriptName, entryPoints string, scriptFiles ...string) {
	t.Helper()
	if err := os.MkdirAll(venvDir, 0o755); err != nil {
		t.Fatalf("não deu para criar o venv falso: %v", err)
	}

	python := filepath.Join(venvDir, "bin", "python")
	scriptsDir := filepath.Join(venvDir, "bin")
	site := filepath.Join(venvDir, "lib", "python3.12", "site-packages")
	if runtime.GOOS == "windows" {
		python = filepath.Join(venvDir, "Scripts", "python.exe")
		scriptsDir = filepath.Join(venvDir, "Scripts")
		site = filepath.Join(venvDir, "Lib", "site-packages")
	}
	if err := os.MkdirAll(filepath.Dir(python), 0o755); err != nil {
		t.Fatalf("não deu para criar o diretório do Python: %v", err)
	}
	if err := os.WriteFile(python, []byte("python"), 0o755); err != nil {
		t.Fatalf("não deu para gravar o Python falso: %v", err)
	}
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		t.Fatalf("não deu para criar Scripts/bin: %v", err)
	}

	distInfo := filepath.Join(site, packageName+"-1.0.0.dist-info")
	if err := os.MkdirAll(distInfo, 0o755); err != nil {
		t.Fatalf("não deu para criar o dist-info: %v", err)
	}
	if entryPoints == "" {
		entryPoints = "[console_scripts]\n" + scriptName + " = pacote.cli:main\n"
	}
	if err := os.WriteFile(filepath.Join(distInfo, "entry_points.txt"), []byte(entryPoints), 0o644); err != nil {
		t.Fatalf("não deu para gravar entry_points.txt: %v", err)
	}

	if len(scriptFiles) == 0 {
		if runtime.GOOS == "windows" {
			scriptFiles = []string{scriptName + ".exe"}
		} else {
			scriptFiles = []string{scriptName}
		}
	}
	for _, name := range scriptFiles {
		caminho := filepath.Join(scriptsDir, name)
		if err := os.WriteFile(caminho, []byte("#!/usr/bin/env python\n"), 0o755); err != nil {
			t.Fatalf("não deu para gravar o script %s: %v", name, err)
		}
	}
}

func TestUVEntryPointLeConsoleScripts(t *testing.T) {
	venv := t.TempDir()
	venvFalso(t, venv, "fast_agent_mcp", "fast-agent", "")

	script, python, err := UVEntryPoint(venv, "fast-agent-mcp")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada: %v", err)
	}
	if filepath.Base(python) != "python" && filepath.Base(python) != "python.exe" {
		t.Errorf("python = %q, queria o Python do venv", python)
	}
	want := "fast-agent"
	if runtime.GOOS == "windows" {
		want = "fast-agent.exe"
	}
	if filepath.Base(script) != want {
		t.Errorf("script = %q, queria %q", script, want)
	}
}

func TestUVEntryPointPrefereOScriptComONomeDoPacote(t *testing.T) {
	venv := t.TempDir()
	entry := "[console_scripts]\naaa-ferramenta = tools.aux:main\nfast-agent = pacote.cli:main\n"
	files := []string{"aaa-ferramenta", "fast-agent"}
	if runtime.GOOS == "windows" {
		files = []string{"aaa-ferramenta.exe", "fast-agent.exe"}
	}
	venvFalso(t, venv, "fast_agent_mcp", "fast-agent", entry, files...)

	script, _, err := UVEntryPoint(venv, "fast-agent-mcp")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada: %v", err)
	}
	base := filepath.Base(script)
	if base != "fast-agent" && base != "fast-agent.exe" {
		t.Errorf("script = %q, queria o que casa com o pacote", script)
	}
}

func TestUVEntryPointComUmScriptSoUsaEsse(t *testing.T) {
	venv := t.TempDir()
	venvFalso(t, venv, "minion_code", "minion",
		"[console_scripts]\nminion = minion.cli:main\n")

	script, _, err := UVEntryPoint(venv, "minion-code")
	if err != nil {
		t.Fatalf("não resolveu o ponto de entrada: %v", err)
	}
	base := filepath.Base(script)
	if base != "minion" && base != "minion.exe" {
		t.Errorf("script = %q, queria minion", script)
	}
}

func TestUVEntryPointRecusaCmdNoWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("recusa de .cmd é regra do Windows (AEP-0084 D15)")
	}
	venv := t.TempDir()
	// Só o .cmd — sem .exe ao lado. Aceitá-lo deixaria o agente como processo
	// neto de um interpretador de lote.
	venvFalso(t, venv, "pacote", "agente",
		"[console_scripts]\nagente = pacote.cli:main\n",
		"agente.cmd")

	if _, _, err := UVEntryPoint(venv, "pacote"); err == nil {
		t.Fatal("aceitou um .cmd como ponto de entrada")
	}
}

func TestFindUVRuntimeReusaDetectRuntime(t *testing.T) {
	machine := fakeMachine{
		goos: "linux",
		path: map[string]string{"uv": "/usr/bin/uv"},
	}
	runtime := findUVRuntime(machine.probe())
	if !runtime.Found || runtime.UV != "/usr/bin/uv" {
		t.Errorf("runtime = %+v, queria o uv do PATH", runtime)
	}
}
