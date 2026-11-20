package skills

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/osutil"
)

func TestPreprocessCommands_BasicEcho(t *testing.T) {
	var content string
	if runtime.GOOS == "windows" {
		content = "Before\n!echo hello\nAfter"
	} else {
		content = "Before\n!echo hello\nAfter"
	}

	result := renderWithLocalShell(content, []string{"echo"})

	if !strings.Contains(result, "hello") {
		t.Errorf("expected output to contain 'hello', got: %q", result)
	}
	if !strings.Contains(result, "Before") {
		t.Errorf("expected 'Before' to be preserved, got: %q", result)
	}
	if !strings.Contains(result, "After") {
		t.Errorf("expected 'After' to be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_NoCommands(t *testing.T) {
	content := "This is normal content\nNo commands here"
	result := renderWithLocalShell(content, nil)
	if result != content {
		t.Errorf("expected unchanged content, got: %q", result)
	}
}

func TestPreprocessCommands_EmptyContent(t *testing.T) {
	result := renderWithLocalShell("", nil)
	if result != "" {
		t.Errorf("expected empty string, got: %q", result)
	}
}

func TestPreprocessCommands_FailedCommandDoesNotEchoCommand(t *testing.T) {
	failure := errors.New("falha de execução")
	skill := Skill{}
	skill.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: []string{"example"}}}
	output, err := renderCommands(t.Context(), &skill, "!example --token secret-value", false, func(context.Context, *Skill, string) (string, error) { return "private-output", failure }, nil)
	if !errors.Is(err, failure) || output != "" || strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("output=%q err=%v", output, err)
	}
}

func TestPreprocessCommands_BlockedCommand(t *testing.T) {
	content := "!`echo blocked`"
	result := renderWithLocalShell(content, []string{"git", "ls"})
	if !strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("expected blocked error, got: %q", result)
	}
}

func TestPreprocessCommands_AllowedCommand(t *testing.T) {
	content := "!echo allowed"
	result := renderWithLocalShell(content, []string{"echo", "git"})
	if !strings.Contains(result, "allowed") {
		t.Errorf("expected 'allowed' in output, got: %q", result)
	}
	if strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("command should not be blocked, got: %q", result)
	}
}

func TestPreprocessCommands_OnlyExclamationMark(t *testing.T) {
	content := "!\nNormal line"
	result := renderWithLocalShell(content, nil)
	if !strings.Contains(result, "!") {
		t.Errorf("lone '!' should be preserved, got: %q", result)
	}
	if !strings.Contains(result, "Normal line") {
		t.Errorf("'Normal line' should be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_PreservesIndentation(t *testing.T) {
	content := "  Normal indented line\nAnother line"
	result := renderWithLocalShell(content, nil)
	if result != content {
		t.Errorf("indented non-command lines should be preserved, got: %q", result)
	}
}

func TestPreprocessCommands_MultipleCommands(t *testing.T) {
	var content string
	if runtime.GOOS == "windows" {
		content = "!echo first\nMiddle\n!echo second"
	} else {
		content = "!echo first\nMiddle\n!echo second"
	}

	result := renderWithLocalShell(content, []string{"echo"})

	if !strings.Contains(result, "first") {
		t.Errorf("expected 'first' in output, got: %q", result)
	}
	if !strings.Contains(result, "Middle") {
		t.Errorf("expected 'Middle' in output, got: %q", result)
	}
	if !strings.Contains(result, "second") {
		t.Errorf("expected 'second' in output, got: %q", result)
	}
}

func TestPreprocessCommands_BacktickSyntax(t *testing.T) {
	// Formato oficial Claude Code: !`command`
	content := "PR diff:\n!`echo backtick-works`\nDone"
	result := renderWithLocalShell(content, []string{"echo"})

	if !strings.Contains(result, "backtick-works") {
		t.Errorf("expected backtick command output, got: %q", result)
	}
	if strings.Contains(result, "`") {
		t.Errorf("backticks should be stripped from output, got: %q", result)
	}
}

func TestPreprocessCommands_BacktickAndPlain(t *testing.T) {
	// Ambas as sintaxes no mesmo conteúdo
	var content string
	if runtime.GOOS == "windows" {
		content = "!echo plain\n!`echo backtick`"
	} else {
		content = "!echo plain\n!`echo backtick`"
	}
	result := renderWithLocalShell(content, []string{"echo"})

	if !strings.Contains(result, "plain") {
		t.Errorf("expected plain command output, got: %q", result)
	}
	if !strings.Contains(result, "backtick") {
		t.Errorf("expected backtick command output, got: %q", result)
	}
}

func TestStripBackticks(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"`echo hello`", "echo hello"},
		{"echo hello", "echo hello"},
		{"`git log --oneline`", "git log --oneline"},
		{"``", ""},
		{"`", "`"},
		{"", ""},
	}

	for _, tt := range tests {
		result := stripBackticks(tt.input)
		if result != tt.expected {
			t.Errorf("stripBackticks(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

// --- Default-deny e integração com commandpolicy (issue #235) ---

func TestPreprocessCommands_NilAllowlistBlocksAll(t *testing.T) {
	content := "!`echo should-not-run`"
	result := renderWithLocalShell(content, nil)
	if !strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("nil allowlist must block all commands (default-deny), got: %q", result)
	}
	// A linha inteira deve virar o comentário de bloqueio (sem output do echo).
	if !strings.HasPrefix(strings.TrimSpace(result), "comando bloqueado para a skill") {
		t.Errorf("command must not execute with nil allowlist, got: %q", result)
	}
}

func TestPreprocessCommands_EmptyAllowlistBlocksAll(t *testing.T) {
	content := "!`echo should-not-run`"
	result := renderWithLocalShell(content, []string{})
	if !strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("empty allowlist must block all commands (default-deny), got: %q", result)
	}
}

func TestPreprocessCommands_CompositeWithDisallowedPartBlocked(t *testing.T) {
	// "echo" está na allowlist, mas "rm" não: a linha composta inteira deve
	// ser bloqueada (cada átomo precisa ser aprovado pela política).
	content := "!echo hello; rm -rf x"
	result := renderWithLocalShell(content, []string{"echo"})
	if !strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("composite command with disallowed part must be blocked, got: %q", result)
	}

}

func TestPreprocessCommands_BlockedErrorDoesNotEchoCommand(t *testing.T) {
	content := "!echo token=secret-value; rm -rf x"
	result := renderWithLocalShell(content, []string{"echo"})
	if !strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("expected blocked error, got: %q", result)
	}
	if strings.Contains(result, "secret-value") || strings.Contains(result, "rm -rf") {
		t.Errorf("blocked error must not echo the command line or secrets, got: %q", result)
	}
}

