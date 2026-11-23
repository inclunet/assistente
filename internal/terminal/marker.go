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
	checkLibrary    bool
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
		m.checkLibrary = true
		return m.wrapPowerShell(command, directory, true)
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

	// CAN cancela qualquer controle VT aberto antes das marcas, inclusive DCS
	// multilinha. O shell emite o byte ao executar printf, depois do eco.
	// A quebra inicial separa o marker de prompts sem newline (por exemplo sh),
	// inclusive quando o comando foi enviado antes de o shell exibir o prompt.
	return fmt.Sprintf(
		"printf '\\030\\n%%s\\n' '%s'; %s eval %s; printf '\\030\\n%%s%%s%s\\n' '%s' $?",
		m.startTag, prefix, quotePOSIXScript(body), markerStatusTerminator, m.endTag,
	)
}

// O tokenizador do PowerShell aceita o conjunto Unicode White_Space entre
// tokens. Só CR, LF e CRLF encerram comentários de linha ou seguem a crase
// numa continuação. Os seletores consultam o parser para confirmar candidatos.
const powerShellSpace = `[\x09-\x0d\x85\p{Z}]`
const powerShellContinuation = `\x60(?:\r\n?|\n)`
const powerShellGap = `(?:` + powerShellSpace + `|` + powerShellContinuation + `|(?s:<#.*?#>))`

// O ponto em modo de argumento pode ser o diretório atual (Get-ChildItem .).
// Somente a posição de comando admite dot-sourcing, com ou sem espaço
// antes de uma variável, expressão ou string. A AST confirma o alvo.
const powerShellDotStatement = `(?:^|[;({|\r\n])` + powerShellGap + `*(?:(?:return|throw|exit|break|continue)` + powerShellGap + `+)?`

// O sinal de igualdade só inicia posição de comando quando pertence a uma
// atribuição de variável; em argumentos como --format=.$x ele é texto literal.
const powerShellAssignment = `(?:^|[;({|\r\n])` + powerShellGap + `*\$(?:\{[^}]+\}|[\pL\pN_:?]+)(?:\[[^]\r\n]*\]|\.[\pL\pN_]+)*` + powerShellGap + `*(?:[+*/%-]?=)` + powerShellGap + `*`
const powerShellForeachSource = `\(` + powerShellGap + `*\$(?:\{[^}]+\}|[\pL\pN_:]+)` + powerShellGap + `+in` + powerShellGap + `+`

var powerShellInvocation = regexp.MustCompile(`(?i)(?:(?:^|[;({|]|` + powerShellGap + `)&|(?:` +
	powerShellDotStatement + `|` + powerShellAssignment + `|` + powerShellForeachSource + `)\.(?:` + powerShellGap + `|[$({"']))`)

// Candidatos após } são conservadores: a AST decide se o ponto é invocação;
// texto ambíguo pode acionar preparação, sem tentar analisar o shell em Go.
// Uma interpolação como "${base}.$ext" também pode selecionar essa rota.
// A AST distingue o dado interpolado de uma invocação após o fim do bloco.
var powerShellAfterBlock = regexp.MustCompile(`}` + powerShellGap + `*\.(?:` + powerShellGap + `|[$({"'])`)
var standalonePowerShellGroup = regexp.MustCompile(`(?i)(?:^|[;\r\n{]|#>)` + powerShellGap + `*(?:return` + powerShellGap + `*)?[$@]?\(`)
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
		computed:  strings.ContainsAny(command, "$(") && (powerShellInvocation.MatchString(command) || powerShellAfterBlock.MatchString(command)),
	}
}

// A biblioteca fica no processo do shell. A entrada interativa contém apenas
// a moldura específica da invocação, inclusive quando PSReadLine está ausente.
func shellStartupArgs(shell string) []string {
	if shellType(shell) != "powershell" {
		return nil
	}
	return []string{"-NoExit", "-Command", powerShellSessionLibrary()}
}

func powerShellSessionLibrary() string {
	const define = `Microsoft.PowerShell.Utility\New-Variable -Scope Global -Option Constant -ErrorAction Stop -Name `
	return define + "AssistenteTerminalStatus -Value " + singlePowerShellLine(failedPowerShellStatus) +
		"; " + define + "AssistenteTerminalPrepare -Value " + singlePowerShellLine(prepareGroupedPowerShell) +
		"; " + define + "AssistenteTerminalTarget -Value " + singlePowerShellLine(powerShellBareTargetExpression)
}

