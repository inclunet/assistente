# Persistência de Credenciais

Este documento descreve o sistema de persistência segura de credenciais implementado no assistente.

## Visão Geral

O sistema permite:
- ✅ Armazenamento criptografado de credenciais (tokens, senhas, headers personalizados)
- ✅ Master password protege todas as credenciais
- ✅ Recovery key para recuperação de acesso
- ✅ Integração com keychain do sistema operacional
- ✅ Sem "hints" de senha (segurança por design)
- ✅ Apenas campos sensíveis são criptografados

## Arquitetura

### Componentes

1. **CredentialManager** (`internal/credentials/manager.go`)
   - Gerencia credenciais em memória
   - Resolve credenciais por domínio/padrão
   - Persiste via Store interface

2. **DBStore** (`internal/credentials/db_store.go`)
   - Implementa persistência no banco SQLite
   - Armazena credenciais criptografadas
   - Armazena wraps da DEK (Data Encryption Key)

3. **Master Key** (`internal/credentials/master_key.go`)
   - Deriva chaves usando Argon2id
   - Wrap/unwrap da DEK com AES-256-GCM
   - Geração de recovery keys (48 chars hexadecimal)

4. **Keyring** (`internal/credentials/keyring.go`)
   - Integração com keychain do OS (via go-keyring)
   - Armazena DEK descriptografada para acesso rápido
   - Fallback para DB se keyring não disponível

### Fluxo de Criptografia

```
┌─────────────────────┐
│  Master Password    │
│  (fornecida 1x)     │
└──────────┬──────────┘
           │ Argon2id (32k iterations)
           ↓
    ┌──────────────┐
    │  Derived Key │ (256 bits)
    └──────┬───────┘
           │ AES-256-GCM wrap
           ↓
    ┌──────────────┐
    │     DEK      │ (256 bits, gerada aleatoriamente)
    │ (wrapped)    │ ────→ Salva no DB
    └──────────────┘
           │
           │ Salva descriptografada no keychain
           ↓
    ┌──────────────┐
    │  OS Keychain │
    └──────────────┘
           │
           │ Usada para encrypt/decrypt credenciais
           ↓
    ┌──────────────────────┐
    │  Credenciais (DB)    │
    │  - token (encrypted) │
    │  - password (encr.)  │
    │  - header (encr.)    │
    └──────────────────────┘
```

### Recovery Key

- Gerada automaticamente durante setup
- 48 caracteres hexadecimais (192 bits de entropia)
- Também faz wrap da DEK (salva no DB separadamente)
- Permite recuperação se usuário esquecer master password
- **Deve ser guardada em local seguro offline**

## Primeiro Uso

### Welcome Wizard

1. **Step 0: Master Password**
   - Usuário define senha mestre (mínimo 8 caracteres recomendado)
   - Não há hint de senha por segurança
   - Senha nunca é armazenada

2. **Step 1: Recovery Key**
   - Sistema exibe recovery key gerada
   - Usuário deve copiar e guardar em local seguro
   - Não é possível recuperar depois

3. **Steps seguintes**
   - Configuração de provider (opcional)
   - Outras configurações

### Processo Técnico

```go
// 1. Setup master key (primeira vez)
dek, recoveryKey, err := credentials.SetupMasterKey(masterPassword)

// 2. Salva wraps no DB
store.SaveKeyWrap("master", masterSalt, wrappedDEK, argonParams)
store.SaveKeyWrap("recovery", recoverySalt, wrappedDEKRecovery, argonParams)

// 3. Salva DEK no keychain
keyring.SaveDEK(dek)

// 4. Credenciais podem ser registradas
credMgr.RegisterPattern("*.github.com", &AuthConfig{
    Type:  "bearer",
    Token: "ghp_xxxxx",
})
```

## Uso em Runtime

### Carregamento Automático

```go
// app.go - initToolRegistry()
dek, err := keyring.LoadDEK()
if err != nil {
    // Fallback: pedir master password e unwrap do DB
}

credStore := db_store.NewDBStore(db, dek)
credMgr := credentials.NewManagerWithStore(dek, credStore, true)

// Carrega credenciais persistidas
credMgr.LoadFromStore(ctx)

// Registra env vars (não persistidas)
registerEnvCredentials(credMgr)
```

### HTTPRequest Tool

