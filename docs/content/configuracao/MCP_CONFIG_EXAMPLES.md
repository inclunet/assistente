---
title: "MCP — Exemplos"
weight: 3
---

# Exemplos de Configuração de Servidores MCP

> **Em 2 linhas:** MCP conecta ferramentas externas (arquivos, GitHub, Slack) ao assistente. Você descreve onde está o servidor e o assistente passa a oferecer aquelas ferramentas nas conversas.

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

## OAuth em servidores MCP remotos

Ao informar uma URL `https://` com transporte SSE ou Streamable HTTP, o
Assistente procura automaticamente metadados de recurso protegido e do servidor
OAuth/OIDC. A busca considera o caminho completo da URL, seus diretórios
ancestrais e a origem. Isso permite descobrir instalações atrás de gateways com
caminhos como `https://host/api/2.0/mcp`, sem regras específicas de fornecedor.

O resultado mostrado no formulário pode ser:

- **OAuth configurado automaticamente**: endpoints obrigatórios foram
  encontrados. Se o servidor publicar registro dinâmico, o Client ID poderá ser
  registrado durante a conexão.
- **OAuth detectado sem registro dinâmico**: informe o Client ID do aplicativo.
- **Descoberta parcial**: o recurso protegido foi reconhecido, mas o servidor de
  autorização não foi localizado; complete os endpoints manualmente.
- **Não detectado**: use a configuração manual, se o servidor exigir
  autenticação.

O discovery nunca substitui valores OAuth que você já preencheu. Falha parcial
ou total também não impede salvar o servidor. Desafios HTTP e erros de discovery
são tratados com limites e saneamento; tokens, cookies e credenciais não são
copiados para o resultado exibido. O ciclo completo tem duração limitada e pode
ser tentado novamente para a mesma URL depois de resultado parcial ou falha.
Escolher **Configurar manualmente** apenas abre os campos completos; isso não
transforma uma descoberta bem-sucedida em mensagem de falha.

### Reautorização OAuth (token expirado)

O token de acesso OAuth de servidores como o Atlassian expira periodicamente. O
Assistente tenta renová-lo automaticamente em segundo plano usando o
`refresh_token`. Quando a renovação não é possível (o provedor não emitiu
`refresh_token`, ou ele foi revogado/consumido), o servidor passa a exibir o
selo **"Reautorização necessária"** na coluna de status e é temporariamente
retirado do modo nativo — assim o Assistente não envia um token vencido ao
provedor de IA.

Para resolver, selecione o servidor e use a ação **Reautorizar** (disponível no
menu de ações da linha e na barra de ferramentas, apenas para servidores
OAuth2 PKCE). Uma janela do navegador será aberta para você autenticar
novamente; ao concluir, o token é renovado, o servidor reconecta e o selo
desaparece. **Reautorizar** é diferente de **Reconectar**: reconectar apenas
reabre a conexão com o mesmo token, enquanto reautorizar refaz o login OAuth.

> Dica de acessibilidade: a ação anuncia via leitor de telas que uma janela do
> navegador será aberta e confirma quando a reautorização termina.

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

