// Package shelltrust adapta operações shell representáveis ao store comum.
// Comandos livres, compostos ou com argumentos desconhecidos permanecem once.
package shelltrust

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/commandpolicy"
	"assistente/internal/trustscope"
)

type Selector struct {
	Program   string   `json:"program"`
	Arguments []string `json:"arguments"`
	Directory string   `json:"directory"`
}

type Entry struct {
	Selector  Selector         `json:"selector"`
	Scope     trustscope.Scope `json:"scope"`
	CreatedAt time.Time        `json:"created_at"`
}

type Manager struct {
	*trustscope.Manager[Entry, Selector]
}

// As formas são exatas. Não há prefixo livre, argumentos de credenciais,
// nomes de arquivos, env, pipelines, substituições ou valores arbitrários.
var reusableOperations = map[string]bool{
	"git\x00status":            true,
	"git\x00status\x00--short": true,
	"git\x00diff":              true,
	"git\x00diff\x00--stat":    true,
	"git\x00log":               true,
	"git\x00log\x00--oneline":  true,
	"git\x00fetch":             true,
	"git\x00pull":              true,
	"git\x00push":              true,
	"npm\x00test":              true,
	"npm\x00install":           true,
	"npm\x00ci":                true,
	"npm\x00run\x00build":      true,
	"go\x00test":               true,
	"go\x00test\x00./...":      true,
	"go\x00build":              true,
	"go\x00build\x00./...":     true,
}

func operation(program string, args []string) string {
	return strings.Join(append([]string{program}, args...), "\x00")
}
func key(s Selector) string { return operation(s.Program, s.Arguments) + "\x00" + s.Directory }

func NewManager(options trustscope.Options) *Manager {
	return &Manager{trustscope.NewManager(trustscope.Config[Entry, Selector]{
		Domain: "shell", NotFoundError: errors.New("autorização shell não encontrada"),
		Adapter: trustscope.Adapter[Entry, Selector]{
			Scope: func(e Entry) trustscope.Scope { return e.Scope },
			Normalize: func(e Entry) (Entry, error) {
				if !reusableOperations[operation(e.Selector.Program, e.Selector.Arguments)] || !filepath.IsAbs(e.Selector.Directory) {
					return e, errors.New("operação shell não representável")
				}
				if e.CreatedAt.IsZero() {
					e.CreatedAt = time.Now().UTC()
				}
				return e, nil
			},
			Key:       func(e Entry) Selector { return e.Selector },
			Matches:   func(e Entry, s Selector) bool { return key(e.Selector) == key(s) },
			SameKey:   func(e Entry, s Selector) bool { return key(e.Selector) == key(s) },
			Identity:  func(e Entry) string { return key(e.Selector) },
			Effect:    func(Entry) string { return "allow" },
			Operation: func(Entry) string { return "execute" },
		},
	}, options)}
}

// Represent distingue sintaxe analisável de operação que pode ser lembrada.
// O retorno false não nega a tentativa: exige consentimento pontual integral.
func Represent(command, directory string) (Selector, bool) {
	parsed := commandpolicy.Parse(command)
	if parsed.RequiresConfirmation() || len(parsed.Commands) != 1 {
		return Selector{}, false
	}
	cmd := parsed.Commands[0]
	// O parser comum não é um interpretador de cada shell. Só lembrar as
	// formas literais enumeradas, sem colapsar aspas/escapes para outro comando.
	if strings.ContainsAny(command, "\r\n") || strings.Join(strings.Fields(command), "\x00") != operation(cmd.Program, cmd.Args) {
		return Selector{}, false
	}
	if len(cmd.EnvAssignments) != 0 || !reusableOperations[operation(cmd.Program, cmd.Args)] {
		return Selector{}, false
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return Selector{}, false
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return Selector{}, false
	}
	return Selector{Program: cmd.Program, Arguments: append([]string(nil), cmd.Args...), Directory: filepath.Clean(root)}, true
}
