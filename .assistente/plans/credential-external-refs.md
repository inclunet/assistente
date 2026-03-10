# Plano: Referências externas para credenciais (keyring:// e env://)

## Contexto

O Assistente tem um credential manager (`internal/credentials/manager.go`) que armazena tokens criptografados com AES-GCM. Hoje o usuário cola o token literal no campo. O objetivo é permitir que o campo Token/Password contenha uma **referência** a uma fonte externa:

- `keyring://service/user` → busca no OS keyring via `go-keyring` (Windows Credential Manager, macOS Keychain, etc.)
- `env://NOME_DA_VARIAVEL` → busca em `os.Getenv()` no momento da resolução

Isso elimina cópia de tokens e torna tudo sempre fresco.

A UI de credenciais (`frontend/src/pages/CredentialsPage.tsx`) ganha autocomplete: ao digitar `keyring://` ou `env://`, o backend oferece sugestões.

---

## Pré-condições

- `go-keyring` v0.2.2 já é dependência direta (`go.mod`)
- `danieljoos/wincred` v1.1.2 já é dependência indireta (via go-keyring) — usaremos direto para `List()`
- Componente `Combobox` já existe em `frontend/src/components/pickers/Combobox.tsx` (mas NÃO será usado — o campo token é um Input de texto livre que aceita tanto valores literais quanto refs)
- O `decryptAuth()` em `manager.go:335` é o ponto onde valores criptografados viram plaintext — é aqui que refs serão resolvidas

---

## Etapas (atômicas, com checkpoint de verificação)

### ETAPA 1 — Criar `internal/credentials/resolver.go`

Novo arquivo com a lógica de resolução de referências externas.

```go
// internal/credentials/resolver.go
package credentials

import (
	"fmt"
	"os"
	"strings"

	"github.com/zalando/go-keyring"
)

// ResolveExternalRef verifica se value é uma referência externa (keyring:// ou env://)
// e resolve o valor real. Se não for referência, retorna o valor original inalterado.
func ResolveExternalRef(value string) (string, error) {
	switch {
	case strings.HasPrefix(value, "keyring://"):
		return resolveKeyringRef(value)
	case strings.HasPrefix(value, "env://"):
		return resolveEnvRef(value)
	default:
		return value, nil
	}
}

// IsExternalRef retorna true se o valor é uma referência externa.
func IsExternalRef(value string) bool {
	return strings.HasPrefix(value, "keyring://") || strings.HasPrefix(value, "env://")
}

// resolveKeyringRef resolve keyring://service/user
func resolveKeyringRef(ref string) (string, error) {
	// Remove prefixo "keyring://"
	path := strings.TrimPrefix(ref, "keyring://")
	if path == "" {
		return "", fmt.Errorf("referência keyring vazia: %s", ref)
	}

	// Separa service/user no último "/"
	idx := strings.LastIndex(path, "/")
	if idx <= 0 || idx == len(path)-1 {
		return "", fmt.Errorf("referência keyring inválida (esperado keyring://service/user): %s", ref)
	}

	service := path[:idx]
	user := path[idx+1:]

	secret, err := keyring.Get(service, user)
	if err != nil {
		return "", fmt.Errorf("erro ao buscar keyring://%s/%s: %w", service, user, err)
	}
	return secret, nil
}

// resolveEnvRef resolve env://NOME_DA_VARIAVEL
func resolveEnvRef(ref string) (string, error) {
	name := strings.TrimPrefix(ref, "env://")
	if name == "" {
		return "", fmt.Errorf("referência env vazia: %s", ref)
	}

	value := os.Getenv(name)
	if value == "" {
		return "", fmt.Errorf("variável de ambiente %q não definida ou vazia", name)
	}
	return value, nil
}
```

**Checkpoint:** `go build ./internal/credentials/...` compila sem erros.

---

### ETAPA 2 — Criar `internal/credentials/resolver_test.go`

Testes unitários para o resolver.