```go
// Resolve automaticamente credenciais por URL
tool := web.NewHTTPRequest(credMgr)

// Ao fazer request para github.com:
// 1. credMgr.ResolveForURL("https://api.github.com/...")
// 2. Retorna credencial descriptografada
// 3. Adiciona header Authorization
// 4. Modelo NUNCA vê a credencial
```

## Modelos de Dados

### CredentialEntry

```go
type CredentialEntry struct {
    ID            uint      
    Pattern       string    // "*.github.com", "api.openai.com"
    AuthType      string    // "bearer", "basic", "header"
    TokenEnc      []byte    // Encrypted token
    UsernameEnc   []byte    // Encrypted username (basic auth)
    PasswordEnc   []byte    // Encrypted password (basic auth)
    HeaderNameEnc []byte    // Encrypted header name (custom)
    HeaderValEnc  []byte    // Encrypted header value (custom)
    CreatedAt     time.Time
    UpdatedAt     time.Time
}
```

### CredentialKeyWrap

```go
type CredentialKeyWrap struct {
    ID               uint
    Kind             string // "master" ou "recovery"
    Salt             []byte // Random salt (16 bytes)
    WrappedDEK       []byte // DEK wrapped (32 bytes data + 12 nonce + 16 tag)
    Argon2Time       uint32 // 2
    Argon2Memory     uint32 // 64 MB
    Argon2Threads    uint8  // 4
    Argon2KeyLen     uint32 // 32
    CreatedAt        time.Time
}
```

## Recuperação de Acesso

### Usando Recovery Key

```go
// 1. Usuário perdeu master password
// 2. Pede recovery key (48 chars hex)
dek, err := credentials.UnwrapDEK(recoveryKey, keyWrap)

// 3. Salva nova master password (opcional)
newWrap, err := credentials.WrapDEK(dek, newMasterPassword)
store.SaveKeyWrap("master", newSalt, newWrap, params)

// 4. Salva DEK no keychain
keyring.SaveDEK(dek)
```

### Perda Total

Se usuário perder **master password E recovery key**:
- ❌ Não há como recuperar credenciais
- ✅ Pode criar novo setup (perde credenciais antigas)
- ✅ Env vars continuam funcionando (não persistidas)

## Segurança

### O Que É Criptografado

✅ **Criptografado:**
- Tokens de API (GitHub, GitLab, OpenAI, etc)
- Senhas (basic auth)
- Headers personalizados (nome e valor)

❌ **NÃO criptografado:**
- Padrões de domínio (`*.github.com`)
- Tipo de autenticação (`bearer`, `basic`, `header`)
- Metadados (created_at, updated_at)

### Parâmetros Argon2id

- **Time cost:** 2 iterations
- **Memory:** 64 MB
- **Threads:** 4 (parallelismo)
- **Key length:** 32 bytes (256 bits)
- **Salt:** 16 bytes (128 bits) aleatório por wrap

### AES-256-GCM

- **Chave:** 256 bits (DEK)
- **Nonce:** 96 bits (12 bytes) aleatório por operação
- **Tag:** 128 bits (16 bytes) para autenticação
- **Authenticated Encryption:** garante integridade e confidencialidade

## API Backend

### Métodos Principais

```go
// Setup inicial (wizard)
func SetupMasterKey(password string) (dek, recoveryKey []byte, err error)

// Wrap/Unwrap DEK
func WrapDEK(dek, password []byte) (wrapped []byte, salt []byte, err error)
func UnwrapDEK(password, salt, wrapped []byte) (dek []byte, err error)

// Credential Manager
func (m *Manager) RegisterPattern(pattern string, auth *AuthConfig) error
func (m *Manager) RegisterPatternWithContext(ctx context.Context, pattern string, auth *AuthConfig) error
func (m *Manager) ResolveForURL(rawURL string) *AuthConfig
func (m *Manager) LoadFromStore(ctx context.Context) error

// Store operations
func (s *DBStore) SaveCredential(ctx context.Context, cred *StoredCredential) error
func (s *DBStore) ListCredentials(ctx context.Context) ([]*StoredCredential, error)
func (s *DBStore) DeleteCredential(ctx context.Context, pattern string) error

// Keyring
func LoadDEK() ([]byte, error)
func SaveDEK(dek []byte) error
```

### Wizard Flow (app.go)

