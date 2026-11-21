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
var ansiEscape = regexp.MustCompile(
	`\x1b\[[0-?]*[ -/]*[@-~]` + // CSI
		`|\x1b\][^\n]*?(?:\x07|\x1b\\)` + // OSC terminado por BEL ou ST
		`|\x1b[PX^_][^\n]*?\x1b\\` + // demais strings de controle terminadas por ST
		`|\x1b[ -/]+[0-~]` + // ESC com bytes intermediários
		`|\x1b[0-OQ-WYZ\\\x60-~]`, // ESC simples, exceto os introdutores acima
)

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
	return m.wrapCommandInDirectory(command, "", shellType)
}

func (m *CommandMarker) wrapCommandInDirectory(command, directory, shellType string) string {
	switch shellType {
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return m.wrapPowerShell(command, directory)
	default:
		return m.wrapBash(posixCommandInDirectory(command, directory))
	}
}

// O espaço inicial impede que shells tratem o conteúdo como opção de eval.
// As aspas isolam o fim do programa, inclusive para comentários e erros de sintaxe.
func quotePOSIXScript(command string) string {
	return "' " + strings.ReplaceAll(command, "'", "'\"'\"'") + "'"
}

// wrapBash gera comando wrapped para bash/sh/zsh.
func (m *CommandMarker) wrapBash(command string) string {
	// printf em vez de echo para evitar problemas com \n
	// A quebra inicial separa o marker de prompts sem newline (por exemplo sh),
	// inclusive quando o comando foi enviado antes de o shell exibir o prompt.
	return fmt.Sprintf(
		"printf '\\n%%s\\n' '%s'; command eval %s; printf '\\n%%s%%s\\n' '%s' $?",
		m.startTag, quotePOSIXScript(command), m.endTag,
	)
}

// Só expressões usadas como alvo de invocação precisam registrar o valor
// calculado. A AST do shell confirma o candidato antes de alterar o comando.
var computedPowerShellTarget = regexp.MustCompile(`(?:^|[\s;({|])(?:&|\.)\s*[($@"{]`)

// Parênteses em argumentos de um cmdlet não agrupam o pipeline do comando.
var standalonePowerShellGroup = regexp.MustCompile(`(?m)(?:^|[;{]|#>)\s*[$@]?\(`)

// wrapPowerShell gera comando wrapped para PowerShell.
func (m *CommandMarker) wrapPowerShell(command, directory string) string {
	// O ConPTY pode colar a saída ao eco ou redesenhar o comando anterior.
	// Só o shell concatena os fragmentos: o eco nunca contém as tags deste comando.
	start := fmt.Sprintf("Write-Host ([Environment]::NewLine + '%s' + '%s')", markerStart, m.id+markerSuffix)
	end := fmt.Sprintf("('%s' + '%s')", markerEnd, m.id+markerSuffix)
	// Na falha, o próprio scriptblock compilado identifica o pipeline final e
	// a origem de um NativeCommandError. O caminho de sucesso só lê $?.
	code := "$__asstCode" + m.id
	capture, savedTargets, cleanup := "", "$null", code[1:]
	if computedPowerShellTarget.MatchString(command) {
		savedTargets = "$__asstTargets" + m.id
		capture = savedTargets
		cleanup += "," + savedTargets[1:]
	}
	status := fmt.Sprintf("%s = if ($?) { 0 } else { %s $MyInvocation.MyCommand.ScriptBlock.Ast %s }", code, singlePowerShellLine(failedPowerShellStatus), savedTargets)
	body := powerShellDirectoryGuard(directory) + command
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	source := fmt.Sprintf("[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')) + [Environment]::NewLine", encoded)
	frame := fmt.Sprintf("'try { ' + %s + ' } finally { ' + {%s}.ToString() + ' }'", source, status)
	script := "[scriptblock]::Create(" + frame + ")"
	// O sufixo é conferido no texto do usuário, antes da guarda de diretório.
	grouped := standalonePowerShellGroup.MatchString(command) && (strings.HasSuffix(strings.TrimRight(command, "; \r\n\t"), ")") || strings.ContainsAny(command, "#}") || strings.Contains(strings.ToLower(command), "return"))
	if capture != "" || grouped {
		script = fmt.Sprintf("%s (%s) {%s} '%s'", singlePowerShellLine(prepareGroupedPowerShell), source, status, capture)
	}
	return fmt.Sprintf("%s; try { . (%s) } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; %s=1 }; Write-Host (%s + %s); Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction Ignore", start, script, code, end, code, cleanup)
}

// A preparação e a leitura da falha precisam identificar a mesma operação.
// Um retorno de valor literal conserva o status da operação precedente.
const powerShellLastOperation = `	$last = $null;
	foreach ($stmt in $stmts) {
		if ($stmt -is [System.Management.Automation.Language.ReturnStatementAst]) {
			if ($stmt.Pipeline) {
				$value = $stmt.Pipeline;
				while ($value.PipelineElements.Count -eq 1 -and $value.PipelineElements[0] -is [System.Management.Automation.Language.CommandExpressionAst]) {
					$expr = $value.PipelineElements[0].Expression;
					if ($expr -is [System.Management.Automation.Language.ParenExpressionAst]) {
						$value = $expr.Pipeline
					} elseif ($expr -is [System.Management.Automation.Language.SubExpressionAst] -or $expr -is [System.Management.Automation.Language.ArrayExpressionAst]) {
						$value = $expr.SubExpression.Statements[-1]
					} else { break }
				};
				if ($value.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) { $last = $value }
			};
			break
		};
		$last = $stmt
	};
	while ($last -is [System.Management.Automation.Language.IfStatementAst] -and $last.Clauses.Count -eq 1 -and !$last.ElseClause) {
		$last = $last.Clauses[0].Item2.Statements[-1]
	};
`

