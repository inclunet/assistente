---
title: "Terminal"
weight: 3
---

# Terminal Integrado

O Assistente possui um terminal integrado com sessões persistentes, permitindo executar comandos sem sair do aplicativo.

## Como Funciona

O terminal cria sessões PTY (pseudo-terminal) reais usando o shell do sistema:

- **Windows**: PowerShell
- **macOS/Linux**: Bash (ou shell padrão)

Cada sessão mantém:

- Diretório de trabalho atual
- Histórico de comandos (até 200 entradas por sessão)
- Estado: Idle, Busy ou Closed
- Saída formatada

## Sessões

Múltiplas sessões podem existir simultaneamente (até 10 por padrão), cada uma em sua própria aba.

| Propriedade | Valor Padrão |
|---|---|
| Máximo de sessões | 10 |
| Timeout padrão | 30 segundos |
| Histórico por sessão | 200 entradas |

## Histórico de Comandos

Cada entrada no histórico registra:

- Comando executado
- Saída completa
- Código de saída
- Timestamps de início e fim
- Origem: `user` (digitado) ou `llm` (executado pelo assistente)

## Integração com IA

O assistente pode executar comandos no terminal como parte de suas ferramentas (tool calling). Quando o LLM usa a ferramenta de terminal:

- O comando aparece no histórico com origem `llm`
- A saída é capturada e retornada ao LLM
- O usuário pode acompanhar em tempo real

## Interface

A interface inclui:

- **Toolbar**: Nome da sessão, diretório atual, botão de interrupção
- **Histórico**: Lista de comandos e saídas
- **Input**: Campo para digitar novos comandos

## Atalhos

| Atalho | Ação |
|---|---|
| `Enter` | Executar comando |
| `Ctrl + C` | Interromper comando em execução |
| Deep link | `assistente://terminal/{id}` |