```go
func (a *App) NeedsWelcomeWizard() bool {
    // Verifica se master key foi configurada
    return !a.credStore.HasKeyWrap(context.Background(), "master")
}

func (a *App) SubmitWelcomeStep(stepIdx int, answer interface{}) WizStepResponse {
    switch stepIdx {
    case 0: // Master password
        password := answer.(string)
        dek, recoveryKey, _ := credentials.SetupMasterKey(password)
        // Salva wraps no DB
        // Salva DEK no keychain
        return WizStepResponse{
            Complete: false,
            NextPrompt: "Guarde este código de recuperação...",
            NextExtra: map[string]interface{}{
                "recoveryKey": hex.EncodeToString(recoveryKey),
            },
        }
    
    case 1: // Recovery key display (só exibir)
        return WizStepResponse{Complete: false}
    
    // ... outros steps
    }
}
```

## Frontend

### QuestionnaireDialog.tsx

```tsx
// Suporta tipo "password"
if (q.type === "password") {
  return (
    <input
      type="password"
      value={answers[idx] || ""}
      onChange={(e) => handleChange(idx, e.target.value)}
      className="..."
      autoComplete="new-password"
    />
  );
}
```

### Wizard Steps

```typescript
// Step 0: Password input
{
  type: "password",
  question: "Defina uma senha mestre para proteger suas credenciais:",
  required: true
}

// Step 1: Recovery key display
{
  type: "text",
  question: "⚠️ IMPORTANTE: Guarde este código de recuperação...",
  extra: { recoveryKey: "abc123..." },
  readonly: true
}
```

## Boas Práticas

### Para Usuários

1. ✅ Use senha forte (mínimo 12+ caracteres, mix de tipos)
2. ✅ Guarde recovery key em gerenciador de senhas ou local seguro offline
3. ✅ Nunca compartilhe master password ou recovery key
4. ❌ Não tire screenshot da recovery key (evite cloud backup automático)

### Para Desenvolvedores

1. ✅ Sempre use `RegisterPatternWithContext` para persistir
2. ✅ Nunca logue credenciais (nem em DEV mode)
3. ✅ DEK nunca sai do processo (não serializar, não enviar)
4. ✅ Zeroize buffers após uso quando possível
5. ✅ Env vars não são persistidas (apenas runtime)

## Troubleshooting

### Keyring não funciona

Se `keyring.LoadDEK()` falhar:
1. Windows: verifica se "Credential Manager" está habilitado
2. macOS: verifica permissões do Keychain
3. Linux: instala `libsecret` ou `gnome-keyring`
4. Fallback: re-pedir master password e unwrap do DB

### Esqueci master password

1. Use recovery key para recuperar acesso
2. Opcionalmente defina nova master password
3. Se não tiver recovery key: reset completo (perde credenciais)

### Banco corrompido

```bash
# Backup antes
cp ~/.config/assistente/assistente.db backup.db

# Reset credenciais
DELETE FROM credential_entries;
DELETE FROM credential_key_wraps;

# Reiniciar wizard
# Reconfigurar credenciais
```

## Testes

### Testes Unitários

```bash
# Testes de master key wrap/unwrap
go test ./internal/credentials -v -run TestWrapUnwrapDEK
go test ./internal/credentials -v -run TestRecoveryKey

# Testes de manager
go test ./internal/credentials -v -run TestManager

# Testes de HTTPRequest com credenciais
go test ./internal/tools/web -v
```

### Coverage

```bash
go test ./internal/credentials -cover
```

## Roadmap

### Futuro (não implementado)

- [ ] Rotação de DEK (gerar nova DEK, re-encrypt tudo)
- [ ] Múltiplos usuários/perfis com senhas diferentes
- [ ] Biometria (Touch ID, Windows Hello)
- [ ] Backup/restore criptografado
- [ ] Auditoria de acesso (quando credencial foi usada)
- [ ] TOTP/2FA para master password (se requisitado)

### Não Planejado

- ❌ Password hints (segurança: evitar social engineering)
- ❌ Recuperação automática (sem recovery key = sem acesso)
- ❌ Sincronização cloud (privacidade: tudo local)

## Referências

- [Argon2 RFC 9106](https://datatracker.ietf.org/doc/html/rfc9106)
- [AES-GCM NIST SP 800-38D](https://csrc.nist.gov/publications/detail/sp/800-38d/final)
- [go-keyring](https://github.com/zalando/go-keyring)
- [OWASP Key Management](https://cheatsheetseries.owasp.org/cheatsheets/Key_Management_Cheat_Sheet.html)
