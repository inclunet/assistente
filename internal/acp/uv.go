package acp

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// UVRuntime é o `uv` desta máquina, procurado sem executar nada — a mesma
// disciplina de `FindNodeRuntime`, e o que o AEP-0086 D7 pede para o runtime
// dos agentes distribuídos por `uvx`.
type UVRuntime struct {
	// Found diz se há `uv` para rodar. Falso não é erro: é o estado que a tela
	// explica, com o requisito em texto (D7).
	Found bool

	// UV é o executável do `uv`, já conferido como spawnável.
	UV string

	// Searched são os lugares consultados, na ordem, para a tela poder dizer
	// onde se procurou quando não se achou nada.
	Searched []string

	// Failures são os lugares que não deu para conferir, já com o motivo.
	Failures []string
}

// FindUVRuntime procura o `uv` nesta máquina.
func FindUVRuntime() UVRuntime {
	return findUVRuntime(systemProbe())
}

// findUVRuntime é o FindUVRuntime com a máquina injetada.
func findUVRuntime(p probe) UVRuntime {
	// É a mesma procura que a tela do catálogo faz por `DetectRuntime`, e de
	// propósito: com duas, a tela diria "uv encontrado" e o instalador diria o
	// contrário na mesma máquina.
	install, _ := detectRuntime(RuntimeUV, p)
	return UVRuntime{
		Found:    install.Found,
		UV:       install.Path,
		Searched: install.Searched,
		Failures: install.Failures,
	}
}

// ErrUVEntryPoint diz que não deu para saber o que rodar de um pacote instalado
// pelo `uv` num venv do app. Quem recebe este erro não tem instalação: um
// provider salvo com comando adivinhado falharia na primeira conversa, longe de
// quem poderia consertá-lo (D8).
var ErrUVEntryPoint = errors.New("ponto de entrada do pacote uv não resolvido")

// maxEntryPointsBytes é o teto de leitura do `entry_points.txt`. O arquivo
// honesto tem algumas dezenas de linhas; o teto existe para um pacote hostil não
// virar memória gasta antes de qualquer validação.
const maxEntryPointsBytes = 1 << 20

// UVEntryPoint resolve o que executar de um pacote instalado em `venvDir` pelo
// `uv pip install` (AEP-0086 D6).
//
// O ponto de entrada sai dos `console_scripts` do `entry_points.txt` do
// `*.dist-info`, e não de um caminho adivinhado. Preferimos o script cujo nome
// casa com o pacote (normalizando `-`/`_`); se só houver um, usamos esse.
//
// O comando gravado é o Python do venv + o script gerado — ou, no Windows, o
// `.exe` em `Scripts/` quando ele existe. `.cmd` e `.bat` são recusados
// (AEP-0084 D15).
func UVEntryPoint(venvDir, packageName string) (scriptPath, pythonPath string, err error) {
	root, err := filepath.Abs(venvDir)
	if err != nil {
		return "", "", fmt.Errorf("%w: venv %q ilegível: %v", ErrUVEntryPoint, venvDir, err)
	}

	pythonPath, err = venvPython(root)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUVEntryPoint, err)
	}

	scriptName, err := consoleScriptForPackage(root, packageName)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUVEntryPoint, err)
	}

	scriptPath, err = locateConsoleScript(root, scriptName)
	if err != nil {
		return "", "", fmt.Errorf("%w: %v", ErrUVEntryPoint, err)
	}
	if !WithinDir(root, scriptPath) {
		return "", "", fmt.Errorf("%w: o script %s aponta para fora da instalação", ErrUVEntryPoint, scriptPath)
	}
	real, err := filepath.EvalSymlinks(scriptPath)
	if err != nil {
		return "", "", fmt.Errorf("%w: o script %s não é um arquivo", ErrUVEntryPoint, scriptPath)
	}
	realRoot := root
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		realRoot = resolved
	}
	if !WithinDir(realRoot, real) {
		return "", "", fmt.Errorf("%w: o script %s aponta para fora da instalação", ErrUVEntryPoint, scriptPath)
	}
	if ok, err := isRegularFile(real); err != nil || !ok {
		return "", "", fmt.Errorf("%w: o script %s não é um arquivo", ErrUVEntryPoint, scriptPath)
	}
	return scriptPath, pythonPath, nil
}