// LASTEXITCODE pertence à sessão e sobrevive a cmdlets. Consulte a operação
// antes de atribuir sua falha a um executável. Na saída stderr com status zero,
// confira a posição real do erro e aceite só os formatadores conhecidos.
const failedPowerShellStatus = `& {
	param($ast, $targets);
	$stmts = $ast.EndBlock.Statements[0].Body.Statements;
` + powerShellLastOperation + `	$stages = $last.PipelineElements;
	if ($LASTEXITCODE) {
		for ($n = 0; $n -lt $stages.Count; $n++) {
			$stage = $stages[$n];
			if ($stage -isnot [System.Management.Automation.Language.CommandAst]) { continue };
			$target = $stage.GetCommandName();
			if ($targets -and $targets.ContainsKey($n)) { $target = $targets[$n] };
			if (!$target -or $target -is [scriptblock]) { continue };
			$info = if ($target -is [System.Management.Automation.CommandInfo]) { $target } else { $ExecutionContext.InvokeCommand.GetCommand([string]$target, 'All') };
			while ($info -is [System.Management.Automation.AliasInfo]) { $info = $info.ResolvedCommand };
			if ($info.CommandType -in 'Application','ExternalScript') { return $LASTEXITCODE }
		};
		return 1
	};
	$e = $Error[0];
	if ($LASTEXITCODE -ne 0 -or !$e -or $e.FullyQualifiedErrorId -notlike 'NativeCommandError*' -or !$stages.Count) { return 1 };
	$first = $stages[0].Extent.StartScriptPosition;
	$pos = $e.InvocationInfo;
	if ($first.LineNumber -ne $pos.ScriptLineNumber -or $first.ColumnNumber -ne $pos.OffsetInLine -or $first.Line.TrimEnd() -cne $pos.Line.TrimEnd()) { return 1 };
	for ($n = 1; $n -lt $stages.Count; $n++) {
		$stage = $stages[$n];
		if ($stage -isnot [System.Management.Automation.Language.CommandAst] -or $stage.Redirections.Count) { return 1 };
		$argsList = $stage.CommandElements;
		if ($argsList.Count -gt 2) { return 1 };
		if ($argsList.Count -eq 2 -and ($argsList[1] -isnot [System.Management.Automation.Language.CommandParameterAst] -or $argsList[1].ParameterName -ine 'Stream' -or $argsList[1].Argument)) { return 1 };
		$name = $stage.GetCommandName();
		if ($name -notin 'Out-String','Microsoft.PowerShell.Utility\Out-String' -or $ExecutionContext.InvokeCommand.GetCommand($name, 'All').ImplementingType.FullName -ne 'Microsoft.PowerShell.Commands.OutStringCommand') { return 1 }
	};
	return 0
}`

// No PowerShell 5.1, agrupamentos no fim do script ou antes de return podem
// restaurar $? mesmo quando o comando dentro deles falhou. Remova somente os
// delimitadores da expressão independente que termina num pipeline de comando.
const prepareGroupedPowerShell = `& {
	param($source, $status, $capture);
	$tokens = $null;
	$errors = $null;
	$ast = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
	$stmts = $ast.EndBlock.Statements;
` + powerShellLastOperation + `	$chars = $source.ToCharArray();
	while ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -eq 1) {
		$element = $last.PipelineElements[0];
		if ($element -isnot [System.Management.Automation.Language.CommandExpressionAst] -or $element.Redirections.Count) { break };
		$group = $element.Expression;
		if ($group -is [System.Management.Automation.Language.ParenExpressionAst]) {
			$last = $group.Pipeline
		} elseif ($group -is [System.Management.Automation.Language.SubExpressionAst] -or $group -is [System.Management.Automation.Language.ArrayExpressionAst]) {
			if (!$group.SubExpression.Statements.Count) { break };
			$chars[$group.Extent.StartOffset + 1] = ' ';
			$last = $group.SubExpression.Statements[-1]
		} else { break };
		$chars[$group.Extent.StartOffset] = ' ';
		$chars[$group.Extent.EndOffset - 1] = ' '
	};
	if ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) { $source = -join $chars };
	if ($capture) {
		$stages = $last.PipelineElements;
		for ($n = $stages.Count - 1; $n -ge 0; $n--) {
			$stage = $stages[$n];
			if ($stage -isnot [System.Management.Automation.Language.CommandAst] -or $stage.GetCommandName()) { continue };
			$target = $stage.CommandElements[0].Extent;
			$source = $source.Insert($target.EndOffset, ')').Insert($target.StartOffset, '(' + $capture + '[' + $n + '] = ')
		};
		$source = $capture + ' = @{}; ' + $source
	};
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

// StripANSI remove todas as sequências ANSI escape de uma string.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
