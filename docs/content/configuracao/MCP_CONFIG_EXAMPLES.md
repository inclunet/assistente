---
title: "MCP — Exemplos"
weight: 3
---

# Exemplos de Configuração de Servidores MCP

Este arquivo contém exemplos práticos de configuração de servidores MCP para o Assistente.

## Transporte e MCP Nativo

A forma como o Assistente consome um servidor MCP depende de três dimensões: o
**transporte** do servidor, a **capacidade física do provider LLM** e a
**política tri-state do perfil**. Para transportes HTTP, a URL ainda precisa
passar pela regra de elegibilidade de segurança.

| Transporte | Caminho | Quando |
|------------|---------|--------|
| `stdio` | Sempre **adapter/bridge local** | Servidor roda como processo local; não pode ser acessado remotamente |
| `sse` / `streamable` | **MCP nativo** | Provider capaz, `native_mcp` permite, URL elegível, `prefer_bridge=false` e ao menos uma tool do servidor está `preloaded` pela política efetiva |
| `sse` / `streamable` | **Adapter/bridge local** | Qualquer gate nativo falha, a tool está apenas `on_demand`, ou ocorre fallback automático |

A capacidade física vem de `NativeMCPCapable()`. A política vem de
`Profile.Chat.NativeMCP *bool`: `nil` tenta nativo automaticamente quando
possível, `true` força a tentativa nativa e `false` força adapter. Se o modelo
ou endpoint rejeitar MCP nativo no modo automático, o Assistente refaz o mesmo
turno com bridge tools e persiste `nil` → `false` no perfil. URLs `http://` com
host remoto continuam excluídas por segurança.

Em **perfil legado sem `tool_policy` e sem `tool_policy_default`**,
`enabled_tools: null` com `tool_catalog` disponível pré-carrega inicialmente
apenas o catálogo; portanto tools MCP permanecem `on_demand` e o servidor não
entra no caminho nativo no início do turno. Quando uma tool MCP é carregada sob
demanda, ela permanece bridge/function nesse turno. Um `tool_policy` explícito
ou `tool_policy_default` não vazio pode ativar a política nova mesmo com
`enabled_tools: null`; entradas efetivamente `preloaded` podem então satisfazer
o gate nativo.

## 📁 Localização

Arquivos de configuração ficam em:
```
~/.assistente/mcp/
├── filesystem.json
├── github.json
├── slack.json
└── custom-server.json
```

---

## Exemplo 1: Servidor Stdio (Node.js)

### Filesystem Server
**Arquivo:** `~/.assistente/mcp/filesystem.json`

```json
{
  "name": "Filesystem Server",
  "description": "Acesso a arquivos e diretórios do sistema",
  "transport": "stdio",
  "command": "npx",
  "args": [
    "-y",
    "@modelcontextprotocol/server-filesystem",
    "/home/user/projects"
  ],
  "env": {
    "NODE_ENV": "production"
  },
  "enabled": true,
  "auto_connect": true
}
```

**Capabilities:**
- ✅ Tools: `read_file`, `write_file`, `list_directory`, etc.
- ✅ Resources: Arquivos como `file:///path/to/file.txt`
- ❌ Prompts: Não disponível

---

## Exemplo 2: Servidor Stdio (Python)

### GitHub MCP Server
**Arquivo:** `~/.assistente/mcp/github.json`

```json
{
  "name": "GitHub Server",
  "description": "Integração com GitHub API",
  "transport": "stdio",
  "command": "python",
  "args": [
    "-m",
    "mcp_server_github"
  ],
  "env": {
    "GITHUB_TOKEN": "ghp_your_token_here"
  },
  "enabled": true,
  "auto_connect": true
}
```

**Capabilities:**
- ✅ Tools: `create_issue`, `list_prs`, `search_code`, etc.
- ✅ Resources: Issues, PRs como `github://owner/repo/issues/123`
- ✅ Prompts: Templates para PRs, issues

