package terminal

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/google/uuid"
)

const (
	markerPrefix           = "__ASSISTENTE_"
	markerStart            = markerPrefix + "START_"
	markerEnd              = markerPrefix + "END_"
	markerSuffix           = "__"
	markerStatusTerminator = "!"
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
	control         terminalControl
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

func (m *CommandMarker) wrapCommandInDirectory(command, directory, shellType string) string {
	switch shellType {
	case "powershell", "powershell.exe", "pwsh", "pwsh.exe":
		return m.wrapPowerShell(command, directory)
	default:
		return m.wrapPOSIX(command, directory, shellType)
	}
}

// O espaço inicial impede que shells tratem o conteúdo como opção de eval.
// As aspas isolam o fim do programa, inclusive para comentários e erros de sintaxe.
func quotePOSIXScript(command string) string {
	return "' " + strings.ReplaceAll(command, "'", "'\"'\"'") + "'"
}

func (m *CommandMarker) wrapPOSIX(command, directory, shellType string) string {
	// No zsh nativo, command procura executáveis externos. builtin seleciona
	// os comandos internos sem mudar a opção POSIX_BUILTINS da sessão.
	prefix := "command"
	if shellType == "zsh" {
		prefix = "builtin"
	}
	body := posixCommandInDirectory(command, directory, prefix)

	// printf em vez de echo para evitar problemas com \n
	// A quebra inicial separa o marker de prompts sem newline (por exemplo sh),
	// inclusive quando o comando foi enviado antes de o shell exibir o prompt.
	return fmt.Sprintf(
		"printf '\\n%%s\\n' '%s'; %s eval %s; printf '\\n%%s%%s%s\\n' '%s' $?",
		m.startTag, prefix, quotePOSIXScript(body), markerStatusTerminator, m.endTag,
	)
}

// O tokenizador do PowerShell aceita o conjunto Unicode White_Space entre
// tokens. Só CR, LF e CRLF encerram comentários de linha ou seguem a crase
// numa continuação. Os seletores consultam o parser para confirmar candidatos.
const powerShellSpace = `[\x09-\x0d\x85\p{Z}]`
const powerShellContinuation = `\x60(?:\r\n?|\n)`
const powerShellGap = `(?:` + powerShellSpace + `|` + powerShellContinuation + `|(?s:<#.*?#>))`

var powerShellInvocation = regexp.MustCompile(`(?:^|[;({|]|` + powerShellGap + `)[&.]`)
var standalonePowerShellGroup = regexp.MustCompile(`(?:^|[;\r\n{]|#>)` + powerShellGap + `*[$@]?\(`)
var powerShellGroupEnd = regexp.MustCompile(`\)(?:;|` + powerShellGap + `)*\z`)
var powerShellNamespace = regexp.MustCompile(`(?i)(?:^|[;\r\n]|#>)` + powerShellGap + `*using` + powerShellGap + `+namespace\b`)

// O retorno começa uma instrução. No modo de comando do PowerShell, ponto,
// hífen e cifrão podem integrar o mesmo token; uma fronteira de palavra Go
// também reconheceria nomes como return-value.
var powerShellReturn = regexp.MustCompile(`(?i)(?:^|[;\r\n{]|#>)` + powerShellGap + `*return(?:\z|` + powerShellSpace + `|[\x00&(),;{|}])`)

type powerShellPreparation struct {
	grouped   bool
	namespace bool
	computed  bool
}

func (p powerShellPreparation) needed() bool {
	return p.grouped || p.namespace || p.computed
}

// A decisão usa apenas o texto do usuário, antes da guarda de diretório.
func selectPowerShellPreparation(command string) powerShellPreparation {
	return powerShellPreparation{
		grouped: standalonePowerShellGroup.MatchString(command) &&
			(powerShellGroupEnd.MatchString(command) || strings.ContainsAny(command, "#}") || powerShellReturn.MatchString(command)),
		namespace: powerShellNamespace.MatchString(command),
		computed:  strings.ContainsAny(command, "$(") && powerShellInvocation.MatchString(command),
	}
}

// wrapPowerShell gera comando wrapped para PowerShell.
func (m *CommandMarker) wrapPowerShell(command, directory string) string {
	// O ConPTY pode colar a saída ao eco ou redesenhar o comando anterior.
	// Só o shell concatena os fragmentos: o eco nunca contém as tags deste comando.
	start := fmt.Sprintf("Write-Host ([Environment]::NewLine + '%s' + '%s')", markerStart, m.id+markerSuffix)
	end := fmt.Sprintf("('%s' + '%s')", markerEnd, m.id+markerSuffix)
	// Na falha, o próprio scriptblock compilado identifica o pipeline final e
	// a origem de um NativeCommandError. O caminho de sucesso só lê $?.
	code := "$__asstCode" + m.id
	preparation := selectPowerShellPreparation(command)
	state, setup, cleanup, success := "$null", "", code[1:], "$?"
	if preparation.needed() {
		state = "$__asstState" + m.id
		setup = state + " = @{}; "
		cleanup += "," + state[1:]
		success += " -and " + state + "['Ok'] -ne $false"
	}
	status := fmt.Sprintf("%s = if (%s) { 0 } else { %s $MyInvocation.MyCommand.ScriptBlock.Ast %s }", code, success, singlePowerShellLine(failedPowerShellStatus), state)
	guard := powerShellDirectoryGuard(directory)
	body, parseGuard := guard+command, ""
	if preparation.namespace {
		// O parser recebe o texto original. Confira o diretório antes dessa
		// leitura e novamente antes de executar o corpo compilado.
		body = command
		if guard != "" {
			setup += state + ".EnterDirectory = {" + guard + "}; "
			parseGuard = "& (" + state + ".EnterDirectory); "
		}
	}
	encoded := base64.StdEncoding.EncodeToString([]byte(body))
	source := fmt.Sprintf("[Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s')) + [Environment]::NewLine", encoded)
	frame := fmt.Sprintf("'try { ' + %s + ' } finally { ' + {%s}.ToString() + ' }'", source, status)
	script := "[scriptblock]::Create(" + frame + ")"
	if preparation.needed() {
		normalize := "$null"
		if preparation.computed {
			normalize = singlePowerShellLine(powerShellBareTargetExpression)
		}
		script = fmt.Sprintf("%s (%s) {%s} '%s' '%s' %s", singlePowerShellLine(prepareGroupedPowerShell), source, status, state, parseGuard, normalize)
	}
	return fmt.Sprintf("%s; %stry { %s. (%s) } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; %s=1 }; Write-Host (%s + %s + '%s'); Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction Ignore", start, setup, parseGuard, script, code, end, code, markerStatusTerminator, cleanup)
}

// A preparação e a leitura da falha precisam identificar a mesma operação.
// Um retorno de valor literal conserva o status da operação precedente.
const powerShellLastOperation = `	$last = $null;
	foreach ($stmt in $stmts) {
		if ($stmt -is [System.Management.Automation.Language.ReturnStatementAst]) {
			if ($stmt.Pipeline) {
				$value = $stmt.Pipeline;
				while ($value -is [System.Management.Automation.Language.PipelineAst] -and $value.PipelineElements.Count -eq 1 -and $value.PipelineElements[0] -is [System.Management.Automation.Language.CommandExpressionAst]) {
					$expr = $value.PipelineElements[0].Expression;
					if ($expr -is [System.Management.Automation.Language.ParenExpressionAst]) {
						$value = $expr.Pipeline
					} elseif ($expr -is [System.Management.Automation.Language.SubExpressionAst] -or $expr -is [System.Management.Automation.Language.ArrayExpressionAst]) {
						if (!$expr.SubExpression.Statements.Count) { break };
						$value = $expr.SubExpression.Statements[-1]
					} else { break }
				};
				if ($value -is [System.Management.Automation.Language.PipelineAst] -and $value.PipelineElements.Count -and $value.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) { $last = $value }
			};
			break
		};
		$last = $stmt
	};
	while ($last -is [System.Management.Automation.Language.IfStatementAst] -and $last.Clauses.Count -eq 1 -and !$last.ElseClause) {
		if (!$last.Clauses[0].Item2.Statements.Count) { break };
		$last = $last.Clauses[0].Item2.Statements[-1]
	};
`

// LASTEXITCODE pertence à sessão e sobrevive a cmdlets. Consulte a operação
// antes de atribuir sua falha a um executável. Na saída stderr com status zero,
// confira a posição real do erro e aceite só os formatadores conhecidos.
const failedPowerShellStatus = `& {
	param($ast, $targets);
	$scope = $ast.EndBlock.Statements[0].Body;
	$stmts = $scope.Statements;
` + powerShellLastOperation + `
	if ($targets -and $targets.ContainsKey('Offset')) {
		$last = $ast.Find({ param($node) $node -is [System.Management.Automation.Language.PipelineAst] -and $node.Extent.StartOffset -eq $targets['Offset'] }, $false);
		$scope = $last
	};
	if ($last -isnot [System.Management.Automation.Language.PipelineAst] -or !$ExecutionContext.SessionState.PSVariable.Get('LASTEXITCODE')) { return 1 };
	$start = $last.Extent.StartOffset;
	$end = $last.Extent.EndOffset;
	$earlyReturn = $scope.Find({
		param($node);
		if ($node -isnot [System.Management.Automation.Language.ReturnStatementAst]) { return $false };
		return $node.Extent.EndOffset -le $start -or ($node.Extent.StartOffset -ge $start -and $node.Extent.StartOffset -lt $end)
	}, $false);
	if ($earlyReturn) { return 1 };
	$stages = $last.PipelineElements;
	$n = $stages.Count - 1;
	while ($n -gt 0) {
		$stage = $stages[$n];
		if ($stage -isnot [System.Management.Automation.Language.CommandAst] -or $stage.Redirections.Count) { break };
		$name = $stage.GetCommandName();
		if ($name -notin 'Out-String','Microsoft.PowerShell.Utility\Out-String') { break };
		$argsList = $stage.CommandElements;
		if ($argsList.Count -gt 2) { break };
		if ($argsList.Count -eq 2 -and ($argsList[1] -isnot [System.Management.Automation.Language.CommandParameterAst] -or $argsList[1].ParameterName -ine 'Stream' -or $argsList[1].Argument)) { break };
		$formatter = $ExecutionContext.InvokeCommand.GetCommand($name, 'All');
		if ($formatter -isnot [System.Management.Automation.CmdletInfo] -or $formatter.ImplementingType.FullName -ne 'Microsoft.PowerShell.Commands.OutStringCommand') { break };
		$n--
	};
	if ($LASTEXITCODE) {
		$stage = $stages[$n];
		if ($stage -isnot [System.Management.Automation.Language.CommandAst]) { return 1 };
		$target = $stage.GetCommandName();
		if ($targets -and $targets.ContainsKey($n)) { $target = $targets[$n] };
		if (!$target -or $target -is [scriptblock]) { return 1 };
		$info = if ($target -is [System.Management.Automation.CommandInfo]) { $target } else { $ExecutionContext.InvokeCommand.GetCommand([string]$target, 'All') };
		while ($info -is [System.Management.Automation.AliasInfo]) { $info = $info.ResolvedCommand };
		if ($info -and $info.CommandType -in 'Application','ExternalScript') { return $LASTEXITCODE };
		return 1
	};
	if ($n -ne 0 -or !$Error.Count -or $LASTEXITCODE -ne 0) { return 1 };
	$e = $Error[0];
	if (!$e -or $e.FullyQualifiedErrorId -notlike 'NativeCommandError*') { return 1 };
	$first = $stages[0].Extent.StartScriptPosition;
	$pos = $e.InvocationInfo;
	if ($first.LineNumber -ne $pos.ScriptLineNumber -or $first.ColumnNumber -ne $pos.OffsetInLine -or $first.Line.TrimEnd() -cne $pos.Line.TrimEnd()) { return 1 };
	return 0
}`

// Um nome como cmd$ext é um argumento expansível. Em posição de expressão,
// a mesma grafia poderia invocar um comando. O parser decodifica suas partes
// literais com marcadores no lugar das expansões; só os nós de interpolação
// identificados na AST original voltam como código, uma vez e na ordem original.
const powerShellBareTargetExpression = `{
	param($arg);
	$raw = $arg.Extent.Text;
	$mark = '__asstLiteralZ';
	while ($arg.Value.Contains($mark)) { $mark = '_' + $mark };
	$nodes = $arg.NestedExpressions;
	for ($k = $nodes.Count - 1; $k -ge 0; $k--) {
		$pos = $nodes[$k].Extent;
		$off = $pos.StartOffset - $arg.Extent.StartOffset;
		$raw = $raw.Remove($off, $pos.EndOffset - $pos.StartOffset).Insert($off, $mark)
	};
	$t = $null;
	$err = $null;
	$ast = [System.Management.Automation.Language.Parser]::ParseInput('_ ' + $raw, [ref]$t, [ref]$err);
	if ($err.Count -or $ast.EndBlock.Statements.Count -ne 1) { return };
	$stages = $ast.EndBlock.Statements[0].PipelineElements;
	if ($stages.Count -ne 1 -or $stages[0].CommandElements.Count -ne 2) { return };
	$literal = $stages[0].CommandElements[1];
	if ($literal -isnot [System.Management.Automation.Language.StringConstantExpressionAst]) { return };
	$parts = $literal.Value.Split([string[]]@($mark), [StringSplitOptions]::None);
	if ($parts.Length -ne $nodes.Count + 1) { return };
	$pieces = for ($k = 0; $k -lt $parts.Length; $k++) {
		"'" + [System.Management.Automation.Language.CodeGeneration]::EscapeSingleQuotedStringContent($parts[$k]) + "'";
		if ($k -lt $nodes.Count) { '"' + $nodes[$k].Extent.Text + '"' }
	};
	return '(' + ($pieces -join ' + ') + ')'
}`

// Capture $? antes que o agrupamento o restaure. Preserve os delimitadores:
// @() pode descartar toda a saída acumulada em uma falha terminante.
// () só admite um pipeline; a subexpressão interna comporta seu try/finally.
const prepareGroupedPowerShell = `& {
	param($source, $status, $capture, $guard, $normalize);
	$tokens = $null;
	$errors = $null;
	$ast = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
	if ($errors.Count) { throw [System.Management.Automation.ParseException]::new($errors) };
	$insertAt = 0;
	$frame = 'try { ' + $guard;
	if ($ast.UsingStatements.Count) {
		foreach ($using in $ast.UsingStatements) {
			if ($using.UsingStatementKind -ne 'Namespace') { $insertAt = 0; break };
			$insertAt = $using.Extent.EndOffset
		};
		if ($insertAt) { $frame = [Environment]::NewLine + $frame }
	};
	$stmts = $ast.EndBlock.Statements;
` + powerShellLastOperation + `	$container = $null;
	while ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -eq 1) {
		$element = $last.PipelineElements[0];
		if ($element -isnot [System.Management.Automation.Language.CommandExpressionAst] -or $element.Redirections.Count) { break };
		$group = $element.Expression;
		if ($group -is [System.Management.Automation.Language.ParenExpressionAst]) {
			$last = $group.Pipeline
		} elseif ($group -is [System.Management.Automation.Language.SubExpressionAst] -or $group -is [System.Management.Automation.Language.ArrayExpressionAst]) {
			if (!$group.SubExpression.Statements.Count) { break };
			$last = $group.SubExpression.Statements[-1]
		} else { break };
		$container = $group
	};
	if ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -and $last.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) {
		$stages = $last.PipelineElements;
		$prefix = '';
		if ($container) {
			if ($container -is [System.Management.Automation.Language.ParenExpressionAst]) { $prefix = '$(try { ' };
			$offset = $frame.Length + $last.Extent.StartOffset + $prefix.Length;
			$record = $capture + '.Ok = $?; ' + $capture + '.Offset = ' + $offset;
			$suffix = if ($prefix) { ' } finally { ' + $record + ' })' } else { '; ' + $record };
			$source = $source.Insert($last.Extent.EndOffset, [Environment]::NewLine + $suffix + [Environment]::NewLine)
		};
		for ($n = $stages.Count - 1; $n -ge 0; $n--) {
			$stage = $stages[$n];
			if ($stage -isnot [System.Management.Automation.Language.CommandAst] -or $stage.GetCommandName()) { continue };
			$arg = $stage.CommandElements[0];
			$target = $arg.Extent;
			$text = $target.Text;
			if ($arg -is [System.Management.Automation.Language.ExpandableStringExpressionAst] -and $arg.StringConstantType -eq 'BareWord') {
				if (!$normalize) { continue };
				$text = & $normalize $arg;
				if (!$text) { continue }
			};
			$replacement = '(' + $capture + '[' + $n + '] = ' + $text + ')';
			$source = $source.Remove($target.StartOffset, $target.EndOffset - $target.StartOffset).Insert($target.StartOffset, $replacement)
		};
		if ($prefix) { $source = $source.Insert($last.Extent.StartOffset, $prefix) }
	};
	return [scriptblock]::Create($source.Insert($insertAt, $frame) + ' } finally { ' + $status.ToString() + ' }')
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

// StripANSI remove todas as sequências ANSI escape de uma string.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
