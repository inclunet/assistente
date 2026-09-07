---
title: "Signal"
weight: 7
---

# Signal (canal de comunicação)

Este guia explica como configurar o canal Signal no Assistente.

## 1) Pré-requisito: signal-cli

O canal Signal depende do [`signal-cli`](https://github.com/AsamK/signal-cli) vinculado a um número de telefone.

1. Instale `signal-cli` e vincule: `signal-cli link` ou `signal-cli register` + `verify`
2. Confirme que `signal-cli --version` funciona no mesmo usuário que roda o Assistente

## 2) Configurar no Assistente

1. Abra **Canais → Signal**
2. Informe o número vinculado e o caminho do `signal-cli` se necessário
3. Habilite o canal — o Assistente passa a receber mensagens via `signal-cli daemon`/`jsonRpc`

## 3) Dicas

- Signal exige número real; mensagens de grupo precisam que o bot esteja no grupo.
- Sem `signal-cli` disponível, o canal fica desconectado com aviso no log.
- Assim como Telegram e Slack, o canal cria conversas automaticamente na primeira mensagem recebida.