---

## Exemplo 3: Servidor SSE (HTTP)

### Custom Web Server
**Arquivo:** `~/.assistente/mcp/custom-web.json`

```json
{
  "name": "Custom Web Service",
  "description": "Servidor MCP customizado via HTTP",
  "transport": "sse",
  "url": "http://localhost:3000/mcp",
  "enabled": true,
  "auto_connect": true
}
```

**Vantagens SSE:**
- Permite deployar servidor MCP em container/cloud
- Suporte a múltiplos clientes simultaneamente
- Facilita load balancing
- **Candidato a MCP nativo** quando conectado, com tools disponíveis e URL
  `https://` (ou `http://` apenas em localhost/loopback); o caminho final ainda
  exige provider capaz, `native_mcp` permitindo, `prefer_bridge=false` e ao
  menos uma tool preloaded no turno

---

## Exemplo 4: Servidor Local Go

### Database MCP Server
**Arquivo:** `~/.assistente/mcp/database.json`

```json
{
  "name": "Database Server",
  "description": "Queries em banco de dados",
  "transport": "stdio",
  "command": "C:\\path\\to\\mcp-database-server.exe",
  "args": [],
  "env": {
    "DB_HOST": "localhost",
    "DB_PORT": "5432",
    "DB_NAME": "myapp",
    "DB_USER": "postgres",
    "DB_PASSWORD": "secret"
  },
  "enabled": true,
  "auto_connect": false
}
```

**Nota:** `auto_connect: false` - conecta manualmente por segurança

---

## Exemplo 5: Servidor com Múltiplos Ambientes

### Development vs Production
**Arquivo:** `~/.assistente/mcp/api-dev.json`

```json
{
  "name": "API Server (Dev)",
  "description": "Servidor de API em desenvolvimento",
  "transport": "sse",
  "url": "http://localhost:8080/mcp",
  "enabled": true,
  "auto_connect": true
}
```

**Arquivo:** `~/.assistente/mcp/api-prod.json`

```json
{
  "name": "API Server (Prod)",
  "description": "Servidor de API em produção",
  "transport": "sse",
  "url": "https://api.mycompany.com/mcp",
  "enabled": false,
  "auto_connect": false
}
```

**Estratégia:**
- Dev: sempre conectado
- Prod: manual, por segurança

---

## Exemplo 6: Servidor Docker

### Containerized MCP Server
**Arquivo:** `~/.assistente/mcp/docker-server.json`

```json
{
  "name": "Docker MCP Server",
  "description": "Servidor MCP rodando em container",
  "transport": "stdio",
  "command": "docker",
  "args": [
    "run",
    "--rm",
    "-i",
    "mycompany/mcp-server:latest"
  ],
  "env": {
    "API_KEY": "your-api-key"
  },
  "enabled": true,
  "auto_connect": true
}
```

**Nota:** Flags importantes:
- `--rm`: Remove container ao desconectar
- `-i`: Modo interativo (stdin/stdout)

---

## Exemplo 7: Servidor com Auth

### Slack MCP Server
**Arquivo:** `~/.assistente/mcp/slack.json`

```json
{
  "name": "Slack Server",
  "description": "Integração com Slack",
  "transport": "stdio",
  "command": "npx",
  "args": [
    "-y",
    "@modelcontextprotocol/server-slack"
  ],
  "env": {
    "SLACK_BOT_TOKEN": "xoxb-your-token",
    "SLACK_APP_TOKEN": "xapp-your-token"
  },
  "enabled": true,
  "auto_connect": false
}
```

**Security:** Tokens em env vars, não no JSON

---

## Exemplo 8: Servidor Multi-tenancy

### Workspace-Specific Server
**Arquivo:** `~/.assistente/mcp/workspace-tools.json`

