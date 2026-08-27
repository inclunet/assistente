---
title: "Agentes ACP"
weight: 8
---

# Agentes como provedores (ACP)

O Assistente pode usar **agentes de código locais** como provedores de IA, via Agent Client Protocol (ACP) sobre stdio. Cada agente (Cursor, OpenCode, Copilot, Claude) roda como processo na sua máquina e é selecionável por perfil como qualquer provedor HTTP.

## O que é

- **Transporte**: JSON-RPC sobre `stdio` (`coder/acp-go-sdk`). Um processo por provedor, sessões multiplexadas por conversa.
- **Sessão = conversa**: o histórico fica no agente; retomar conversa tenta `session/load`, falhando cria nova sessão com aviso.
- **Sem persona/skills do app**: o turno leva só a mensagem do usuário e anexos compatíveis.

## Instalar via catálogo

1. **Configurações → Provedores → Adicionar → Agente ACP**
2. O catálogo oficial (~38 agentes) mostra estado (`instalado`, `não instalado`, requisito faltando) e integridade (digest).
3. Escolher o agente preenche `comando`, `args` e `env`; a instalação baixa o binário versionado. Sem rede, o modo manual (comando + args) continua.

## Criar manualmente

Informe `comando` (ex.: `node`, `powershell -File agent.ps1 acp`), `args` e `env`. `URL` não é necessária quando `api_format=acp`.

## Modelos e opções

Modelos vêm da sessão de descoberta do agente (cache por provider). A troca usa `set_config_option`/`set_model` conforme o agente.

## Permissões

Todo `request_permission` do agente vira um questionário na UI (ou mensagem numerada em canais). `Permitir sempre` grava por **classe** (`execute`/`edit`/`read`) em `.assistente/acp-permissions/<perfil>.json`. Gerencie em **Configurações → Permissões do Agente**; canais e jobs nunca gravam "sempre".

## Limitações

- Não leva persona/skills/memória do app; não exporta tools MCP.
- Não gerencia credenciais HTTP; `acp_credential_env` injeta segredos do cofre se necessário.
- Cancelamento é serial (1 turno/sessão por vez).