func TestPreprocessCommands_CompositeAllAllowedExecutes(t *testing.T) {
	// Composição com && onde todos os átomos são permitidos deve executar.
	content := "!echo first && echo second"
	result := renderWithLocalShell(content, []string{"echo"})
	if strings.Contains(result, "comando bloqueado para a skill") {
		t.Errorf("composite command with all parts allowed should execute, got: %q", result)
	}
	if !strings.Contains(result, "first") || !strings.Contains(result, "second") {
		t.Errorf("expected output of both commands, got: %q", result)
	}
}

func TestPreprocessCommands_ShellFeaturesBlocked(t *testing.T) {
	cases := []struct {
		name    string
		content string
		allowed []string
	}{
		{"pipe", "!echo a | sort", []string{"echo", "sort"}},
		{"redirect output", "!echo a > out.txt", []string{"echo"}},
		{"redirect append", "!echo a >> out.txt", []string{"echo"}},
		{"command substitution", "!echo $(whoami)", []string{"echo"}},
		{"env assignment", "!`FOO=bar echo x`", []string{"echo"}},
		{"background", "!echo a &", []string{"echo"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := renderWithLocalShell(tc.content, tc.allowed)
			if !strings.Contains(result, "comando bloqueado para a skill") {
				t.Errorf("unsupported shell feature must be blocked, got: %q", result)
			}
		})
	}
}

func TestEvaluateSkillCommand_NilPolicyDenies(t *testing.T) {
	allowed, reason := evaluateSkillCommand("echo hello", nil)
	if allowed {
		t.Error("nil policy must deny everything (default-deny)")
	}
	if reason == "" {
		t.Error("expected a non-empty block reason")
	}
}

func TestEvaluateSkillCommand_CaseInsensitive(t *testing.T) {
	policy := buildSkillPolicy([]string{"echo"})
	if allowed, _ := evaluateSkillCommand("ECHO hello", policy); !allowed {
		t.Error("executable matching should be case-insensitive")
	}
}

func TestEvaluateSkillCommand_MatchesExecutable(t *testing.T) {
	policy := buildSkillPolicy([]string{"git", "echo"})
	if allowed, _ := evaluateSkillCommand("git log --oneline -5", policy); !allowed {
		t.Error("allowed executable with args should be approved")
	}
	if allowed, _ := evaluateSkillCommand("rm -rf /", policy); allowed {
		t.Error("non-allowed executable must be denied")
	}
}

func TestBuildSkillPolicy_NilOrEmptyReturnsNil(t *testing.T) {
	if buildSkillPolicy(nil) != nil {
		t.Error("nil allowedCommands should produce nil policy")
	}
	if buildSkillPolicy([]string{}) != nil {
		t.Error("empty allowedCommands should produce nil policy")
	}
	if buildSkillPolicy([]string{"  ", ""}) != nil {
		t.Error("whitespace-only entries should produce nil policy")
	}
}

// Exercise the renderer with a real, test-only shell for the integration cases above.
func renderWithLocalShell(content string, allowed []string) string {
	skill := Skill{}
	skill.Tools = &ToolPermissions{BashCommands: &BashCommands{Allowed: allowed}}
	value, err := renderCommands(context.Background(), &skill, content, false, func(_ context.Context, _ *Skill, cmd string) (string, error) {
		return executeCommand(cmd)
	}, nil)
	if err != nil {
		return err.Error()
	}
	return value
}

// executeCommand executa um comando shell com timeout.
func executeCommand(cmd string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCommandTimeout)
	defer cancel()

	var c *exec.Cmd
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", cmd)
	} else {
		c = exec.CommandContext(ctx, "sh", "-c", cmd)
	}
	// Evita que a janela de console do comando roube o foco no Windows.
	osutil.HideConsoleWindow(c)

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", ctx.Err()
		}
		return "", err
	}

	output := stdout.String()

	// Limita o tamanho do output
	if len(output) > MaxCommandOutputSize {
		output = output[:MaxCommandOutputSize] + "\n<!-- output truncated -->"
	}

	return output, nil
}

func renderCommands(ctx context.Context, skill *Skill, content string, base bool, run SkillCommandRunner, expand func(string) string) (string, error) {
	doc, err := planCommandLines(ctx, skill, content, base, run, expand, "")
	if err != nil {
		return "", err
	}
	return doc.render(ctx, skill, run, base, -1)
}
