package plugins

import (
	"errors"
	"strings"
	"unicode/utf8"

	"assistente/internal/subagent"
	"gopkg.in/yaml.v3"
)

var errAgentTooLarge = errors.New("arquivo de agente acima do limite")

const maxClaudeAgentBytes = 32 << 10

// readClaudeAgent maps the supported definition to the native run contract.
// The caller supplies the user's task separately from the package instructions.
func readClaudeAgent(raw []byte) (subagent.RunParams, string, error) {
	var run subagent.RunParams
	invalid := errors.New("definição de agente inválida")
	unsupported := errors.New("configuração de agente sem suporte")
	if len(raw) > maxClaudeAgentBytes {
		return run, "", errAgentTooLarge
	}
	if !utf8.Valid(raw) {
		return run, "", invalid
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) < 3 || lines[0] != "---" {
		return run, "", invalid
	}
	end := 1
	for end < len(lines) && strings.TrimRight(lines[end], " \t") != "---" {
		end++
	}
	if end == len(lines) {
		return run, "", invalid
	}
	var fields map[string]any
	if err := yaml.Unmarshal([]byte(strings.Join(lines[1:end], "\n")), &fields); err != nil {
		return run, "", invalid
	}
	name, _ := fields["name"].(string)
	description, _ := fields["description"].(string)
	run.AgentInstructions = strings.TrimSpace(strings.Join(lines[end+1:], "\n"))
	if strings.TrimSpace(name) == "" {
		return run, "name", invalid
	}
	if strings.TrimSpace(description) == "" {
		return run, "description", invalid
	}
	if run.AgentInstructions == "" {
		return run, "instructions", invalid
	}
	run.Title = name
	for key, value := range fields {
		switch key {
		case "name", "description":
		case "model":
			model, ok := value.(string)
			if !ok || model == "" {
				return run, key, invalid
			}
			if model != "inherit" {
				return run, key, unsupported
			}
			run.Model = model
		case "tools":
			var names []string
			switch list := value.(type) {
			case string:
				names = strings.Split(list, ",")
			case []any:
				for _, item := range list {
					n, ok := item.(string)
					if !ok {
						return run, key, invalid
					}
					names = append(names, n)
				}
			default:
				return run, key, invalid
			}
			run.Tools = make([]string, 0, len(names))
			for _, n := range names {
				var tool string
				switch strings.TrimSpace(n) {
				case "Read":
					tool = "read_file"
				case "Grep":
					tool = "grep_search"
				default:
					return run, key, unsupported
				}
				run.Tools = append(run.Tools, tool)
			}
		default:
			return run, key, unsupported
		}
	}
	return run, "", nil
}
