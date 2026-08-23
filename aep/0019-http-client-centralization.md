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
- **Status**: Pendente
- **Localização**: `internal/profiles/mcp_testing.go`
- **Clientes Locais**: Cria `http.Client` inline (linhas 56, 119)
- **Mudanças Necessárias**:
  - [ ] Simples - apenas criar clientes de teste centralizados
- **Complexidade**: ⭐ (baixa)
- **Nota**: **Último** - não é crítico

---

## 📋 Roadmap Recomendado

```
Fase 1: ✅ CONCLUÍDA
├── http_request.go
├── web_fetch.go
└── signal.go

Fase 2: ⏳ WEB TOOLS (2-3 horas)
└── web_search.go

Fase 3: ⏳ FALA (2-3 horas)
├── openai_tts.go
└── openai_whisper.go

Fase 4: ✅ MESSAGING
├── signal/adapter.go
└── signal/register.go

Fase 5: ✅ INFRAESTRUTURA
├── updater.go
├── http_pool.go
└── sync_client.go

Fase 6: ⏳ TESTES (1 hora)
└── mcp_testing.go
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
- A interface `SearchProvider` aceita `*http.Client` como parâmetro
- Precisa ser atualizada para aceitar `*httpclient.Client`
- Impacta implementações: `duckDuckGoProvider`, `mockSearchProvider` (testes)

### `openai_tts.go` e `openai_whisper.go`
- Gerenciam credenciais OpenAI
- Atual: Credenciais passadas no header manualmente
- Futuro: Usar resolver automático do `httpclient.Client`
- ⚠️ Testar autenticação cuidadosamente

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

## 📊 Métricas de Sucesso

- ✅ Todos os testes passando
- ✅ Nenhum `http.Client` criado diretamente (exceto em testes)
- ✅ Todos os HTTP tools usando `httpclient.Client`
- ✅ Autenticação funcionando em todos os serviços
- ✅ Retry policy aplicada globalmente
- ✅ Zero duplicação de lógica HTTP

---

## 🚀 Próximos Passos

1. **Fase 2**: Refatorar `web_search.go`
   - Ler e entender interface SearchProvider
   - Atualizar provider.Search() signature
   - Testar buscas após mudança

2. **Fase 3**: Refatorar OpenAI TTS/Whisper
   - Testar autenticação OpenAI
   - Validar requisições multipart (Whisper)

3. **Fases 4-5**: Continuar sequencialmente

4. **Fase 6**: Apenas após todas outras concluídas

---

## 📝 Anotações

- Este documento será atualizado conforme progresso
- Cada fase completada terá um commit individual
- Branches de feature: `feat/http-client-{component-name}`
- PRs separadas por fase quando possível
