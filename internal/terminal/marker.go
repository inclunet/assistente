package terminal

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	markerPrefix = "__ASSISTENTE_"
	markerStart  = markerPrefix + "START_"
	markerEnd    = markerPrefix + "END_"
	markerSuffix = "__"
)

// ansiEscape remove sequências ANSI escape do texto.
var ansiEscape = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]|\x1b\].*?\x07|\x1b\[[\?]?[0-9;]*[a-zA-Z]`)

// CommandMarker gera e parseia markers para detectar início/fim de comandos em PTY.
// Cada execução recebe um UUID único para evitar conflitos.
type CommandMarker struct {
	id              string
	startTag        string
	endTag          string
	inlineEnd       bool
	conPTY          bool
	viewportColumns int
}

func newCommandMarkerForShell(shell string) *CommandMarker {
	id := uuid.New().String()[:8] // usa só os primeiros 8 chars para markers mais curtos
	return &CommandMarker{
		inlineEnd: shell == "powershell",
		conPTY:    runtime.GOOS == "windows" && shell == "powershell",
		id:        id,
		startTag:  markerStart + id + markerSuffix,
		endTag:    markerEnd + id + markerSuffix,
	}
}

// WrapCommand envolve um comando com markers de início/fim.
// O shell type determina a sintaxe usada (bash vs powershell).
func (m *CommandMarker) WrapCommand(command, shellType string) string {
	switch shellType {
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return m.wrapPowerShell(command)
	default:
		return m.wrapBash(command)
	}
}

// wrapBash gera comando wrapped para bash/sh/zsh.
func (m *CommandMarker) wrapBash(command string) string {
	// printf em vez de echo para evitar problemas com \n
	// A quebra inicial separa o marker de prompts sem newline (por exemplo sh),
	// inclusive quando o comando foi enviado antes de o shell exibir o prompt.
	return fmt.Sprintf(
		"printf '\\n%%s\\n' '%s'; %s; printf '\\n%%s%%s\\n' '%s' $?",
		m.startTag, command, m.endTag,
	)
}

// wrapPowerShell gera comando wrapped para PowerShell.
func (m *CommandMarker) wrapPowerShell(command string) string {
	// O ConPTY pode colar a saída ao eco ou redesenhar o comando anterior.
	// Só o shell concatena os fragmentos: o eco nunca contém as tags deste comando.
	start := fmt.Sprintf("Write-Host ([Environment]::NewLine + '%s' + '%s')", markerStart, m.id+markerSuffix)
	end := fmt.Sprintf("('%s' + '%s')", markerEnd, m.id+markerSuffix)
	// No Windows PowerShell 5.1, stderr redirecionado pode deixar $? falso
	// mesmo com exit 0. Um NativeCommandError anterior não identifica o
	// comando que acabou de terminar. Delimite o trecho entre a posição do
	// erro e o fim do código do usuário e peça ao parser nativo para conferir.
	boundary := "<#" + m.id + "#>"
	const nativeStderr = `& {
    $e = $Error[0];
    if (!$e -or $e.FullyQualifiedErrorId -notlike 'NativeCommandError*') { return $false };
    $i = $e.InvocationInfo;
    $line = $i.Line;
    if (!$line) { return $false };
    $end = $line.IndexOf('%[1]s');
    $start = $i.OffsetInLine - 1;
    if ($end -lt 0) {
        $full = (Microsoft.PowerShell.Utility\Get-PSCallStack)[0].Position.StartScriptPosition.GetFullScript();
        if (!$full -or $i.ScriptLineNumber -lt 1) { return $false };
        $offset = 0;
        for ($n = 1; $n -lt $i.ScriptLineNumber; $n++) {
            $offset = $full.IndexOf([char]10, $offset) + 1;
            if (!$offset) { return $false }
        };
        $next = $full.IndexOf([char]10, $offset);
        if ($next -lt 0) { $next = $full.Length };
        if ($full.Substring($offset, $next - $offset).TrimEnd([char]13) -cne $line.TrimEnd([char]13, [char]10)) { return $false };
        $line = $full;
        $start += $offset;
        $end = $line.IndexOf('%[1]s')
    };
    if ($start -lt 0 -or $end -le $start) { return $false };
    $tokens = $null;
    $errors = $null;
    $ast = [System.Management.Automation.Language.Parser]::ParseInput($line.Substring($start, $end - $start), [ref]$tokens, [ref]$errors);
    $stmts = $ast.EndBlock.Statements;
    if ($errors.Count -or $stmts.Count -ne 1) { return $false };
    $stages = $stmts[0].PipelineElements;
    if (!$stages.Count -or $stages[0] -isnot [System.Management.Automation.Language.CommandAst]) { return $false };
    for ($n = 1; $n -lt $stages.Count; $n++) {
        $stage = $stages[$n];
        if ($stage -isnot [System.Management.Automation.Language.CommandAst] -or $stage.CommandElements.Count -ne 1 -or $stage.Redirections.Count) { return $false };
        $name = $stage.GetCommandName();
        if ($name -notin 'Out-String','Microsoft.PowerShell.Utility\Out-String' -or (Microsoft.PowerShell.Core\Get-Command -Name $name).ImplementingType.FullName -ne 'Microsoft.PowerShell.Commands.OutStringCommand') { return $false }
    };
    return $true
}`
	// O ConPTY recebe uma única linha de entrada.
	check := fmt.Sprintf(singlePowerShellLine(nativeStderr), boundary)
	// Capture $? dentro do finally. Um return pode deixar o bloco antes do
	// rodapé e o dot-source já terá restaurado $? quando devolver o controle.
	code := "$__asstCode" + m.id
	status := fmt.Sprintf("%s = if ($?) { 0 } elseif ($LASTEXITCODE) { $LASTEXITCODE } elseif ($LASTEXITCODE -eq 0 -and (%s)) { 0 } else { 1 }", code, check)
	// O parser do shell recebe o comando como dado. Assim até uma falha de
	// sintaxe ou comentário no fim da linha deixa a moldura externa executável.
	body := strings.TrimSuffix(command, powershellDirectoryCatch)
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	tail := strings.ReplaceAll(" "+boundary+strings.TrimPrefix(command, body), "'", "''")
	source := fmt.Sprintf("[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))", encoded)
	// A nova linha separa o rodapé até de comentários e do stop-parsing --%.
	source += " + [Environment]::NewLine + '" + tail + "'"
	frame := fmt.Sprintf("'try { ' + %s + ' } finally { ' + {%s}.ToString() + ' }'", source, status)
	script := "[scriptblock]::Create(" + frame + ")"
	// Só o agrupamento final pode exigir a reescrita da AST no PowerShell 5.1.
	// Um comentário após os parênteses também pode ocultar o fim do agrupamento.
	if strings.HasSuffix(strings.TrimRight(body, "; \r\n\t"), ")") || strings.Contains(body, ")") && strings.Contains(body, "#") {
		script = fmt.Sprintf("%s (%s) {%s}", singlePowerShellLine(prepareGroupedPowerShell), source, status)
	}
	return fmt.Sprintf("%s; try { . (%s) } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; %s=1 }; Write-Host (%s + %s); Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction SilentlyContinue", start, script, code, end, code, code[1:])
}

// No Windows PowerShell 5.1, parênteses restauram $? para true mesmo após uma
// falha. No pipeline final independente, tire apenas os parênteses externos de
// agrupamento de um comando. O alvo, os argumentos e as subexpressões ficam
// intactos. O script preparado roda uma vez no escopo original, com a leitura
// de $? logo após a operação. O parser trabalha no texto original para que os
// offsets coincidam mesmo quando o comando foi envolvido no try de diretório.
const prepareGroupedPowerShell = `& {
	param($source, $status);
	$tokens = $null;
	$errors = $null;
	$ast = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
	$last = $ast.EndBlock.Statements[-1];
	if ($last -is [System.Management.Automation.Language.TryStatementAst]) { $last = $last.Body.Statements[-1] };
	$chars = $source.ToCharArray();
	while ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -eq 1) {
		$element = $last.PipelineElements[0];
		if ($element -isnot [System.Management.Automation.Language.CommandExpressionAst]) { break };
		$group = $element.Expression;
		if ($element.Redirections.Count -or $group -isnot [System.Management.Automation.Language.ParenExpressionAst]) { break };
		$chars[$group.Extent.StartOffset] = ' ';
		$chars[$group.Extent.EndOffset - 1] = ' ';
		$last = $group.Pipeline
	};
	if ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) { $source = -join $chars };
	return [scriptblock]::Create('try { ' + $source + ' } finally { ' + $status.ToString() + ' }')
}`

// Preserva espaços dentro de literais do script ao remover o recuo das linhas.
func singlePowerShellLine(script string) string {
	lines := strings.Split(script, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.Join(lines, " ")
}

// ParseResult representa o resultado do parsing de output com markers.
type ParseResult struct {
	// Output é o conteúdo entre os markers (o output real do comando)
	Output string

	// ExitCode é o código de saída extraído do end marker
	ExitCode int

	// Found indica se os markers foram encontrados no output
	Found bool

	// Raw é o output completo não processado
	Raw string
}

// afterStart exige a tag completa e a quebra que precede a saída do comando.
// No ConPTY, a tag montada pelo shell pode aparecer logo após o eco da entrada.
func (m *CommandMarker) afterStart(cleaned string) (string, bool) {
	start := m.startTag + "\n"
	if m.conPTY {
		_, output, ok := strings.Cut(cleaned, start)
		return output, ok
	}
	if output, ok := strings.CutPrefix(cleaned, start); ok {
		return output, true
	}
	_, output, ok := strings.Cut(cleaned, "\n"+start)
	return output, ok
}

// ParseOutput extrai a saída e o código após observar ambos os marcadores.
func (m *CommandMarker) ParseOutput(raw string) ParseResult {
	stream := newMarkerStream(m)
	stream.text.feed(raw)
	stream.text.finish()
	_, result := stream.progress()
	result.Raw = raw
	return result
}

// StartTag retorna o start marker para detecção durante streaming.
func (m *CommandMarker) StartTag() string {
	return m.startTag
}

// EndTag retorna o end marker para detecção durante streaming.
func (m *CommandMarker) EndTag() string {
	return m.endTag
}

var partialCSI = regexp.MustCompile("\x1b(?:\\[[?0-9;]*)?$")

var conPTYResize = regexp.MustCompile("\x1b\\[8;[0-9]+;([0-9]+)t")

// O ConPTY emite CSI 8;linhas;colunas t quando o próprio shell muda o viewport.
func viewportAfterOutput(raw string, columns int) int {
	if !strings.Contains(raw, "\x1b[8;") {
		return columns
	}
	for _, match := range conPTYResize.FindAllStringSubmatch(raw, -1) {
		columns, _ = strconv.Atoi(match[1]) // largura inválida desativa a reconstrução
	}
	return columns
}

func (m *CommandMarker) cleanOutput(raw string) string {
	text := parsedTerminalText{conPTY: m.conPTY, columns: m.viewportColumns}
	text.feed(raw)
	text.finish()
	return string(text.data)
}

// StripANSI remove todas as sequências ANSI escape de uma string.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