```go
// internal/credentials/resolver_test.go
package credentials

import (
	"os"
	"testing"
)

func TestResolveExternalRef_LiteralValue(t *testing.T) {
	val, err := ResolveExternalRef("ghp_abc123")
	if err != nil {
		t.Fatal(err)
	}
	if val != "ghp_abc123" {
		t.Errorf("esperado ghp_abc123, obteve %s", val)
	}
}

func TestResolveExternalRef_EmptyString(t *testing.T) {
	val, err := ResolveExternalRef("")
	if err != nil {
		t.Fatal(err)
	}
	if val != "" {
		t.Errorf("esperado string vazia, obteve %q", val)
	}
}

func TestResolveEnvRef_Success(t *testing.T) {
	os.Setenv("TEST_CRED_TOKEN", "my-secret-token")
	defer os.Unsetenv("TEST_CRED_TOKEN")

	val, err := ResolveExternalRef("env://TEST_CRED_TOKEN")
	if err != nil {
		t.Fatal(err)
	}
	if val != "my-secret-token" {
		t.Errorf("esperado my-secret-token, obteve %s", val)
	}
}

func TestResolveEnvRef_Missing(t *testing.T) {
	os.Unsetenv("NONEXISTENT_VAR_XYZ")
	_, err := ResolveExternalRef("env://NONEXISTENT_VAR_XYZ")
	if err == nil {
		t.Error("esperava erro para var não definida")
	}
}

func TestResolveEnvRef_EmptyName(t *testing.T) {
	_, err := ResolveExternalRef("env://")
	if err == nil {
		t.Error("esperava erro para nome vazio")
	}
}

func TestResolveKeyringRef_InvalidFormat(t *testing.T) {
	_, err := resolveKeyringRef("keyring://")
	if err == nil {
		t.Error("esperava erro para ref vazia")
	}
	_, err = resolveKeyringRef("keyring://serviceonly")
	if err == nil {
		t.Error("esperava erro para ref sem user")
	}
}

func TestIsExternalRef(t *testing.T) {
	if !IsExternalRef("keyring://gh:github.com/leo") {
		t.Error("deveria ser external ref")
	}
	if !IsExternalRef("env://GITHUB_TOKEN") {
		t.Error("deveria ser external ref")
	}
	if IsExternalRef("ghp_abc123") {
		t.Error("não deveria ser external ref")
	}
	if IsExternalRef("") {
		t.Error("string vazia não deveria ser external ref")
	}
}
```

**Checkpoint:** `go test ./internal/credentials/... -run "TestResolve|TestIsExternal"` passa (testes de keyring podem falhar em CI sem keyring — ok, os de env passam).

---

### ETAPA 3 — Integrar resolver no `decryptAuth` (manager.go)

Editar `internal/credentials/manager.go`, função `decryptAuth`. Após descriptografar cada campo, chamar `ResolveExternalRef()`.

**Localizar** (linha ~335):
```go
func (m *Manager) decryptAuth(auth *AuthConfig) (*AuthConfig, error) {
```

**Lógica:** Após `m.decrypt(auth.Token)` retornar o plaintext, se esse plaintext for uma ref (`keyring://...` ou `env://...`), resolver. Mesma lógica para Password e Headers.

**Editar o bloco do Token** — mudar de:
```go
	if auth.Token != "" {
		token, err := m.decrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		decrypted.Token = token
	}
```
Para:
```go
	if auth.Token != "" {
		token, err := m.decrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		token, err = ResolveExternalRef(token)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência do token: %w", err)
		}
		decrypted.Token = token
	}
```

**Editar o bloco do Password** — mesma coisa:
```go
	if auth.Password != "" {
		pwd, err := m.decrypt(auth.Password)
		if err != nil {
			return nil, err
		}
		pwd, err = ResolveExternalRef(pwd)
		if err != nil {
			return nil, fmt.Errorf("erro ao resolver referência da senha: %w", err)
		}
		decrypted.Password = pwd
	}
```

**Editar o bloco dos Headers** — mesma coisa no loop:
```go
		for k, v := range auth.Headers {
			decV, err := m.decrypt(v)
			if err != nil {
				return nil, err
			}
			decV, err = ResolveExternalRef(decV)
			if err != nil {
				return nil, fmt.Errorf("erro ao resolver referência do header %s: %w", k, err)
			}
			decrypted.Headers[k] = decV
		}
```

**Checkpoint:** `go build ./internal/credentials/...` compila. Testes existentes continuam passando: `go test ./internal/credentials/...`

---

### ETAPA 4 — Ajustar `summarizeAuth` para mostrar refs sem mascarar

Editar `app.go`, função `summarizeAuth` (~linha 2510). Se o valor descriptografado é uma ref, mostrar a ref (não tem dado sensível). Porém `summarizeAuth` recebe o `*AuthConfig` ANTES da descriptografia (vem direto do `ListCredentials` do manager que já descriptografa).

Na verdade, `ListCredentials()` do manager (linha 164) chama `decryptAuth` que agora resolveria refs — não queremos isso para listagem. Precisamos de um caminho que descriptografe mas NÃO resolva refs.

