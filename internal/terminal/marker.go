package terminal

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/rivo/uniseg"
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
	id        string
	startTag  string
	endTag    string
	inlineEnd bool
}

func newCommandMarkerForShell(shell string) *CommandMarker {
	id := uuid.New().String()[:8] // usa só os primeiros 8 chars para markers mais curtos
	return &CommandMarker{
		inlineEnd: shell == "powershell",
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
	start := fmt.Sprintf("Write-Host ([Environment]::NewLine + '%s')", m.startTag)
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
        if ($stage.Extent.Text -ine 'Out-String' -or (Microsoft.PowerShell.Core\Get-Command Out-String).ImplementingType.FullName -ne 'Microsoft.PowerShell.Commands.OutStringCommand') { return $false }
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
	tail := " " + boundary + strings.TrimPrefix(command, body)
	return fmt.Sprintf("%s; try { . (%s ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String('%s'))) {%s} '%s') } catch { Microsoft.PowerShell.Utility\\Write-Error -ErrorRecord $_ -ErrorAction Continue; %s=1 }; Write-Host ('%s' + %s); Microsoft.PowerShell.Utility\\Remove-Variable -Name %s -ErrorAction SilentlyContinue", start, singlePowerShellLine(prepareGroupedPowerShell), encoded, status, strings.ReplaceAll(tail, "'", "''"), code, m.endTag, code, code[1:])
}

// No Windows PowerShell 5.1, parênteses restauram $? para true mesmo após uma
// falha. No pipeline final independente, tire apenas os parênteses externos de
// agrupamento de um comando. O alvo, os argumentos e as subexpressões ficam
// intactos. O script preparado roda uma vez no escopo original, com a leitura
// de $? logo após a operação. O parser trabalha no texto original para que os
// offsets coincidam mesmo quando o comando foi envolvido no try de diretório.
const prepareGroupedPowerShell = `& {
	param($body, $status, $tail);
	$source = $body.ToString();
	$tokens = $null;
	$errors = $null;
	$null = [System.Management.Automation.Language.Parser]::ParseInput($source, [ref]$tokens, [ref]$errors);
	$comment = $tokens | Microsoft.PowerShell.Core\Where-Object { $_.Kind -eq 'Comment' -and $_.Text.StartsWith('#') -and !$source.Substring($_.Extent.EndOffset).Trim() } | Microsoft.PowerShell.Utility\Select-Object -Last 1;
	if ($comment) { $source = $comment.Text + [Environment]::NewLine + $source.Substring(0, $comment.Extent.StartOffset) };
	$source += $tail;
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

// ParseOutput extrai o output e exit code do texto bruto do PTY.
// O parser busca markers apenas no início de linhas para ignorar o echo
// da linha de comando que o PTY produz (onde os markers aparecem embutidos
// dentro do texto do comando, não em linhas próprias).
func (m *CommandMarker) ParseOutput(raw string) ParseResult {
	result := ParseResult{Raw: raw}

	// Remove ANSI escapes para facilitar o parsing
	cleaned := m.cleanOutput(raw)

	// Encontra o start marker no INÍCIO de uma linha (após \n ou no início do texto).
	// Isso ignora ocorrências dentro do echo da linha de comando.
	startIdx := -1
	nlStart := strings.Index(cleaned, "\n"+m.startTag)
	if nlStart != -1 {
		startIdx = nlStart + 1 // pula o \n
	} else if strings.HasPrefix(cleaned, m.startTag) {
		startIdx = 0
	}
	if startIdx == -1 {
		return result
	}

	// Pula o start marker + newline
	contentStart := startIdx + len(m.startTag)
	if contentStart < len(cleaned) && cleaned[contentStart] == '\n' {
		contentStart++
	}

	// Encontra o end marker no início de uma linha (última ocorrência).
	endIdx := -1
	nlEnd := strings.LastIndex(cleaned, "\n"+m.endTag)
	if nlEnd != -1 {
		endIdx = nlEnd + 1 // pula o \n
	} else if strings.HasPrefix(cleaned, m.endTag) {
		endIdx = 0
	}
	if m.inlineEnd {
		// Só o marcador desta execução depois do start real pode fechar o comando.
		// A linha de status completa, validada abaixo, distingue o eco e leituras parciais.
		endIdx = strings.LastIndex(cleaned, m.endTag)
	}
	if endIdx == -1 || endIdx < contentStart {
		return result
	}

	// O PTY pode dividir o próprio código de saída entre duas leituras.
	status, _, complete := strings.Cut(cleaned[endIdx+len(m.endTag):], "\n")
	if !complete {
		return result
	}
	code, err := strconv.Atoi(strings.TrimSpace(status))
	if err != nil {
		return result
	}
	result.ExitCode = code
	result.Found = true

	// Extrai o output entre os markers
	if contentStart < endIdx {
		result.Output = cleaned[contentStart:endIdx]
		// Remove trailing newline
		result.Output = strings.TrimRight(result.Output, "\n")
	}

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

// O próximo chunk pode completar um escape ou um grafema redesenhado.
func (m *CommandMarker) cleanStreamingOutput(raw string) string {
	if len(raw) > 0 && raw[len(raw)-1] >= utf8.RuneSelf {
		start := len(raw) - 1
		for start > 0 && !utf8.RuneStart(raw[start]) {
			start--
		}
		if !utf8.FullRuneInString(raw[start:]) {
			raw = raw[:start]
		}
	}
	if esc := strings.LastIndexByte(raw, '\x1b'); esc >= 0 && partialCSI.MatchString(raw[esc:]) {
		raw = raw[:esc]
	}
	return m.cleanOutput(raw)
}

var conPTYCursorAfterWrap = regexp.MustCompile("\r\n\x1b\\[[0-9]+;([1-9][0-9]*)H")

// Ao rolar a tela, o ConPTY pode redesenhar a última célula da linha:
// CRLF, cursor na última coluna, cópia do grafema e continuação da saída.
// Confira a coluna e o grafema antes de unir as linhas.
func (m *CommandMarker) cleanOutput(raw string) string {
	if m.inlineEnd && strings.Contains(raw, "\r\n\x1b[") {
		var joined strings.Builder
		consumed := 0
		for _, match := range conPTYCursorAfterWrap.FindAllStringSubmatchIndex(raw, -1) {
			before := raw[consumed:match[0]]
			row := StripANSI(before[strings.LastIndexByte(before, '\n')+1:])
			column, err := strconv.Atoi(raw[match[2]:match[3]])
			if err != nil || row == "" {
				continue
			}
			graphemes := uniseg.NewGraphemes(row)
			cells := 0
			var last string
			for graphemes.Next() {
				last = graphemes.Str()
				cells += graphemes.Width()
			}
			if cells != column {
				continue
			}
			replay := raw[match[1]:]
			if len(replay) < len(last) && strings.HasPrefix(last, replay) {
				// A próxima leitura completará o grafema ou revelará uma quebra real.
				joined.WriteString(before)
				consumed = len(raw)
				break
			}
			if !strings.HasPrefix(replay, last) {
				continue
			}
			joined.WriteString(before)
			consumed = match[1] + len(last)
		}
		if consumed > 0 {
			joined.WriteString(raw[consumed:])
			raw = joined.String()
		}
	}
	return strings.ReplaceAll(StripANSI(raw), "\r\n", "\n")
}

// StripANSI remove todas as sequências ANSI escape de uma string.
func StripANSI(s string) string {
	return ansiEscape.ReplaceAllString(s, "")
}
