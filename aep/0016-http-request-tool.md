# HTTP Request Tool - Proposta de Implementação

**Status:** Done — escopo funcional entregue pelo contrato centralizado vigente

## Contexto

A ferramenta atual `web_fetch` possui limitações significativas:
- ✅ Suporta apenas método GET
- ❌ Não suporta POST/PUT/DELETE/PATCH
- ❌ Não permite headers customizados
- ❌ Não permite envio de body/payload
- ❌ Não suporta autenticação (Basic Auth, Bearer tokens)
- ❌ Força uso de `curl` via terminal (ineficiente)

## Contrato vigente

`internal/tools/web/http_request.go` expõe somente:

- `url`;
- `method`;
- `headers`;
- `body` e `body_type`;
- `max_response_size`;
- `extract_mode`.

Não existem argumentos `auth_basic`, `auth_bearer` ou `timeout_seconds` no
schema nem em `httpRequestArgs`. Credenciais são resolvidas pela URL no cliente
central de `internal/tools/http`, que recebe o `credentials.Manager` no
construtor da tool. Timeout e retry também pertencem à configuração desse
cliente, não ao payload controlado pelo modelo.

A resolução automática evita exigir argumentos dedicados de autenticação e
reduz exposição acidental. Ela não torna o payload incapaz de carregar
segredos: headers arbitrários, inclusive `Authorization`, são preservados, e a
própria URL pode conter informação sensível. O contrato centralizado foi
consolidado pelas AEPs 0018 e 0019.

## Design original da tool (histórico)

Os parâmetros e exemplos desta seção registram a proposta inicial. Em
particular, `auth_basic`, `auth_bearer` e `timeout_seconds` foram superseded
pelo contrato centralizado acima e **não devem ser enviados à tool vigente**.

### Proposta original

### Objetivo
Criar ferramenta completa para requisições HTTP que suporte:
- ✅ Todos os métodos HTTP (GET, POST, PUT, DELETE, PATCH, HEAD, OPTIONS)
- ✅ Headers customizados (Authorization, Content-Type, etc.)
- ✅ Body/payload (JSON, form-data, text/plain, raw)
- ✅ Autenticação resolvida automaticamente pelo cliente central
- ✅ Validações de segurança mantidas (bloqueio de hosts privados)
- ✅ Timeout governado pelo cliente HTTP central
- ✅ Suporte a diferentes encodings de resposta

### Especificação de Parâmetros

```json
{
  "type": "object",
  "properties": {
    "url": {
      "type": "string",
      "description": "URL completa (http:// ou https://)"
    },
    "method": {
      "type": "string",
      "enum": ["GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"],
      "description": "Método HTTP (padrão: GET)",
      "default": "GET"
    },
    "headers": {
      "type": "object",
      "additionalProperties": {"type": "string"},
      "description": "Headers HTTP customizados (ex: {\"Authorization\": \"Bearer token\", \"Content-Type\": \"application/json\"})"
    },
    "body": {
      "type": "string",
      "description": "Body da requisição (para POST/PUT/PATCH). Pode ser JSON string, form data ou texto."
    },
    "body_type": {
      "type": "string",
      "enum": ["json", "form", "text", "raw"],
      "description": "Tipo do body: 'json' (application/json), 'form' (application/x-www-form-urlencoded), 'text' (text/plain), 'raw' (sem Content-Type)",
      "default": "json"
    },
    "auth_basic": {
      "type": "object",
      "properties": {
        "username": {"type": "string"},
        "password": {"type": "string"}
      },
      "description": "Credenciais para Basic Authentication"
    },
    "auth_bearer": {
      "type": "string",
      "description": "Token para Bearer Authentication (alternativa a auth_basic)"
    },
    "timeout_seconds": {
      "type": "integer",
      "description": "Timeout da requisição em segundos (padrão: 30, máx: 120)",
      "default": 30,
      "minimum": 1,
      "maximum": 120
    },
    "max_response_size": {
      "type": "integer",
      "description": "Tamanho máximo da resposta em caracteres (padrão: 50000)",
      "default": 50000
    },
    "extract_mode": {
      "type": "string",
      "enum": ["auto", "text", "json", "raw"],
      "description": "Modo de processamento da resposta: 'auto' detecta automaticamente, 'text' extrai texto, 'json' formata JSON, 'raw' retorna sem processar",
      "default": "auto"
    }
  },
  "required": ["url"],
  "additionalProperties": false
}
```

### Salvaguardas de Segurança

#### Mantidas do `web_fetch`:
1. **Bloqueio de hosts privados** (padrão)
   - localhost, 127.0.0.1, ::1
   - Ranges privados: 10.x, 192.168.x, 172.16-31.x
   - Apenas desabilitado em testes com flag `allowPrivateHosts`

2. **Limites de tamanho**
   - Máximo 10MB de download por requisição
   - Truncamento configurável da resposta

3. **Timeout**
   - Padrão: 30 segundos
   - Máximo: 120 segundos

