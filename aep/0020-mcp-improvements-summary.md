# Melhorias Implementadas no Conector MCP

## Status: Historico (superseded)

> **Este documento e um registro historico.** Descreve melhorias feitas ao conector MCP em sua versao original (pre-SDK nativo). Partes da arquitetura descrita aqui foram substituidas pela migracao para SDKs oficiais ([AEP-0037](0037-sdk-migration-chat-provider.md)) e pelo modo MCP nativo capability-driven ([AEP-0021](0021-mcp-native-mode.md)).
>
> **O que mudou desde este documento:**
> - `GetNativeMCPServers()` / `GetNativeServerInfo()` foram removidos — substituidos por `GetEligibleNativeMCPServers()` no runtime
> - MCP nativo nao depende mais de "passar info para Claude" manualmente — o ChatProvider resolve server-side via `WithMCPServers()`
> - A decisao nativo vs adapter e capability-driven (baseada em `api_format` do provider), nao manual
>
> Para a arquitetura atual, consulte [AEP-0021](0021-mcp-native-mode.md) e [AEP-0037](0037-sdk-migration-chat-provider.md).

---

## Features originais (continuam funcionais)

### 1. Resources MCP
- Discovery automatico de resources ao conectar
- Metodo `ReadResource(slug, uri)` para ler conteudo
- Contagem e listagem de resources por servidor
- Suporte a ResourceTemplates

### 2. Prompts MCP
- Discovery automatico de prompts ao conectar
- Metodo `GetPrompt(slug, name, args)` para executar prompts
- Contagem e listagem de prompts por servidor
- Suporte a argumentos de prompts

### 3. Reconnect Automatico com Exponential Backoff
- Retry automatico quando conexao falha
- Exponential backoff (1s, 2s, 4s, 8s, 16s, max 5min)
- Maximo de 5 tentativas configuravel
- Tracking de `retryCount` por servidor

### 4. Health Checks Periodicos
- Goroutine dedicada por servidor conectado
- Ping a cada 30 segundos
- Timeout de 5 segundos por ping
- Atualizacao de `lastPing` timestamp
- Reconexao automatica em falhas

### 5. MCP Nativo

> **Superseded.** A secao original descrevia `GetNativeMCPServers()` e `convertToClaudeMCPFormat()` como mecanismo manual de passagem de info MCP para modelos. Essa abordagem foi substituida pela arquitetura capability-driven descrita em [AEP-0021](0021-mcp-native-mode.md) e [AEP-0037](0037-sdk-migration-chat-provider.md), onde o ChatProvider resolve MCP nativo server-side via `SupportsNativeMCP()` + `WithMCPServers()`.

---

## Referencias

- [AEP-0020: MCP Implementation](0020-mcp-implementation.md) — implementacao completa do protocolo
- [AEP-0021: MCP Modo Nativo](0021-mcp-native-mode.md) — arquitetura atual de MCP nativo
- [AEP-0037: SDK Migration + ChatProvider](0037-sdk-migration-chat-provider.md) — migracao para SDKs oficiais
