# Plano de Centralização do Cliente HTTP

**Status**: In Progress
**Objetivo**: Unificar todas as instâncias de `http.Client` sob o cliente HTTP centralizado (`internal/tools/http/client.go`)  
**Benefícios**: Eliminação de duplicação, auth centralizada, retry automático, manutenção simplificada

---

## 📊 Situação Atual

### ✅ Já Refatoradas (Fase 1-3)
- **`internal/tools/web/http_request.go`** - Ferramenta HTTP completa
- **`internal/tools/web/web_fetch.go`** - Busca e extração de URLs
- **`internal/tools/http/signal.go`** - Adapter Signal
- **`internal/tools/web/web_search.go`** - Busca web (DuckDuckGo)
- **`internal/speech/openai_tts.go`** - Text-to-Speech OpenAI
- **`internal/speech/openai_whisper.go`** - Speech-to-Text OpenAI
- **`internal/speech/speech_manager.go`** - Gerenciador de speech

### ⏳ Pendentes de Refatoração

- `internal/messaging/telegram/adapter.go` ainda usa `http.Get`.
- Providers e conexão ainda criam clientes próprios em `internal/providers/`.
- OAuth/discovery MCP mantém clientes próprios em `internal/mcp/`.
- `internal/llm/google_provider.go` e `internal/auth/external_jwks.go` ainda
  possuem clientes `net/http` diretos.

Esses caminhos podem exigir transporte ou ciclo de vida próprio, mas precisam
ser migrados ou explicitamente delimitados antes de concluir esta AEP.

---

## 🎯 Fases de Implementação

### Fase 2: Web Tools (Prioridade Alta)

#### 2.1. **`web_search.go`** - Busca Web
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/tools/web/web_search.go`
- **Responsabilidade**: Busca com DuckDuckGo
- **Cliente Anterior**: Criava `http.Client` próprio (linhas 41, 51)
- **Mudanças Realizadas**:
  - ✅ Adicionados imports `httpclient` e `credentials`
  - ✅ Removido import `time`
  - ✅ Struct `WebSearch` mudada para usar `*httpclient.Client`
  - ✅ Interface `SearchProvider` atualizada para aceitar `*httpclient.Client`
  - ✅ `NewWebSearch()` refatorado para aceitar `credMgr` e criar cliente centralizado
  - ✅ `NewWebSearchWithProvider()` também atualizado
  - ✅ Chamadas `client.Do()` atualizadas para `client.Do(ctx, req)`
  - ✅ Provider `duckDuckGoProvider` atualizado
  - ✅ Instanciação em `app.go` atualizada
  - ✅ Todos os testes atualizados e passando
- **Complexidade**: ⭐⭐ (média - interface SearchProvider foi refatorada com sucesso)

---

### Fase 3: Serviços de Fala (Prioridade Alta)

#### 3.1. **`openai_tts.go`** - Text-to-Speech
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/speech/openai_tts.go`
- **Responsabilidade**: Síntese de voz OpenAI
- **Mudanças Realizadas**:
  - ✅ Adicionados imports `httpclient` e `credentials`
  - ✅ Struct `TTSClient` mudada para usar `*httpclient.Client`
  - ✅ `NewTTSClient()` refatorado para aceitar `credMgr` e criar cliente centralizado com timeout 60s
  - ✅ Chamadas `client.Do(req)` atualizadas para `client.Do(ctx, req)`
  - ✅ Import `time` mantido para `time.Second`
  - ✅ Todos os testes passando
- **Complexidade**: ⭐⭐ (média - gerencia credenciais OpenAI)
- **Nota**: Credenciais OpenAI já gerenciadas via `credentials.Manager`