// venvPython acha o Python do venv. São dois layouts, e os dois são do Python,
// não nossos: em Unix o executável fica em `bin/`; no Windows, em `Scripts/`.
func venvPython(venvDir string) (string, error) {
	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{
			filepath.Join(venvDir, "Scripts", "python.exe"),
			filepath.Join(venvDir, "Scripts", "python3.exe"),
		}
	} else {
		candidates = []string{
			filepath.Join(venvDir, "bin", "python"),
			filepath.Join(venvDir, "bin", "python3"),
		}
	}
	for _, candidate := range candidates {
		if ok, err := isRegularFile(candidate); err == nil && ok && Spawnable(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("não há Python spawnável no venv %s", venvDir)
}

// consoleScriptForPackage lê os `console_scripts` dos `*.dist-info` do
// site-packages e escolhe o que corresponde ao pacote.
func consoleScriptForPackage(venvDir, packageName string) (string, error) {
	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		return "", errors.New("sem nome de pacote para resolver o ponto de entrada")
	}

	entries, err := findEntryPointsFiles(venvDir)
	if err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", fmt.Errorf("não há entry_points.txt no venv %s", venvDir)
	}

	var names []string
	seen := map[string]struct{}{}
	for _, entry := range entries {
		scripts, err := readConsoleScripts(entry)
		if err != nil {
			return "", err
		}
		for _, name := range scripts {
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return "", errors.New("o pacote não declara console_scripts, e é deles que sai o ponto de entrada")
	}
	if len(names) == 1 {
		return names[0], nil
	}

	preferred := normalizePackageKey(packageName)
	var matches []string
	for _, name := range names {
		key := normalizePackageKey(name)
		// Casa exato, ou o script é prefixo do pacote (`fast-agent` em
		// `fast-agent-mcp`): o PyPI costuma publicar o executável sem o sufixo
		// do ecossistema.
		if key == preferred || strings.HasPrefix(preferred, key+"_") {
			matches = append(matches, name)
		}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	if len(matches) > 1 {
		// Com mais de um prefixo, o mais específico ganha — `fast_agent` em vez
		// de `fast` quando os dois existiriam.
		best := matches[0]
		for _, name := range matches[1:] {
			if len(normalizePackageKey(name)) > len(normalizePackageKey(best)) {
				best = name
			}
		}
		return best, nil
	}
	slices.Sort(names)
	return "", fmt.Errorf("há mais de um console_script (%s) e nenhum casa com o pacote %s",
		strings.Join(names, ", "), packageName)
}

// findEntryPointsFiles lista os `entry_points.txt` dentro do site-packages do
// venv. O layout muda com a plataforma (`lib/pythonX.Y` vs `Lib`) e com a
// versão do Python; varrer os `*.dist-info` evita adivinhar o caminho.
func findEntryPointsFiles(venvDir string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(venvDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// site-packages é o único lugar onde o pip/uv põe o dist-info; o
			// resto do venv (include, Scripts com lançadores) não interessa e
			// só alongaria a varredura.
			name := d.Name()
			if name == "include" || name == "share" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Name() != "entry_points.txt" {
			return nil
		}
		if !strings.HasSuffix(filepath.Base(filepath.Dir(path)), ".dist-info") {
			return nil
		}
		found = append(found, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("não foi possível varrer o venv %s: %w", venvDir, err)
	}
	return found, nil
}

// readConsoleScripts lê a seção `[console_scripts]` de um entry_points.txt.
func readConsoleScripts(path string) ([]string, error) {
	data, err := readFileAtMost(path, maxEntryPointsBytes)
	if err != nil {
		return nil, fmt.Errorf("não foi possível ler %s: %w", path, err)
	}

	var names []string
	inSection := false
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.EqualFold(line, "[console_scripts]")
			continue
		}
		if !inSection {
			continue
		}
		name, _, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			continue
		}
		names = append(names, name)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%s ilegível: %w", path, err)
	}
	return names, nil
}

// locateConsoleScript acha o arquivo gerado pelo instalador a partir do nome
// do console_script. No Windows o lançador spawnável é o `.exe`; `.cmd`/`.bat`
// existem, e são recusados de propósito (AEP-0084 D15).
func locateConsoleScript(venvDir, scriptName string) (string, error) {
	var dir string
	var candidates []string
	if runtime.GOOS == "windows" {
		dir = filepath.Join(venvDir, "Scripts")
		candidates = []string{
			filepath.Join(dir, scriptName+".exe"),
			filepath.Join(dir, scriptName),
		}
	} else {
		dir = filepath.Join(venvDir, "bin")
		candidates = []string{filepath.Join(dir, scriptName)}
	}
	for _, candidate := range candidates {
		ok, err := isRegularFile(candidate)
		if err != nil || !ok {
			continue
		}
		ext := strings.ToLower(filepath.Ext(candidate))
		if ext == ".cmd" || ext == ".bat" || ext == ".ps1" {
			continue
		}
		// No Windows o script sem extensão não sobe sozinho; o `.exe` ao lado é
		// o que o app consegue spawnar. Preferimos o `.exe` na lista acima.
		if runtime.GOOS == "windows" && !Spawnable(candidate) {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("não há script spawnável %q em %s", scriptName, dir)
}

// normalizePackageKey compara nome de pacote com nome de script ignorando a
// diferença `-`/`_` que o PyPI trata como a mesma coisa.
func normalizePackageKey(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.ReplaceAll(name, "-", "_")
}