**Alternativa melhor:** criar `decryptAuthRaw()` que só descriptografa sem resolver, e usar em `ListCredentials`. O `decryptAuth` (usado por `ResolveForURL`) resolve refs.

Adicionar em `manager.go`:
```go
// decryptAuthRaw descriptografa credenciais sem resolver referências externas.
// Usado para listagem/exibição onde queremos ver a ref original (keyring://..., env://...).
func (m *Manager) decryptAuthRaw(auth *AuthConfig) (*AuthConfig, error) {
	if auth == nil {
		return nil, nil
	}
	decrypted := *auth
	if auth.Token != "" {
		token, err := m.decrypt(auth.Token)
		if err != nil {
			return nil, err
		}
		decrypted.Token = token
	}
	if auth.Password != "" {
		pwd, err := m.decrypt(auth.Password)
		if err != nil {
			return nil, err
		}
		decrypted.Password = pwd
	}
	if len(auth.Headers) > 0 {
		decrypted.Headers = make(map[string]string)
		for k, v := range auth.Headers {
			decV, err := m.decrypt(v)
			if err != nil {
				return nil, err
			}
			decrypted.Headers[k] = decV
		}
	}
	return &decrypted, nil
}
```

Alterar `ListCredentials()` (linha ~164) para usar `decryptAuthRaw` em vez de `decryptAuth`.

Alterar `summarizeAuth` em `app.go` para detectar refs e mostrá-las sem mascarar:
```go
func summarizeAuth(auth *credentials.AuthConfig) string {
	if auth == nil {
		return ""
	}
	switch auth.Type {
	case "bearer", "oauth2", "secret":
		if credentials.IsExternalRef(auth.Token) {
			return auth.Token // mostra keyring://... ou env://... sem mascarar
		}
		return maskCredentialValue(auth.Token)
	case "basic":
		if auth.Username == "" && auth.Password == "" {
			return ""
		}
		pwd := maskCredentialValue(auth.Password)
		if credentials.IsExternalRef(auth.Password) {
			pwd = auth.Password
		}
		return fmt.Sprintf("%s:%s", auth.Username, pwd)
	case "custom":
		if len(auth.Headers) == 0 {
			return ""
		}
		keys := make([]string, 0, len(auth.Headers))
		for k := range auth.Headers {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		first := keys[0]
		val := maskCredentialValue(auth.Headers[first])
		if credentials.IsExternalRef(auth.Headers[first]) {
			val = auth.Headers[first]
		}
		return fmt.Sprintf("%s: %s", first, val)
	default:
		return ""
	}
}
```

**Checkpoint:** `go build ./...` compila. `go test ./internal/credentials/...` passa.

---

### ETAPA 5 — Criar endpoint `ListExternalSources` no backend (app.go)

Novo método Wails-bound para o autocomplete da UI.

```go
// ExternalSourceSuggestion representa uma sugestão de fonte externa.
type ExternalSourceSuggestion struct {
	Value string `json:"value"` // ex: "keyring://gh:github.com/leo"
	Label string `json:"label"` // ex: "gh:github.com/leo"
}

// ListExternalSources lista fontes externas disponíveis para autocomplete.
// prefix deve ser "keyring://" ou "env://".
func (a *App) ListExternalSources(prefix string) ([]ExternalSourceSuggestion, error) {
	switch prefix {
	case "keyring://":
		return a.listKeyringEntries()
	case "env://":
		return a.listEnvVars()
	default:
		return []ExternalSourceSuggestion{}, nil
	}
}
```

**Para env://** — simples:
```go
func (a *App) listEnvVars() ([]ExternalSourceSuggestion, error) {
	envs := os.Environ()
	suggestions := make([]ExternalSourceSuggestion, 0, len(envs))
	for _, e := range envs {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) < 2 || parts[1] == "" {
			continue
		}
		name := parts[0]
		// Filtrar vars irrelevantes (system/windows internals)
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "PROCESSOR_") ||
			strings.HasPrefix(upper, "SYSTEM") ||
			strings.HasPrefix(upper, "WINDOWS") ||
			strings.HasPrefix(upper, "COMMON") ||
			upper == "PATH" || upper == "PATHEXT" || upper == "COMSPEC" ||
			upper == "TEMP" || upper == "TMP" || upper == "OS" ||
			upper == "HOMEDRIVE" || upper == "HOMEPATH" ||
			upper == "USERDOMAIN" || upper == "USERNAME" ||
			upper == "LOCALAPPDATA" || upper == "APPDATA" ||
			upper == "PROGRAMFILES" || upper == "PROGRAMDATA" {
			continue
		}
		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: "env://" + name,
			Label: name,
		})
	}
	// Ordenar alfabeticamente
	sort.Slice(suggestions, func(i, j int) bool {
		return suggestions[i].Label < suggestions[j].Label
	})
	return suggestions, nil
}
```

