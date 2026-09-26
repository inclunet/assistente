package credentials

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zalando/go-keyring"
)

// SourceConfig is independent of the HTTP scheme. Command arguments are never shell-parsed.
type SourceConfig struct {
	Env            string             `json:"env,omitempty"`
	KeyringTarget  string             `json:"keyringTarget,omitempty"`
	KeyringService string             `json:"keyringService,omitempty"`
	KeyringUser    string             `json:"keyringUser,omitempty"`
	Command        string             `json:"command,omitempty"`
	Args           []string           `json:"args,omitempty"`
	TimeoutSeconds int                `json:"timeoutSeconds,omitempty"`
	OAuth          *OAuthSourceConfig `json:"oauth,omitempty"`
}

// OAuthSourceConfig reserves an explicit contract; no interactive flow runs implicitly.
type OAuthSourceConfig struct {
	Issuer   string   `json:"issuer"`
	ClientID string   `json:"clientId"`
	Scopes   []string `json:"scopes,omitempty"`
}

var ErrOAuthSourceUnavailable = errors.New("source oauth ainda não implementada")

func ValidateSource(auth *AuthConfig) error {
	if auth == nil {
		return errors.New("credencial ausente")
	}
	c := auth.SourceConfig
	switch auth.Source {
	case "static":
		if c != nil {
			return errors.New("source static não aceita configuração externa")
		}
	case "env":
		if c == nil || strings.TrimSpace(c.Env) == "" || strings.ContainsAny(c.Env, "=\x00") {
			return errors.New("nome de variável de ambiente obrigatório")
		}
	case "keyring":
		if c == nil {
			return errors.New("configuração de keyring obrigatória")
		}
		target := c.KeyringTarget != ""
		pair := c.KeyringService != "" && c.KeyringUser != ""
		if target == pair || (target && (c.KeyringService != "" || c.KeyringUser != "")) {
			return errors.New("informe target ou service e user do keyring")
		}
	case "command":
		if c == nil || strings.TrimSpace(c.Command) == "" {
			return errors.New("executável obrigatório")
		}
		if c.TimeoutSeconds < 0 || c.TimeoutSeconds > 300 {
			return errors.New("timeout deve estar entre 1 e 300 segundos (0 usa 30)")
		}
	case "oauth":
		return ErrOAuthSourceUnavailable
	default:
		return errors.New("source ausente ou inválida; reconfigure a credencial manualmente")
	}
	return nil
}

// ResolveSource materializes a snapshot. It never persists the resolved secret.
func ResolveSource(ctx context.Context, auth *AuthConfig) (*AuthConfig, error) {
	if err := ValidateSource(auth); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	result := *auth
	if auth.Source == "static" {
		return &result, nil
	}
	c := auth.SourceConfig
	var value string
	var err error
	switch auth.Source {
	case "env":
		value = os.Getenv(c.Env)
		if value == "" {
			return nil, errors.New("variável de ambiente não definida ou vazia")
		}
	case "keyring":
		if c.KeyringTarget != "" {
			value, _, err = lookupKeyringTarget(c.KeyringTarget)
		} else {
			value, err = keyring.Get(c.KeyringService, c.KeyringUser)
		}
		if err != nil {
			return nil, errors.New("não foi possível ler a entrada do keyring")
		}
	case "command":
		value, err = resolveCommand(ctx, c)
		if err != nil {
			return nil, err
		}
	}
	if value == "" {
		return nil, errors.New("source retornou credencial vazia")
	}
	switch auth.Type {
	case "bearer", "secret":
		result.Token = value
	case "basic":
		result.Password = value
	case "custom":
		result.Headers = make(map[string]string, len(auth.Headers))
		if len(auth.Headers) != 1 {
			return nil, errors.New("source dinâmica exige exatamente um header customizado")
		}
		for name := range auth.Headers {
			result.Headers[name] = value
		}
	default:
		return nil, errors.New("scheme incompatível com source dinâmica")
	}
	return &result, nil
}

const maxCommandOutput = 64 * 1024

type limitedSecretOutput struct {
	data     []byte
	exceeded bool
}

func (b *limitedSecretOutput) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > maxCommandOutput {
		b.exceeded = true
		return 0, errors.New("limite de saída excedido")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}

func resolveCommand(ctx context.Context, c *SourceConfig) (string, error) {
	timeout := c.TimeoutSeconds
	if timeout == 0 {
		timeout = 30
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, c.Command, c.Args...)
	configureCredentialCommand(cmd)
	cmd.WaitDelay = time.Second
	output := &limitedSecretOutput{}
	cmd.Stdout = output
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("comando de credencial cancelado ou expirado: %w", ctx.Err())
		}
		return "", errors.New("comando de credencial falhou; verifique o executável e os argumentos")
	}
	if output.exceeded {
		return "", errors.New("comando de credencial excedeu o limite de saída")
	}
	value := strings.TrimSpace(string(output.data))
	if value == "" || strings.ContainsAny(value, "\r\n\x00") {
		return "", errors.New("comando deve retornar uma credencial não vazia em uma única linha")
	}
	return value, nil
}