#### 3.2. **`openai_whisper.go`** - Speech-to-Text
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/speech/openai_whisper.go`
- **Responsabilidade**: Transcrição de áudio OpenAI
- **Mudanças Realizadas**:
  - ✅ Adicionados imports `httpclient` e `credentials`
  - ✅ Struct `WhisperClient` mudada para usar `*httpclient.Client`
  - ✅ `NewWhisperClient()` refatorado para aceitar `credMgr` e criar cliente centralizado
  - ✅ Chamadas `client.Do(req)` atualizadas para `client.Do(ctx, req)` em `Transcribe()` e `TranscribeVerbose()`
  - ✅ MultipartForm preservado (não afetado pela mudança)
  - ✅ `speech_manager.go` atualizado para passar `credMgr`
  - ✅ `app.go` atualizado para passar `a.credMgr` ao `NewSpeechManager()`
  - ✅ Todos os testes passando
- **Complexidade**: ⭐⭐ (média)

---

### Fase 4: Messaging (Prioridade Média)

#### 4.1. **`signal/adapter.go`** - Adapter Signal
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/messaging/signal/adapter.go`
- **Responsabilidade**: Comunicação Signal Protocol
- **Cliente Atual**: Cria `http.Client` próprio (linha 61)
- **Mudanças Realizadas**:
  - ✅ Adicionados imports `httpclient` e `credentials`
  - ✅ `NewAdapter()` agora aceita `credMgr`
  - ✅ Cliente centralizado com timeout configurado
  - ✅ Chamadas `Do()` atualizadas para `Do(ctx, req)`
- **Complexidade**: ⭐⭐⭐ (alta - lógica síncrona/assíncrona complexa)

#### 4.2. **`signal/register.go`** - Registração Signal
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/messaging/signal/register.go`
- **Responsabilidade**: Registração/setup Signal
- **Cliente Atual**: `httpClient` global (linha 16)
- **Mudanças Realizadas**:
  - ✅ Removido `httpClient` global
  - ✅ Helper para obter cliente centralizado
  - ✅ Todas as chamadas atualizadas para `Do(ctx, req)`
- **Complexidade**: ⭐⭐ (média - variável global)

---

### Fase 5: Infraestrutura (Prioridade Média)

#### 5.1. **`updater.go`** - Auto-Update
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/updater/updater.go`
- **Responsabilidade**: Download e instalação de updates
- **Cliente Atual**: Cria `http.Client` próprio (linha 77)
- **Mudanças Realizadas**:
  - ✅ `credMgr` adicionado ao constructor
  - ✅ Cliente centralizado com alias `httpclient`
  - ✅ Todas as chamadas `Do()` atualizadas para `Do(ctx, req)`
- **Complexidade**: ⭐⭐⭐ (alta - download de arquivos grandes)

#### 5.2. **`http_pool.go`** - Pool HTTP LLM
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/llm/http_pool.go`
- **Responsabilidade**: Pool compartilhado para requisições LLM
- **Clientes Atuais**: `SharedHTTPClient` (linha 56) e factory `NewHTTPClientWithTimeout` (linha 71)
- **Mudanças Realizadas**:
  - ✅ Removido `SharedHTTPClient` global
  - ✅ Pool substituído pelo `httpclient.Client` centralizado
  - ✅ Arquivo removido após migração completa
- **Complexidade**: ⭐⭐⭐⭐ (muito alta - exigiu mudança estrutural no LLM)

#### 5.3. **`sync_client.go`** - Cliente Síncrono LLM
- **Status**: ✅ CONCLUÍDA
- **Localização**: `internal/llm/sync_client.go`
- **Responsabilidade**: Interface síncrona para chamadas LLM
- **Mudanças Realizadas**:
  - ✅ `SyncClient` migrado para `httpclient.Client`
  - ✅ `NewSyncClient()` agora aceita `credMgr`
  - ✅ Chamadas `Do()` atualizadas para `Do(ctx, req)`
- **Complexidade**: ⭐⭐⭐ (alta - dependia do http_pool)

---

### Fase 6: Testes e Utilitários (Prioridade Baixa)

#### 6.1. **`mcp_testing.go`** - Testes MCP
- **Status**: Superado pelo estado atual; o arquivo não existe mais
- **Localização histórica**: `internal/profiles/mcp_testing.go`
- **Mudanças Necessárias**:
  - [x] Remover o utilitário legado.
  - [ ] Auditar e tratar os consumidores diretos listados em “Situação Atual”.
- **Complexidade**: ⭐ (baixa)
- **Nota**: **Último** - não é crítico

---

## 📋 Roadmap Recomendado

```
Fase 1: ✅ CONCLUÍDA
├── http_request.go
├── web_fetch.go
└── signal.go