**Para keyring://** — usar `wincred.List()` no Windows (build tag), fallback vazio em outros OS:

Criar arquivo `internal/credentials/keyring_list.go`:
```go
//go:build windows

package credentials

import (
	"fmt"
	"github.com/danieljoos/wincred"
)

// ListKeyringEntries enumera credenciais genéricas do Windows Credential Manager.
// Retorna pares service/user formatados como "keyring://target".
func ListKeyringEntries() ([]KeyringEntry, error) {
	creds, err := wincred.List()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar credenciais do Windows: %w", err)
	}

	entries := make([]KeyringEntry, 0, len(creds))
	for _, c := range creds {
		target := c.TargetName
		if target == "" {
			continue
		}
		entries = append(entries, KeyringEntry{
			Target: target,
			User:   c.UserName,
		})
	}
	return entries, nil
}

// KeyringEntry representa uma entrada do keyring do SO.
type KeyringEntry struct {
	Target string
	User   string
}
```

Criar arquivo `internal/credentials/keyring_list_other.go`:
```go
//go:build !windows

package credentials

// ListKeyringEntries retorna lista vazia em plataformas sem suporte a enumeração.
func ListKeyringEntries() ([]KeyringEntry, error) {
	return []KeyringEntry{}, nil
}

// KeyringEntry representa uma entrada do keyring do SO.
type KeyringEntry struct {
	Target string
	User   string
}
```

> **NOTA:** O tipo `KeyringEntry` precisa ser definido em apenas um lugar acessível em todas as builds. Melhor definir a struct separadamente ou usar build tags corretamente. Solução: definir `KeyringEntry` em `keyring.go` (arquivo sem build tag) e nos arquivos com build tag só ter a função.

**Ajustar:** mover `KeyringEntry` para `keyring.go` e só ter a função `ListKeyringEntries` nos arquivos com build tag.

No `app.go`, a função `listKeyringEntries`:
```go
func (a *App) listKeyringEntries() ([]ExternalSourceSuggestion, error) {
	entries, err := credentials.ListKeyringEntries()
	if err != nil {
		return nil, err
	}

	suggestions := make([]ExternalSourceSuggestion, 0, len(entries))
	for _, e := range entries {
		ref := "keyring://" + e.Target
		suggestions = append(suggestions, ExternalSourceSuggestion{
			Value: ref,
			Label: e.Target,
		})
	}
	return suggestions, nil
}
```

> **NOTA sobre formato do target no wincred:** O `go-keyring` no Windows usa `wincred` com target = `service + ":" + user` (ex: `gh:github.com:leoferreira`). Mas `wincred.List()` retorna o TargetName completo. O nosso URI `keyring://service/user` precisa de service e user separados.
> 
> **Ajuste:** Na verdade o `go-keyring` armazena como `TargetName = service + ":" + user`. Para o autocomplete, podemos listar os targets e transformar em `keyring://target` onde target é o TargetName completo. Na resolução, `keyring://gh:github.com:leoferreira` → precisamos reverter.
> 
> **Simplificação:** em vez de `keyring://service/user`, usar `keyring://target` onde target é o TargetName exato do Windows Credential Manager. Na resolução, fazer `wincred.GetGenericCredential(target)` diretamente em vez de `go-keyring.Get(service, user)`.
> 
> **OU** manter `keyring://service/user` e na listagem, mostrar targets parseados (split no último `:` → service e user). Mas isso é frágil se service contém `:`.
> 
> **Decisão recomendada:** usar `keyring://service/user` com `/` como separador. Na listagem, os targets do wincred são `service:user`, então converter `:` → detectar o padrão. Na prática, para `gh:github.com` + `leoferreira` o target é `gh:github.com:leoferreira`. Parseamos no último `:`.
> 
> Alternativamente: manter a API do `go-keyring` (Get com service + user) e na listagem transformar target `X:Y` em `keyring://X/Y` (split no último `:`).

**Checkpoint:** `go build ./...` compila. `ListExternalSources("env://")` retorna env vars. `ListExternalSources("keyring://")` retorna credenciais do Windows.

