# Exemplos de Configuração de Servidores MCP

Este arquivo contém exemplos práticos de configuração de servidores MCP para o Assistente.

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

## Exemplo 3: Servidor Streamable HTTP (Recomendado)

### Servidor remoto com autenticação
**Arquivo:** `~/.assistente/mcp/api-server.json`

```json
{
  "name": "API Server",
  "description": "Servidor MCP remoto via Streamable HTTP",
  "transport": "streamable",
  "url": "https://api.example.com/mcp",
  "enabled": true,
  "auto_connect": true
}
```

**Autenticação:** Configurada na UI do servidor MCP (seção "Autenticação") ou
na tela de Credenciais. As credenciais são armazenadas de forma segura no
credential manager do sistema (criptografia AES-256-GCM + keyring do OS),
nunca em texto puro no arquivo JSON.

O padrão de resolução usa o hostname da URL. Para o exemplo acima,
credenciais registradas para `api.example.com` serão automaticamente injetadas.

**Vantagens Streamable HTTP:**
- Protocolo MCP moderno (substitui SSE legado)
- Autenticação segura via credential manager centralizado
- Retries automáticos com backoff exponencial
- Suporte a server-sent events para notificações do servidor
- Permite deployar servidor MCP em container/cloud
- Não necessita de ferramentas como `mcp-remote`

### Servidor local sem autenticação
**Arquivo:** `~/.assistente/mcp/local-streamable.json`

```json
{
  "name": "Local Streamable Server",
  "description": "Servidor MCP local via Streamable HTTP",
  "transport": "streamable",
  "url": "http://localhost:3000/mcp",
  "enabled": true,
  "auto_connect": true
}
```

### Servidor com OAuth2 Client Credentials
**Arquivo:** `~/.assistente/mcp/api-oauth2-cc.json`

```json
{
  "name": "API Server (OAuth2)",
  "description": "Servidor MCP com autenticação OAuth2 Client Credentials",
  "transport": "streamable",
  "url": "https://api.example.com/mcp",
  "auth_type": "oauth2_client_credentials",
  "oauth2_client_id": "meu-app-id",
  "oauth2_token_url": "https://auth.example.com/oauth/token",
  "oauth2_scopes": ["read", "write"],
  "enabled": true,
  "auto_connect": true
}
```

**Nota:** O `client_secret` é armazenado no credential manager (criptografado), não no JSON.
Configure pela UI na seção de autenticação do servidor.

### Servidor com OAuth2 Authorization Code (PKCE)
**Arquivo:** `~/.assistente/mcp/api-oauth2-pkce.json`

```json
{
  "name": "API Server (PKCE)",
  "description": "Servidor MCP com login OAuth2 via browser",
  "transport": "streamable",
  "url": "https://api.example.com/mcp",
  "auth_type": "oauth2_pkce",
  "oauth2_client_id": "meu-app-id",
  "oauth2_token_url": "https://auth.example.com/oauth/token",
  "oauth2_auth_url": "https://auth.example.com/authorize",
  "oauth2_scopes": ["openid", "profile"],
  "enabled": true,
  "auto_connect": false
}
```

**Nota:** PKCE usa public client (sem client_secret). Na primeira conexão, o browser
abrirá para o usuário se autenticar. O refresh token é salvo no credential manager
para sessões futuras. Recomenda-se `auto_connect: false` para controlar quando o
browser abre.

---

## Exemplo 4: Servidor SSE (Legado)

### Custom Web Server
**Arquivo:** `~/.assistente/mcp/custom-web.json`

```json
{
  "name": "Custom Web Service",
  "description": "Servidor MCP via SSE (legado)",
  "transport": "sse",
  "url": "http://localhost:3000/mcp",
  "enabled": true,
  "auto_connect": true
}
```

**Nota:** O transporte SSE é considerado legado. Prefira `streamable` para novas configurações.
Autenticação é gerida pelo credential manager, da mesma forma que o Streamable HTTP.

---

## Exemplo 5: Servidor Local Go

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

## Exemplo 6: Servidor com Múltiplos Ambientes

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

## Exemplo 7: Servidor Docker

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

## Exemplo 8: Servidor com Auth

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

## Exemplo 9: Servidor Multi-tenancy

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

## Conclusão

Com essas configurações, você pode integrar qualquer servidor MCP ao Assistente:

- 🔧 **Stdio**: Para servidores locais (Node, Python, Go, Rust)
- 🌐 **Streamable HTTP**: Para servidores remotos (recomendado)
- 📡 **SSE**: Para servidores HTTP legado
- 🔐 **Auth**: Tokens via env vars (stdio) ou credential manager (streamable/sse)
- 🔑 **OAuth2**: Client Credentials (M2M) ou Authorization Code + PKCE (com browser)
- 🐳 **Docker**: Servidores containerizados
- 🔄 **Auto-reconnect**: Resiliência automática

**Todos os exemplos são production-ready!** 🚀