// wrapPowerShell gera comando wrapped para PowerShell.
func (m *CommandMarker) wrapPowerShell(command, directory string, compact bool) string {
	// O ConPTY pode colar a saída ao eco ou redesenhar o comando anterior.
	// Só o shell concatena os fragmentos: o eco nunca contém as tags deste comando.
	start := fmt.Sprintf("Write-Host ([string][char]24 + [Environment]::NewLine + '%s' + '%s')", markerStart, m.id+markerSuffix)
	end := fmt.Sprintf("([string][char]24 + '%s' + '%s')", markerEnd, m.id+markerSuffix)
	// Na falha, o próprio scriptblock compilado identifica o pipeline final e
	// a origem de um NativeCommandError. O caminho de sucesso só lê $?.
	code := "$__asstCode" + m.id
	preparation := selectPowerShellPreparation(command)
	state, setup, cleanup, success := "$null", "", code[1:], "$?"
	if preparation.needed() {
		state = "$__asstState" + m.id
		setup = state + " = @{}; "
		cleanup += "," + state[1:]
		success = "($? -or " + state + "['Ok'] -eq $true) -and " + state + "['Ok'] -ne $false"
	}
	statusHelper, prepareHelper, targetHelper := "& $global:AssistenteTerminalStatus", "& $global:AssistenteTerminalPrepare", "$global:AssistenteTerminalTarget"
	if !compact {
		statusHelper = "& " + singlePowerShellLine(failedPowerShellStatus)
		prepareHelper = "& " + singlePowerShellLine(prepareGroupedPowerShell)
		targetHelper = singlePowerShellLine(powerShellBareTargetExpression)
	}
	status := fmt.Sprintf("%s = if (%s) { 0 } else { %s $MyInvocation.MyCommand.ScriptBlock.Ast %s }", code, success, statusHelper, state)
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
			normalize = targetHelper
		}
		script = fmt.Sprintf("%s (%s) {%s} '%s' '%s' %s", prepareHelper, source, status, state, parseGuard, normalize)
	}
	frame = fmt.Sprintf("%s; %stry { %s. (%s) } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; %s=1 }; Write-Host (%s + %s + '%s'); Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction Ignore", start, setup, parseGuard, script, code, end, code, markerStatusTerminator, cleanup)
	if !compact {
		return frame
	}
	// Um shell filho pode assumir o PTY. A resposta de ausência precede o
	// marcador de início: o corpo autorizado ainda não foi executado.
	return fmt.Sprintf("if (@(Microsoft.PowerShell.Utility\\Get-Variable -Scope Global -Name AssistenteTerminalStatus,AssistenteTerminalPrepare,AssistenteTerminalTarget -ErrorAction Ignore).Count -eq 3) { %s } else { Write-Host (%s + '!') }", frame, end)
}

// A preparação e a leitura da falha precisam identificar a mesma operação.
// Um retorno de valor literal conserva o status da operação precedente.
const powerShellLastOperation = `	$isConstantReturn = {
		param($stmt);
		if ($stmt -isnot [System.Management.Automation.Language.ReturnStatementAst]) { return $false };
		if (!$stmt.Pipeline) { return $true };
		$nodes = [System.Collections.Stack]::new();
		$nodes.Push($stmt.Pipeline);
		while ($nodes.Count) {
			$node = $nodes.Pop();
			if ($node -is [System.Management.Automation.Language.ConstantExpressionAst]) { continue };
			if ($node -is [System.Management.Automation.Language.PipelineAst] -and $node.PipelineElements.Count -eq 1) { $nodes.Push($node.PipelineElements[0]) }
			elseif ($node -is [System.Management.Automation.Language.CommandExpressionAst] -and !$node.Redirections.Count) { $nodes.Push($node.Expression) }
			elseif ($node -is [System.Management.Automation.Language.ParenExpressionAst]) { $nodes.Push($node.Pipeline) }
			elseif ($node -is [System.Management.Automation.Language.ArrayLiteralAst]) { foreach ($item in $node.Elements) { $nodes.Push($item) } }
			elseif (($node -is [System.Management.Automation.Language.SubExpressionAst] -or $node -is [System.Management.Automation.Language.ArrayExpressionAst]) -and !$node.SubExpression.Traps) { foreach ($item in $node.SubExpression.Statements) { $nodes.Push($item) } }
			else { return $false }
		};
		return $true
	};
	$lastInBlock = {
		param($items, $keepReturn);
		$last = $null;
		foreach ($stmt in $items) {
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
					if ($value -is [System.Management.Automation.Language.IfStatementAst]) {
						if ($keepReturn) { $last = $stmt } else { $last = $stmt.Pipeline }
					} elseif ($value -is [System.Management.Automation.Language.PipelineAst] -and $value.PipelineElements.Count -and $value.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) {
						if ($keepReturn) { $last = $stmt } else { $last = $value }
					} elseif (!(& $isConstantReturn $stmt)) { $last = $null }
				};
				break
			};
			$last = $stmt
		};
		return $last
	};
	$last = & $lastInBlock $stmts $false;
	while ($last -is [System.Management.Automation.Language.IfStatementAst] -and $last.Clauses.Count -eq 1 -and !$last.ElseClause) {
		if (!$last.Clauses[0].Item2.Statements.Count) { break };
		$last = & $lastInBlock $last.Clauses[0].Item2.Statements $false
	};
`