Fase 2: ✅ WEB TOOLS
└── web_search.go

Fase 3: ✅ FALA
├── openai_tts.go
└── openai_whisper.go

Fase 4: ✅ MESSAGING
├── signal/adapter.go
└── signal/register.go

Fase 5: ✅ INFRAESTRUTURA
├── updater.go
├── http_pool.go
└── sync_client.go

Fase 6: 🚧 AUDITORIA FINAL
├── Telegram
├── providers/connection
├── MCP OAuth/discovery
└── Google provider/external JWKS
```

---

## 🔧 Padrão de Refatoração

### Template Genérico
```go
// Antes
type Component struct {
    client *http.Client
}

func NewComponent() *Component {
    return &Component{
        client: &http.Client{Timeout: 30*time.Second},
    }
}

// Depois
type Component struct {
    client *httpclient.Client
}

func NewComponent(credMgr *credentials.Manager) *Component {
    if credMgr == nil {
        credMgr = credentials.NewManager(nil)
    }
    client := httpclient.New(&httpclient.Config{
        CredentialManager: credMgr,
    }, map[string]string{})
    return &Component{client: client}
}
```

### Chamadas HTTP
```go
// Antes
resp, err := c.client.Do(req)

// Depois
resp, err := c.client.Do(ctx, req)  // ctx geralmente já existe na função
```

---

## ⚠️ Considerações Especiais

### `web_search.go` - Interface SearchProvider
- Migração concluída: provider e mocks usam o cliente central conforme a Fase 2.

### `openai_tts.go` e `openai_whisper.go`
- Migração concluída conforme a Fase 3, com credenciais resolvidas pelo cliente
  central e testes dos serviços de fala.

### `signal/adapter.go` e `signal/register.go`
- Comunicação com Signal servers é crítica
- Manter SSL/TLS validation igual
- Testar integração signal após refatoração

### `updater.go`
- Download de arquivos binários (potencialmente >100MB)
- Pode precisar de `client.GetBaseClient()` para acesso ao Transport
- Considerar resumable downloads

### `http_pool.go` e `sync_client.go`
- Refatoração concluída com migração completa para `httpclient.Client`
- Pool global removido em favor do cliente centralizado

---

## 📊 Métricas para conclusão

- [ ] Suíte completa validada após a migração residual.
- [ ] Nenhum `http.Client` criado diretamente (exceto em testes).
- [x] HTTP tools principais usam `httpclient.Client`.
- [ ] Autenticação centralizada em todos os serviços aplicáveis.
- [ ] Retry policy compartilhada em todos os consumidores aplicáveis.
- [ ] Zero duplicação de lógica HTTP.

---

## 🚀 Próximos Passos

1. Concluir a auditoria dos clientes diretos listados em “Situação Atual”.
2. Migrar primeiro o `http.Get` de Telegram, que contorna timeout, retry,
   credenciais e políticas do wrapper.
3. Para clientes que precisem de transporte especializado, documentar a exceção
   e reutilizar as políticas compartilhadas cabíveis.
4. Rodar os testes focados e a suíte completa após a centralização residual.

---

## 📝 Anotações

- Este documento será atualizado conforme progresso
- Cada fase completada terá um commit individual
- Branches de feature: `feat/http-client-{component-name}`
- PRs separadas por fase quando possível