```json
{
  "name": "Workspace Tools",
  "description": "Ferramentas específicas do workspace atual",
  "transport": "stdio",
  "command": "node",
  "args": [
    "/path/to/workspace-mcp-server.js",
    "${WORKSPACE_PATH}"
  ],
  "env": {
    "WORKSPACE_PATH": "/home/user/projects/myproject"
  },
  "enabled": true,
  "auto_connect": true
}
```

---

## Configurações Avançadas

### Health Check Tuning
Embora não esteja no JSON, você pode ajustar no código:

```go
const (
    healthCheckInterval = 30 * time.Second  // Ping a cada 30s
    healthCheckTimeout = 5 * time.Second    // Timeout de 5s
    maxRetries = 5                          // Máximo 5 tentativas
    baseRetryDelay = 1 * time.Second        // Delay inicial 1s
    maxRetryDelay = 5 * time.Minute         // Delay máximo 5min
)
```

### Tuning Recommendations

| Cenário | healthCheckInterval | healthCheckTimeout | maxRetries |
|---------|--------------------|--------------------|------------|
| **Local (rápido)** | 10s | 2s | 3 |
| **Padrão** | 30s | 5s | 5 |
| **Cloud (lento)** | 60s | 10s | 10 |
| **Critical** | 5s | 1s | ∞ |

---

## Testando Configurações

### 1. Validar JSON
```bash
jq . ~/.assistente/mcp/filesystem.json
# Se retornar sem erro, JSON está válido
```

### 2. Testar Comando Manualmente
```bash
# Teste stdio
npx -y @modelcontextprotocol/server-filesystem /home/user

# Teste SSE
curl http://localhost:3000/mcp/health
```

### 3. Logs do Assistente
```
[MCP] Servidor carregado: filesystem (Filesystem Server, transport=stdio, enabled=true, auto_connect=true)
[MCP] Servidor 'filesystem' conectado: 12 ferramentas descobertas
[MCP]   - tool: mcp_filesystem__read_file (read_file)
[MCP]   - tool: mcp_filesystem__write_file (write_file)
...
```

---

## Troubleshooting

### Erro: "command not found"
```json
{
  "command": "/full/path/to/command",  // Use caminho absoluto
  "env": {
    "PATH": "/usr/local/bin:/usr/bin"  // Adicione PATH se necessário
  }
}
```

### Erro: "connection timeout"
```json
{
  "auto_connect": false  // Conecte manualmente para investigar
}
```

### Erro: "permission denied"
```bash
chmod +x /path/to/mcp-server
```

### Verificar status
```typescript
// No frontend
const servers = await ListMCPServers();
servers.forEach(srv => {
  console.log(`${srv.name}: ${srv.status}`);
  if (srv.error) {
    console.error(`  Error: ${srv.error}`);
  }
  if (srv.lastPing) {
    console.log(`  Last ping: ${srv.lastPing}`);
  }
});
```

---

## Boas Práticas

### ✅ DO
- Use `auto_connect: true` para servidores confiáveis
- Coloque tokens/secrets em `env`, não em args
- Use caminhos absolutos quando possível
- Teste comando manualmente primeiro
- Configure health checks apropriados

### ❌ DON'T
- Não commite arquivos com secrets
- Não use `auto_connect: true` em produção sensível
- Não configure `maxRetries` muito alto (causa loops)
- Não deixe servidores inativos habilitados

---

## Resumo

- **Stdio**: Para servidores locais (Node, Python, Go, Rust) — sempre via adapter/bridge
- **SSE / Streamable HTTP**: Para servidores remotos/locais — candidatos ao
  caminho nativo com URL segura; a decisão final também aplica capacidade do
  provider, tri-state do perfil, `prefer_bridge` e preload efetivo por turno
- **Auth**: Tokens via env vars, nunca hardcoded no JSON
- **Docker**: Servidores containerizados via stdio
- **Auto-reconnect**: Health checks + exponential backoff automático
