package skills

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// ContainsClaudeCommand reports whether the source contains an executable inline placeholder.
func ContainsClaudeCommand(content string) bool {
	if !strings.Contains(content, "!`") {
		return false
	}
	found := false
	inspectSkillMarkdown(content, func(_, _ int) { found = true }, nil)
	return found
}

// Cada ocorrência tem seu próprio espaço no plano, inclusive na mesma linha.
// A renderização comum aplica orçamento, cancelamento e cache por tentativa.
func planPluginCommands(ctx context.Context, skill *Skill, content string, base bool, run SkillCommandRunner, expand func(string) string, rawArgs string) (commandDocument, error) {
	doc := commandDocument{inline: true}
	literal := func(value string) {
		if expand != nil {
			value = expand(value)
		}
		doc.lines = append(doc.lines, value)
	}
	pos := 0
	var planErr error
	var collect func(start, end int)
	if skill.PackageFormat == "claude-code" && strings.Contains(content, "!`") {
		collect = func(start, end int) {
			if planErr != nil {
				return
			}
			literal(content[pos:start])
			pos = end
			command := content[start+2 : end-1]
			if run == nil {
				if !base {
					planErr = fmt.Errorf("executor de comandos indisponível para a skill %q", skill.Slug)
				}
				return
			}
			if expand != nil {
				if shellArgumentReference.MatchString(command) && !validInterpolatedCommand(rawArgs, command, expand) {
					planErr = fmt.Errorf("argumentos incompatíveis com uma linha de comando da skill %q", skill.Slug)
					return
				}
				command = expand(command)
			}
			if strings.ContainsAny(command, "\r\n") {
				planErr = fmt.Errorf("comando de skill deve ocupar uma linha")
				return
			}
			doc.commands = append(doc.commands, commandLine{len(doc.lines), command})
			doc.lines = append(doc.lines, "")
		}
	}
	_, unclosed := inspectSkillMarkdown(content, collect, nil)
	warnUnclosedPre(ctx, skill.Slug, base, unclosed)
	if planErr != nil {
		return commandDocument{}, planErr
	}
	literal(content[pos:])
	return doc, nil
}

var claudeDirectoryReference = regexp.MustCompile(`\$\{?CLAUDE_(?:PLUGIN_ROOT|PLUGIN_DATA|SKILL_DIR|PROJECT_DIR)\b`)
var claudeMarkdownDirectory = regexp.MustCompile(`\$\{CLAUDE_(?:PLUGIN_ROOT|PLUGIN_DATA|SKILL_DIR|PROJECT_DIR)\}`)

// ClaudeRuntimeRequirement identifies paths and command blocks that need the vendor runtime.
func ClaudeRuntimeRequirement(content string) string {
	if claudeMarkdownDirectory.MatchString(content) {
		return "packagePaths"
	}
	if !strings.Contains(content, "!") {
		return ""
	}
	reason := ""
	inspectSkillMarkdown(content, func(start, end int) {
		if claudeDirectoryReference.MatchString(content[start+2 : end-1]) {
			reason = "packagePaths"
		}
	}, func(info string) {
		if strings.TrimSpace(info) == "!" && reason == "" {
			reason = "shellCommands"
		}
	})
	return reason
}
