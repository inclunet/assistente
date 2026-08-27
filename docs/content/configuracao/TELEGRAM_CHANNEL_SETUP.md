---
title: "Telegram"
weight: 6
---

# Telegram (canal de comunicação)

Este guia explica como criar e configurar um bot Telegram para uso no Assistente.

## 1) Criar o bot

1. Abra o Telegram e converse com [@BotFather](https://t.me/BotFather)
2. Envie `/newbot`, escolha nome e username (terminado em `bot`)
3. Copie o token gerado (formato `123456:ABC-...`)

## 2) Ajustar privacidade

Por padrão o bot só recebe mensagens que começam com `/` em grupos. Para que o Assistente veja todas as mensagens do grupo:

1. No @BotFather, envie `/mybots` → escolha o bot → **Bot Settings** → **Group Privacy** → **Turn off**
2. Remova e readicione o bot ao grupo para a mudança valer

## 3) Configurar no Assistente

1. Abra **Canais → Telegram**
2. Preencha **Bot Token** com o token do @BotFather
3. Habilite o canal
4. Envie uma mensagem para o bot (ou adicione-o a um grupo) — o Assistente cria a conversa automaticamente

## 4) Dicas

- Use `/setprivacy` no @BotFather para alternar entre responder só a menções ou a todas as mensagens do grupo.
- Sem token válido o canal fica desconectado; o log mostra `telegram: token inválido`.
- O histórico do Telegram respeita `max_history` do canal; anexos seguem o mesmo fluxo de mídia do Slack (com `max_contacts` para contatos).