Esses nomes canônicos podem ser usados literalmente em `tool_policy`. Para
governar um servidor inteiro, use `mcp/filesystem/*` (ou
`mcp:filesystem/*`); para todas as MCPs, use `mcp/*`. Uma entrada literal
`disabled` sempre vence o wildcard do servidor. Veja
[MCP — Configuração](MCP_PROFILE_CONFIG.md#política-mcp-no-perfil).

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

## Diagnóstico do registro OAuth dinâmico

Quando o servidor oferece registro dinâmico (DCR), o Assistente usa o endpoint
informado e conserva a URL de callback configurada, incluindo host, porta e path.
O pedido de registro tem limite de dez segundos e acompanha o cancelamento da
operação. Redirecionamentos HTTP nesse pedido não são seguidos: configure o
endpoint final de registro, sem um redirecionamento intermediário.

Se aparecer a mensagem de falha ao registrar o cliente OAuth, confira esse endpoint
e a URL de callback. O diagnóstico omite o corpo remoto para proteger segredos.
As conexões já cadastradas continuam usando suas credenciais; esta etapa não exige
novo login nem converte os registros existentes no cofre.

### Serviços corporativos e destinos internos

Se a descoberta OAuth apontar para um domínio ou IP interno, o Assistente consulta
as autorizações de rede existentes. Quando necessário, mostra o mesmo diálogo de
rede usado nas demais operações, com destino, porta e IPs, para você aprovar ou
negar. Isso também vale para redirects da descoberta. Confira o destino antes de
aprovar; uma negativa ou cancelamento encerra a tentativa.

Autorizações persistentes continuam gerenciadas na allowlist de rede. O tempo para
responder ao diálogo não consome os dez segundos do pedido de registro.
Endpoints OAuth e a descoberta no destino inicial exigem HTTPS, com exceção
de localhost/loopback; autorizar a rede
não desativa essa verificação nem a validação de identidade do servidor.

Uma renovação posterior do token pode solicitar autorização de rede novamente.
Para permitir também operações futuras, use a autorização persistente da allowlist.

A opção de permitir somente esta vez vale durante a operação OAuth, inclusive
nas consultas repetidas do Device Flow ao mesmo destino/IP. A espera pelo login
ou pela decisão de rede não consome o timeout do handshake MCP; Desconectar
continua cancelando a operação. Isso também vale para a verificação inicial
da conexão SSE. Se você negar o destino durante uma renovação, a recuperação
encerra a tentativa sem reconectar e perguntar novamente. Uma nova origem, porta
ou IP exige nova avaliação.

### URL do recurso MCP com OAuth

Em conexões locais do Assistente ao MCP com OAuth PKCE ou Client Credentials,
configure a URL final do recurso com HTTPS. HTTP continua permitido para
localhost e IPs de loopback. O token só é enviado ao scheme, domínio e porta
configurados; redirects para outro caminho nessa mesma origem são aceitos.

Redirects para outra origem ou para HTTP remoto são recusados antes do envio de
credenciais. A mesma proteção vale quando o servidor SSE anuncia um endpoint para
mensagens. Se isso ocorrer, confira a URL final com o administrador do serviço,
atualize a configuração e reconecte com a autorização adequada àquele recurso.
Uma permissão na allowlist de rede não autoriza encaminhar o token a outro serviço.

Destinos internos continuam usando as regras de confiança e consentimento já
descritas. SSE e Streamable HTTP permanecem disponíveis; uma resposta em streaming
não é encerrada pelo prazo de leitura das chamadas de token OAuth. Desconectar
continua cancelando a conexão. Esta mudança não altera os modos Bearer/Basic
estáticos nem o transporte remoto executado pelo provedor no modo MCP nativo.


### Código de dispositivo e porta de callback

Na autorização por código de dispositivo, o Assistente mostra o código e abre a
página de verificação. Aguarde a confirmação: as consultas respeitam o intervalo
informado pelo serviço e ficam mais espaçadas se ele pedir. Se você recusar ou o
código expirar, inicie uma nova autorização para tentar novamente; a tentativa
não abre automaticamente outro login. Esse fluxo não precisa de porta local.

Para autorização com callback, mantenha exatamente o host, a porta e o caminho
cadastrados no provedor. O Assistente reserva a porta antes de registrar o cliente
ou abrir o navegador. Se um cliente cadastrado manualmente usa uma porta ocupada,
feche o programa que a utiliza ou ajuste o cadastro e a configuração juntos.
Com registro dinâmico (DCR), o Assistente pode reservar outra porta e registrar a
nova URL antes de continuar. O host de callback deve ser `localhost`, `127.0.0.1`
ou `[::1]`; nunca um endereço de rede externa.

Recusas, códigos expirados, falhas de troca de código e portas indisponíveis têm
mensagens próprias. Não é necessário apagar as credenciais para tentar uma nova
autorização. Client Credentials continua obtendo e reutilizando tokens sem abrir
o navegador. Esta atualização preserva as credenciais MCP existentes; a conversão
para um único registro por autorização será feita em uma etapa posterior.

### Novos cadastros com autorização unificada

Para conferir o formato das configurações existentes, abra **Servidores MCP →
Diagnóstico OAuth**. A consulta mostra autorizações compostas, OAuth legado por
servidor, Client Credentials, entradas sem servidor OAuth correspondente e
credenciais por hostname (que também podem atender recursos fora do MCP).
Tokens Bearer importados por hostname entram no inventário quando correspondem
a um servidor OAuth. A comparação normaliza maiúsculas no hostname, como o
resolvedor de credenciais; tokens Bearer de outros recursos ficam fora da consulta.

O diagnóstico consulta apenas dados locais do usuário atual. Não conecta,
executa comandos, abre o navegador, renova tokens ou altera credenciais. Ele
aponta referências inválidas, registros incompletos, client IDs divergentes,
campos ilegíveis e associações por hostname que precisam de análise. Dados de
versões antigas que não estejam cifrados também aparecem como ilegíveis: isso
não significa que foram perdidos. Não apague entradas com base nesse relatório.

O inventário prepara a migração, mas **não migra nem certifica a validade da
autorização**. A ausência de alertas não substitui uma conexão bem-sucedida.
Metadados descobertos apenas durante a conexão podem não constar no banco.
Após alterações, feche e reabra o diagnóstico para consultar novamente.

Ao criar um servidor OAuth no editor MCP, o Assistente guarda cliente, segredo
opcional, tokens e configuração OAuth em uma única entrada cifrada do cofre.
Conexões existentes e importadas continuam funcionando no formato anterior;
não é necessário apagá-las ou cadastrá-las novamente.

Use **Conectar** para autorizar um novo servidor ou **Reautorizar** para repetir
o consentimento. Iniciar o aplicativo, listar ferramentas ou enviar mensagens
não abre o navegador automaticamente para essas novas autorizações. Se faltar
consentimento, a interface orienta a ação necessária. **Desconectar** cancela uma
tentativa em andamento e encerra a conexão; isso não revoga a autorização remota.

No formulário avançado, **Autenticação do cliente OAuth** permite escolher como
enviar o client secret quando ele existir: no corpo (`client_secret_post`) ou por
HTTP Basic (`client_secret_basic`). Use o método exigido pelo provedor. Clientes
públicos continuam sem precisar de segredo. Client Credentials funciona sem
navegador e reutiliza o token até precisar obter outro.

Os modos nativo e bridge usam a mesma autorização e renovação. Alterar apenas o
nome preserva os tokens. Alterar cliente, recurso, scopes ou endpoints invalida
o material anterior e pode exigir novo consentimento. Durante uma autorização
ou renovação ativa, aguarde a conclusão antes de editar ou excluir. Falhas na
gravação são reportadas; uma resposta ambígua de refresh rotativo exige nova
autorização, evitando reenviar um refresh token que pode já ter sido consumido.

A migração de cadastros existentes e a portabilidade do registro composto serão
entregues separadamente. Duplicar um cadastro não compartilha seus tokens ou
client secret: configure o segredo, se necessário, e autorize a nova conexão.


Se Client Credentials retornar permissões insuficientes, corrija os escopos na
configuração do servidor e conecte novamente. O Assistente não repete o pedido
de token a cada chamada enquanto essa condição permanecer.

Alterações de callback, recurso, endpoints ou escopos em um cadastro DCR invalidam o cliente registrado; a próxima autorização registra outro cliente. Renomear o servidor preserva o registro. A rejeição `invalid_scope` em Client Credentials também exige corrigir os escopos antes de uma nova tentativa.

Se o consentimento falhar após o DCR, o cliente registrado é preservado para a próxima tentativa. Uma conexão OAuth bem-sucedida limpa o aviso anterior de reautorização. Para Client Credentials inválido, revise ID/segredo ou escopos e conecte novamente; esse fluxo não usa o botão Reautorizar.

Durante a autorização ao conectar, o servidor fica em Conectando e oferece Cancelar. Recusa ou falha do login exibe erro; a interface só confirma sucesso após a conexão completar.

O seletor de autenticação Basic/Post aplica-se a clientes configurados manualmente e Client Credentials. Quando o discovery seleciona registro dinâmico público (DCR), o seletor é ocultado, pois o cliente é registrado sem segredo (`none`).

Clientes DCR públicos não aceitam segredo manual. Para usar um cliente confidencial, configure outro ID de cliente e seu método de autenticação; renomear o servidor preserva o registro público existente.

Reautorizar também oferece Cancelar durante o login, mesmo quando o servidor estava desconectado ou em erro. Cancelar/Desconectar encerra a tentativa; uma falha de autorização preserva o estado anterior da conexão.

Se uma conexão OAuth antiga não conseguir salvar o token obtido, o Assistente
interrompe a operação e informa a falha de gravação. Verifique o acesso ao cofre
antes de tentar novamente. A falha de gravação não abre outro consentimento
automaticamente. Não apague as credenciais para resolver esse erro.

Servidores que já usam autorização unificada recusam gravações atrasadas no
formato antigo e alterações antigas que tentem remover seu vínculo OAuth.
Uma mensagem de autorização alterada indica que a lista deve ser recarregada
antes de repetir a ação. As credenciais antigas ainda existentes são preservadas
para diagnóstico; esta proteção não executa migração ou exclusão automática.

A verificação inicial de compatibilidade SSE também interrompe a conexão se
não conseguir salvar um token renovado. O fallback para polling preserva a
configuração OAuth e a porta registrada. Se uma requisição não puder ser
reenviada após autorizar, a interface orienta conectar novamente; o aplicativo
não repete um corpo já consumido.

Ao criar um servidor, as opções Habilitado e Conectar automaticamente são
preservadas conforme escolhidas, inclusive quando desmarcadas. Um servidor
desabilitado não aceita conexão até ser habilitado.

### Renovação de conexões OAuth antigas

O Assistente coordena autorizações e renovações PKCE antigas entre instâncias
atualizadas que usam o mesmo banco. Enquanto uma tentativa estiver em andamento,
aguarde antes de conectar, editar ou remover suas credenciais. Outra instância
consulta os tokens salvos pela primeira, sem renovar novamente por usar um cache
antigo. Não compartilhe o banco com versões anteriores durante essas operações.

Se o aplicativo fechar durante uma renovação, ou receber uma resposta cujo
resultado não puder ser confirmado, ele pode pedir **Reautorizar**. Use essa ação
em **Servidores MCP** para obter novo consentimento. Reiniciar, conectar novamente
ou enviar outra mensagem não repete o refresh token que ficou incerto. Cancelar
a reautorização mantém esse pedido até uma autorização bem-sucedida.

Se a falha ocorreu ao salvar, resolva primeiro o acesso ao cofre. Não é preciso
apagar as credenciais para reautorizar. A remoção explícita continua disponível
quando não há tentativa ativa, mas remove somente os dados locais e não revoga
o acesso no serviço remoto. Esta proteção ainda não converte os cadastros antigos
para o formato unificado.

Ao registrar um cliente automaticamente, os dados do cliente e a configuração
de callback são salvos juntos. Uma falha de gravação preserva o estado local
anterior; após resolver o acesso ao cofre, tente conectar novamente.

Clientes públicos configurados manualmente também aparecem como autenticados
quando possuem tokens, mesmo sem segredo de cliente. Para descartar uma
autorização pendente, escolha autenticação **Nenhuma** e salve: a remoção das
credenciais e a alteração da configuração são confirmadas juntas. Se a gravação
falhar ou houver uma tentativa ativa, ambas são preservadas. Cadastros que dependem de
discovery continuam descobrindo o endpoint de renovação após reiniciar.

A ação **Remover** também pode descartar um servidor com renovação pendente
inativa: servidor e credenciais PKCE são removidos juntos. Uma tentativa ainda
ativa impede essa exclusão até terminar.

Salvar autenticação **Nenhuma** em um servidor remoto também remove as
credenciais locais de forma atômica para Bearer, Basic e Client Credentials.
O backend considera o cadastro atual, inclusive se outra instância mudou o tipo
desde a abertura do editor; uma falha preserva configuração e credenciais.

Ao remover a autenticação de um servidor, as credenciais persistidas são
excluídas juntas. Se a operação falhar ou outra instância alterar o cadastro,
nenhuma das credenciais é removida parcialmente.

### Recuperar configuração OAuth antiga

Em **Servidores MCP → Diagnóstico OAuth → Snapshots OAuth**, selecione um
servidor PKCE ou Client Credentials legado e escolha **Criar snapshot**. A cópia
é cifrada com a chave do seu cofre e inclui configuração e credenciais específicas
desse servidor. Também é possível selecionar separadamente uma credencial estática
por hostname apresentada no diagnóstico. Cadastros que já usam autorização
unificada e fontes externas não são incluídos.

A lista mostra a localização e o prazo de recuperação de 30 dias. Os arquivos
ficam na pasta `.assistente-oauth-recovery` do seu usuário, separados por banco e
usuário do Assistente. Eles não entram na exportação ou sincronização automática.
É necessário manter o banco no mesmo caminho, a conta e a chave do cofre para
recuperar. Mudar a senha preservando a mesma chave não altera essa associação;
perder ou substituir a chave impede abrir a cópia.

**Restaurar configuração** só funciona quando as credenciais estão ausentes e
não houve alterações posteriores no cadastro, ou quando o servidor foi removido
inteiramente. A ação não sobrescreve credenciais atuais. O servidor restaurado
fica desabilitado e sem conexão automática. Para **PKCE**, use **Reautorizar** e,
após concluir o login, habilite o servidor. Para **Client Credentials**, confira
o cliente e o segredo recuperados, habilite e conecte: o serviço obtém um token
novo sem consentimento no navegador. Se o cadastro era incompleto, complete os
campos antes de conectar. Se o Client ID estiver somente no cofre legado, ele será usado quando o campo da configuração estiver vazio. Nesses snapshots de servidor, tokens antigos não são restaurados ao cofre ativo:
restaurar um arquivo não desfaz rotação ou revogação no serviço remoto.

**Para snapshots por hostname**, a confirmação informa que os tokens e demais
segredos serão recuperados. Isso permite recuperar, por exemplo, um token copiado
do provedor cuja única cópia estava no Assistente. A entrada deve estar ausente:
uma credencial existente, mesmo vazia ou diferente, nunca é sobrescrita. A cópia
recupera o padrão original, os segredos e a validade original como fonte estática;
não prolonga tokens expirados nem garante que tokens revogados ou rotacionados
funcionem. Confira o acesso ao provedor após restaurar.

Uma credencial por hostname pode atender vários servidores e outros recursos.
Os próximos usos desse padrão poderão utilizar o token recuperado. Restaurar não
modifica, recria, habilita ou conecta servidores, nem associa a credencial a um
MCP específico. O snapshot permanece disponível para consulta ou descarte.
O resolvedor compara hostnames sem distinguir maiúsculas e minúsculas e aceita
URLs IPv6 com ou sem porta, preservando a escrita do padrão armazenado.
Se já existir uma variante do mesmo padrão com diferença apenas de maiúsculas,
a restauração é recusada. Duplicatas desse tipo no cofre bloqueiam a resolução
ambígua; nenhuma delas é escolhida automaticamente ou apagada.
Entradas incompatíveis com a captura aparecem no diagnóstico, mas não no seletor
de snapshots; isso inclui padrões com URL completa, caminho ou porta.

Snapshots PKCE anteriores continuam legíveis. Snapshots Client Credentials têm
formato próprio e exigem uma versão com este suporte para recuperação.

Ao vencer o prazo, a restauração é bloqueada, mas o arquivo não é apagado
automaticamente. **Descartar snapshot** pede confirmação de que a janela de
rollback terminou e a recuperação ou migração foi validada; pode remover a última
cópia e não pode ser desfeito. A exclusão não garante apagamento físico em SSD.
Esta entrega ainda não converte os cadastros antigos para autorização unificada.

### Client Credentials legado em múltiplas instâncias

Cadastros Client Credentials antigos e persistidos também coordenam a obtenção
de tokens entre instâncias atualizadas. Durante uma emissão, alterações do
cliente, exclusão da autenticação e criação de snapshot são recusadas; aguarde
terminar e repita a ação. O token continua em cache no transporte, mas cada uso
confere o cadastro atual. Um token em memória não mantém acesso após a exclusão
do cliente ou mudança do servidor para uma autorização composta.

Falhas de emissão podem ser tentadas novamente com **Conectar**. Esse fluxo não
precisa de login interativo nem utiliza refresh token. Se houver dados de PKCE
misturados no cadastro, a operação é recusada para investigação, preservando os
dados. Não compartilhe o banco com versões antigas durante operações OAuth.
Esta proteção ainda não converte o cadastro para uma autorização unificada.

Enquanto o cadastro Client Credentials permanecer legado, as ferramentas usam o
bridge local do Assistente. O token de um fallback por hostname não é enviado
ao provedor LLM pelo MCP nativo. Autorizações Client Credentials compostas
continuam disponíveis no modo nativo. A confirmação de rede, quando necessária,
acontece antes da tentativa e não é repetida para o mesmo grant.
Se o destino de rede mudar e exigir outra aprovação, a emissão é interrompida.
Use **Conectar** novamente para avaliar o novo destino antes de obter o token.

### Converter Client Credentials para autorização unificada

No diagnóstico OAuth, crie um snapshot do servidor Client Credentials e, na
entrada desse snapshot, escolha o método de autenticação exigido pelo servidor:
HTTP Basic ou credenciais no corpo. O formato antigo não guardava o método
negociado; consulte a configuração do serviço se não souber qual escolher.
Clique em **Converter autorização** e confirme.

A conversão mantém ID e segredo em uma única autorização cifrada, troca a
referência do servidor e remove seu par de entradas antigas na mesma transação.
O snapshot é mantido. A conexão atual é encerrada; use **Conectar** para obter
um token novo. Repetir a conversão do mesmo snapshot não duplica a autorização.
Essa repetição exige o mesmo método Basic/Post; para mudá-lo depois da conversão,
edite a autorização no cadastro. Em outra instância, a repetição encerra apenas
a conexão que ainda usava o formato antigo. Aguarde esse encerramento antes de
conectar novamente.
Credenciais compartilhadas por hostname não são alteradas.

Se o cadastro mudou desde o snapshot, crie outro. Se houver emissão ativa,
aguarde terminar antes de capturar e converter. Cadastro incompleto, segredo
ilegível, fonte externa, ID divergente ou tokens residuais impedem a conversão,
preservando os dados para investigação. Para PKCE, use a reconexão explícita descrita abaixo.

Para voltar ao cadastro anterior durante os 30 dias de retenção, remova
explicitamente o servidor convertido e restaure o snapshot. A restauração não
sobrescreve uma autorização atual. O servidor volta desabilitado, com ID e
segredo recuperados; habilite e conecte para emitir um token novo. Mantenha o
snapshot até validar a conexão. Não abra o mesmo banco simultaneamente com uma
versão antiga do Assistente.

### Reconectar e migrar PKCE legado

No diagnóstico OAuth, crie um snapshot do servidor PKCE. Na entrada dele,
selecione o método exigido pelo provedor e use **Reconectar e migrar**:

- **Cliente público** para clientes sem segredo, incluindo um novo cadastro DCR.
- **Basic** ou **Post** para clientes com segredo, conforme o provedor.

O Assistente reaproveita o ID e o segredo disponíveis e abre o fluxo normal de
autorização. A nova autorização fornece os metadados que o formato antigo não
guardava. Endpoints e a configuração de callback são reaproveitados. Sem DCR,
uma porta fixa ocupada impede a autorização; com DCR, o protocolo existente pode
registrar outro cliente com uma porta disponível. As decisões de acesso a destinos
de rede seguem as mesmas permissões da conexão OAuth normal.

Somente após o login e a gravação concluírem o Assistente troca o cadastro para
uma entrada composta e remove o par antigo. Use **Conectar** depois para abrir a
conexão MCP. O snapshot permanece cifrado por seus 30 dias originais. Repetir a
ação com o mesmo snapshot e método não abre outro login após o sucesso.

Se cancelar ou ocorrer uma falha, o cadastro local anterior é preservado.
Durante o fluxo, renovações concorrentes são recusadas. Após queda do app,
aguarde a reserva expirar (até dez minutos) e tente novamente com o snapshot
original ou crie outro. Se o cadastro foi editado desde a captura, faça outro
snapshot; a reserva expirada não prende a recuperação à captura anterior.

O provedor pode invalidar tokens anteriores durante a nova autorização; o
snapshot não reverte essa invalidação. A ação é opcional: cadastros legados
continuam funcionando, e o Assistente não abre um login só porque o formato
mudou. A conversão PKCE sem novo login ainda não está disponível quando faltam
os metadados históricos do grant.
