---
title: "Slack"
weight: 5
---

# Slack (canal de comunicação)

Este guia explica como criar e configurar um bot Slack para uso no Assistente.

## 1) Criar o app
1. Acesse https://api.slack.com/apps
2. Clique em Create New App → From scratch
3. Escolha o workspace e confirme

## 2) Ativar Socket Mode
1. No app, abra Socket Mode
2. Ative Enable Socket Mode
3. Gere um App‑Level Token com scope connections:write
4. Copie o token gerado (xapp-...)

## 3) Criar o Bot Token
1. No app, abra OAuth & Permissions
2. Em Bot Token Scopes, adicione:
   - app_mentions:read
   - channels:history
   - channels:read
   - chat:write
   - files:read (obrigatório para baixar anexos de entrada; sem ele, só texto funciona)
   - files:write (obrigatório para enviar anexos de saída via upload)
   - groups:history
   - groups:read
   - im:history
   - im:read
   - im:write
   - mpim:history
   - mpim:read
   - mpim:write
   - users:read
3. Clique em Install to Workspace (e reinstale se adicionar scopes depois)
4. Copie o token gerado (xoxb-...)

## 4) Event Subscriptions
1. No app, abra Event Subscriptions
2. Ative Enable Events
3. Em Subscribe to bot events, adicione:
   - message.channels
   - message.groups
   - message.im
   - message.mpim
   - app_mention (opcional)

## 5) Configurar no Assistente
1. Abra Canais → Slack
2. Preencha:
   - Bot Token: xoxb-...
   - App Token: xapp-...
3. Habilite o canal

## 6) O que são xoxb- e xapp-
- xoxb-: Bot Token do Slack (token do bot da app)
- xapp-: App‑Level Token para Socket Mode (recebimento de eventos)

## 7) Dicas
- Para responder apenas quando for mencionado, use app_mention e ignore message.*
- Para usar mensagens privadas, mantenha im:* e mpim:*
- Sem `files:read`, o Assistente conecta e troca texto normalmente, mas ignora anexos de entrada (com aviso no log). Sem `files:write`, o envio de anexos falha.
- Após alterar scopes, reinstale o app no workspace para o Bot Token refletir as novas permissões.