// LASTEXITCODE pertence à sessão e sobrevive a cmdlets. Consulte a operação
// antes de atribuir sua falha a um executável. Na saída stderr com status zero,
// confira a posição real do erro e aceite só os formatadores conhecidos.
const failedPowerShellStatus = `{
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
const prepareGroupedPowerShell = `{
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
` + powerShellLastOperation + `	$visit = {
		param($last, $container);
		while ($true) {
			if ($last -is [System.Management.Automation.Language.ReturnStatementAst]) {
				$container = $last;
				$last = $last.Pipeline
			};
			if ($last -isnot [System.Management.Automation.Language.PipelineAst] -or $last.PipelineElements.Count -ne 1) { break };
			$element = $last.PipelineElements[0];
			if ($element -isnot [System.Management.Automation.Language.CommandExpressionAst] -or $element.Redirections.Count) { break };
			$group = $element.Expression;
			if ($group -is [System.Management.Automation.Language.ParenExpressionAst]) {
				$last = $group.Pipeline
			} elseif ($group -is [System.Management.Automation.Language.SubExpressionAst] -or $group -is [System.Management.Automation.Language.ArrayExpressionAst]) {
				if (!$group.SubExpression.Statements.Count) { break };
				$last = & $lastInBlock $group.SubExpression.Statements $true
			} else { break };
			$container = $group
		};
		if ($container -and $last -is [System.Management.Automation.Language.IfStatementAst]) {
			$blocks = @($last.Clauses.Item2) + @($last.ElseClause);
			foreach ($block in $blocks) {
				if (!$block) { continue };
				if (!$block.Statements.Count -or (& $isConstantReturn $block.Statements[0])) {
					[pscustomobject]@{ At = $block.Extent.StartOffset + 1; Text = [Environment]::NewLine + $capture + '.Ok = $true' + [Environment]::NewLine }
				} else { & $visit (& $lastInBlock $block.Statements $true) $last }
			};
			if (!$last.ElseClause) {
				[pscustomobject]@{ At = $last.Extent.EndOffset; Text = ' else { ' + $capture + '.Ok = $true }' }
			};
			return
		};
		if ($last -is [System.Management.Automation.Language.PipelineAst] -and $last.PipelineElements.Count -and $last.PipelineElements[-1] -is [System.Management.Automation.Language.CommandAst]) {
			[pscustomobject]@{ Last = $last; Container = $container }
		}
	};
	$originalLength = $source.Length;
	foreach ($operation in (& $visit $last $null)) {
		$shift = $source.Length - $originalLength;
		if ($operation.PSObject.Properties['Text']) { $source = $source.Insert($shift + $operation.At, $operation.Text); continue };
		$last = $operation.Last;
		$container = $operation.Container;
		$stages = $last.PipelineElements;
		$prefix = '';
		if ($container) {
			if ($container -is [System.Management.Automation.Language.ParenExpressionAst] -or $container -is [System.Management.Automation.Language.ReturnStatementAst]) { $prefix = '$(try { ' };
			$offset = $frame.Length + $shift + $last.Extent.StartOffset + $prefix.Length;
			$record = $capture + '.Ok = $?; ' + $capture + '.Offset = ' + $offset;
			$suffix = if ($prefix) { ' } finally { ' + $record + ' })' } else { '; ' + $record };
			$source = $source.Insert($shift + $last.Extent.EndOffset, [Environment]::NewLine + $suffix + [Environment]::NewLine)
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
			$at = $shift + $target.StartOffset;
			$source = $source.Remove($at, $target.EndOffset - $target.StartOffset).Insert($at, $replacement)
		};
		if ($prefix) { $source = $source.Insert($shift + $last.Extent.StartOffset, $prefix) }
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

// parseResult representa o resultado do parsing de output com markers.
type parseResult struct {
	// Output é o conteúdo entre os markers (o output real do comando)
	Output string

	// ExitCode é o código de saída extraído do end marker
	ExitCode int

	// Found indica se os markers foram encontrados no output
	Found bool

	// Bytes do mesmo lote que chegaram depois do terminador do rodapé.
	afterFooter  string
	footerColumn int
}

// stripANSI remove todas as sequências ANSI escape de uma string.
func stripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