4. **Validação de URLs**
   - Apenas http:// e https://
   - Parse e validação antes da execução

#### Novas salvaguardas:
5. **Headers sensíveis**
   - Avisos ao usar headers de autenticação
   - Não logar valores de tokens/passwords

6. **Rate limiting** (futuro)
   - Limitar número de requisições por minuto
   - Evitar abuse de APIs externas

7. **Auditoria**
   - Registrar método, URL e status code
   - Não registrar payloads ou headers sensíveis

### Exemplos de Uso

#### 1. GET simples (compatível com web_fetch)
```json
{
  "url": "https://api.example.com/users"
}
```

#### 2. POST com JSON
```json
{
  "url": "https://api.example.com/users",
  "method": "POST",
  "headers": {
    "Content-Type": "application/json",
    "Authorization": "Bearer abc123"
  },
  "body": "{\"name\": \"João\", \"email\": \"joao@example.com\"}",
  "body_type": "json"
}
```

#### 3. PUT com autenticação básica
```json
{
  "url": "https://api.example.com/users/123",
  "method": "PUT",
  "auth_basic": {
    "username": "admin",
    "password": "secret"
  },
  "body": "{\"status\": \"active\"}",
  "body_type": "json"
}
```

#### 4. DELETE com Bearer token
```json
{
  "url": "https://api.example.com/users/123",
  "method": "DELETE",
  "auth_bearer": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
}
```

#### 5. POST form data
```json
{
  "url": "https://api.example.com/login",
  "method": "POST",
  "body": "username=user&password=pass123",
  "body_type": "form"
}
```

### Implementação

#### Estrutura de arquivos:
```
internal/tools/web/
  ├── web_fetch.go          # Mantido (para compatibilidade)
  ├── web_fetch_test.go     # Mantido
  ├── http_request.go       # NOVO
  ├── http_request_test.go  # NOVO
  ├── web_search.go         # Mantido
  └── web_search_test.go    # Mantido
```

#### Registro em app.go:
```go
// Registra ferramentas web
a.toolRegistry.MustRegister(web.NewWebFetch())    // Mantido para compatibilidade
a.toolRegistry.MustRegister(web.NewHTTPRequest()) // NOVO - ferramenta completa
a.toolRegistry.MustRegister(web.NewWebSearch())
```

### Migração e Compatibilidade

#### Opção 1: Manter ambas as tools (RECOMENDADO)
- `web_fetch` → continua existindo (GET simples, foco em leitura)
- `http_request` → nova tool completa (todos os métodos)
- Benefício: sem breaking changes, transição gradual

#### Opção 2: Depreciar web_fetch
- Adicionar aviso de deprecação na descrição
- Remover em versão futura (major version bump)

**RECOMENDAÇÃO:** Opção 1 (manter ambas).

### Benefícios

1. **Operacional:**
   - Elimina necessidade de usar `curl` via terminal
   - Execução mais rápida e confiável
   - Menos verboso (sem escape de shell)

2. **Funcional:**
   - Permite integração com APIs REST completas
   - Suporta autenticação moderna (Bearer tokens)
   - Permite testes de APIs diretamente do assistente

3. **Segurança:**
   - Mantém proteções contra hosts privados
   - Auditoria de requisições
   - Validações consistentes

4. **Desenvolvimento:**
   - Código Go nativo (mais testável)
   - Tratamento de erros consistente
   - Reutilização de código (HTTP client, validações)

### Implementação entregue

1. ✅ Tool implementada em `internal/tools/web/http_request.go`.
2. ✅ Cobertura unitária em `internal/tools/web/http_request_test.go`.
3. ✅ Registro no catálogo compartilhado de tools.
4. ✅ Integração com cliente HTTP centralizado, credenciais e guardrails
   anti-SSRF; autenticação é resolvida por URL, sem argumentos de segredo.
5. ✅ Execução passa pelo pipeline comum de tools e seus testes de catálogo.

### Critérios verificados

- [x] O código aceita os métodos declarados, aplica `body_type` e implementa os
  modos de extração em `internal/tools/web/http_request.go`.
- [x] `internal/tools/web/http_request_test.go` cobre GET, POST JSON, DELETE,
  confirmação de DELETE, headers customizados e extração JSON.
- [ ] A matriz unitária ainda não cobre PUT/PATCH/HEAD/OPTIONS, todos os
  `body_type` (`form`, `text`, `raw`) nem todos os `extract_mode`.
- [x] O schema publicado coincide com `httpRequestArgs` e não oferece
  `auth_basic`, `auth_bearer` ou `timeout_seconds`.
- [x] Credenciais e timeout são delegados ao cliente central de
  `internal/tools/http`.
- [x] Guardrails anti-SSRF e confirmação de operações mutáveis permanecem no
  pipeline compartilhado.

---

**Estado:** escopo funcional implementado pelo contrato vigente; desenho
original de autenticação/timeout superseded pelas AEPs 0018/0019.
**Prioridade:** Alta (impacto operacional significativo)  
**Estimativa:** 2-3 horas de implementação + testes