---

### ETAPA 6 — Ajustar validação no `UpsertCredential` (app.go)

Editar `app.go`, função `UpsertCredential` (~linha 749). Atualmente exige token não-vazio para bearer/oauth2/secret. Mas se o token for uma ref (`keyring://...` ou `env://...`), não validamos o valor — só a presença.

**Nenhuma mudança necessária** — a validação já exige `strings.TrimSpace(input.Token) == ""`, e refs como `keyring://gh:github.com/leo` não são vazias. Funcionará naturalmente.

**Porém:** refs NÃO devem ser resolvidas no momento do save — apenas no momento do uso (ResolveForURL). O `encryptAuth` vai criptografar a ref como string literal, e no decrypt + resolve, vai descriptografar e depois resolver. ✅ Já funciona naturalmente.

---

### ETAPA 7 — Remover `registerEnvCredentials` (app.go)

**NÃO remover ainda.** Manter como fallback para usuários que já usam env vars hardcoded. Adicionar um TODO comment indicando que pode ser removido quando a migração estiver completa. Ou melhor: manter como está — as env vars diretas continuam funcionando como antes, e o novo esquema `env://` é uma alternativa para credenciais cadastradas pela UI.

**Decisão:** MANTER `registerEnvCredentials` sem alteração nesta etapa.

---

### ETAPA 8 — Frontend: Campo Token com sugestões de referências externas

Editar `frontend/src/pages/CredentialsPage.tsx`.

**8a. Importar `ListExternalSources` dos bindings Wails:**

Após rodar `wails generate`, o binding estará em `frontend/wailsjs/go/main/App.js`. Importar:
```ts
import { ListCredentials, UpsertCredential, DeleteCredential, ListExternalSources } from '@wailsjs/go/main/App';
```

**8b. Adicionar state para sugestões:**
```ts
const [suggestions, setSuggestions] = useState<Array<{value: string; label: string}>>([]);
const [showSuggestions, setShowSuggestions] = useState(false);
```

**8c. Função para buscar sugestões quando o valor do token muda:**
```ts
const handleTokenChange = async (value: string) => {
  crud.updateField('token', value);
  
  if (value === 'keyring://' || value === 'env://') {
    try {
      const results = await ListExternalSources(value);
      setSuggestions(results || []);
      setShowSuggestions(true);
    } catch {
      setSuggestions([]);
      setShowSuggestions(false);
    }
  } else if (value.startsWith('keyring://') || value.startsWith('env://')) {
    // Filtrar sugestões já carregadas
    const prefix = value.startsWith('keyring://') ? 'keyring://' : 'env://';
    const search = value.slice(prefix.length).toLowerCase();
    const filtered = suggestions.filter(s => 
      s.label.toLowerCase().includes(search)
    );
    setSuggestions(filtered);
    setShowSuggestions(filtered.length > 0);
  } else {
    setShowSuggestions(false);
    setSuggestions([]);
  }
};
```

**8d. Substituir o Input de Token por um wrapper com lista de sugestões:**

Trocar o bloco:
```tsx
{(crud.editingItem.type === 'bearer' || crud.editingItem.type === 'oauth2' || crud.editingItem.type === 'secret') && (
  <Input
    label="Token"
    type="password"
    value={crud.editingItem.token || ''}
    onChange={(e) => crud.updateField('token', e.target.value)}
    placeholder="Informe o token"
    fullWidth
  />
)}
```

Por:
```tsx
{(crud.editingItem.type === 'bearer' || crud.editingItem.type === 'oauth2' || crud.editingItem.type === 'secret') && (
  <div className="credentials-page__token-field">
    <Input
      label="Token"
      type={isRefValue(crud.editingItem.token) ? 'text' : 'password'}
      value={crud.editingItem.token || ''}
      onChange={(e) => handleTokenChange(e.target.value)}
      placeholder="Token, keyring://service/user ou env://VAR"
      fullWidth
      autoComplete="off"
    />
    {showSuggestions && suggestions.length > 0 && (
      <ul className="credentials-page__suggestions" role="listbox" aria-label="Sugestões de referência">
        {suggestions.map((s) => (
          <li
            key={s.value}
            role="option"
            className="credentials-page__suggestion"
            onMouseDown={(e) => {
              e.preventDefault();
              crud.updateField('token', s.value);
              setShowSuggestions(false);
            }}
          >
            {s.label}
          </li>
        ))}
      </ul>
    )}
  </div>
)}
```

Onde `isRefValue`:
```ts
function isRefValue(value?: string): boolean {
  return Boolean(value && (value.startsWith('keyring://') || value.startsWith('env://')));
}
```

**8e. Fechar sugestões quando o modal fecha:**

No `useEffect` ou no close handler, resetar:
```ts
// Ao fechar modal, resetar sugestões
setSuggestions([]);
setShowSuggestions(false);
```

**8f. CSS para a lista de sugestões** — adicionar em `CredentialsPage.css`:
```css
.credentials-page__token-field {
  position: relative;
}

.credentials-page__suggestions {
  position: absolute;
  z-index: 10;
  top: 100%;
  left: 0;
  right: 0;
  max-height: 200px;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 0;
  background: var(--bg-secondary, #2d2d2d);
  border: 1px solid var(--border-color, #444);
  border-radius: 4px;
  box-shadow: 0 4px 8px rgba(0,0,0,0.3);
}

.credentials-page__suggestion {
  padding: 8px 12px;
  cursor: pointer;
  font-size: 0.9em;
}

.credentials-page__suggestion:hover,
.credentials-page__suggestion:focus {
  background: var(--bg-hover, #3d3d3d);
}
```

**8g. Ajustar validação no frontend** — no `validate` do `useEditableList`, se token é ref, aceitar:
```ts
if (item.type === 'bearer' || item.type === 'oauth2' || item.type === 'secret') {
  if (!item.token) {
    return 'Token é obrigatório';
  }
}
```
Essa validação já funciona — ref não é string vazia. Nenhuma mudança necessária.

**Checkpoint:** `npm run build` no frontend compila sem erros. UI mostra sugestões ao digitar `keyring://` ou `env://` no campo token.

---

### ETAPA 9 — Gerar bindings Wails

Rodar `wails generate` para que `ListExternalSources` e `ExternalSourceSuggestion` apareçam em `frontend/wailsjs/go/main/App.js` e `App.d.ts`.

**Checkpoint:** arquivo `frontend/wailsjs/go/main/App.d.ts` contém `ListExternalSources`.

---

### ETAPA 10 — Testes end-to-end manuais

1. Abrir app, ir em Credenciais
2. Criar nova credencial:
   - Pattern: `*.github.com`
   - Tipo: Bearer token
   - Token: digitar `env://` → ver sugestões de env vars
   - Selecionar `env://GITHUB_TOKEN` (se existir)
   - Salvar
3. Na lista, coluna "Valor" deve mostrar `env://GITHUB_TOKEN` (sem mascarar)
4. Testar `http_request` para `https://api.github.com/user` → deve autenticar via env var
5. Repetir com `keyring://gh:github.com/leoferreira` se gh CLI estiver instalado

---

## Resumo dos arquivos tocados

| Arquivo | Ação |
|---------|------|
| `internal/credentials/resolver.go` | **CRIAR** — lógica de resolução keyring:// e env:// |
| `internal/credentials/resolver_test.go` | **CRIAR** — testes do resolver |
| `internal/credentials/keyring.go` | **EDITAR** — adicionar struct `KeyringEntry` |
| `internal/credentials/keyring_list.go` | **CRIAR** — ListKeyringEntries (build tag: windows) |
| `internal/credentials/keyring_list_other.go` | **CRIAR** — stub para não-windows |
| `internal/credentials/manager.go` | **EDITAR** — decryptAuth chama ResolveExternalRef; adicionar decryptAuthRaw; ListCredentials usa decryptAuthRaw |
| `app.go` | **EDITAR** — novo ListExternalSources, ExternalSourceSuggestion; ajustar summarizeAuth |
| `frontend/src/pages/CredentialsPage.tsx` | **EDITAR** — campo token com sugestões |
| `frontend/src/pages/CredentialsPage.css` | **EDITAR** — estilos da lista de sugestões |

---

## Riscos e mitigações

1. **wincred.List() pode ser lento** — Cache por 30s no app ou lazy load. Mitigação: só chama quando usuário digita `keyring://`.
2. **Separador service/user no target** — O go-keyring usa `service:user` como target no Windows. Parse no último `:` funciona para maioria dos casos. Se necessário, podemos usar o `wincred` direto pra obter service e user separados.
3. **Env vars sensíveis na lista** — Filtramos vars de sistema. O usuário vê apenas vars relevantes.
4. **Cross-platform** — keyring list só funciona no Windows (via wincred). macOS/Linux retornam lista vazia (stub). `env://` funciona em todos.
