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

Os dois tokens são guardados juntos no cofre do Assistente, em uma credencial
da conexão. O Bot Token continua sendo usado pela API e o App Token pelo Socket
Mode; não há login OAuth ou renovação automática para esses tokens estáticos.

Ao editar, deixe um campo vazio para manter seu token atual. É possível trocar
somente um dos tokens sem perder o outro. Remover um token desativa o canal e
preserva o outro componente; informe novamente o token removido antes de habilitar.
Os valores armazenados não são enviados de volta ao formulário.

Cadastros antigos com as duas referências padrão são reunidos atomicamente ao
salvar ou conectar. Se a conversão falhar, a configuração e os segredos anteriores
permanecem intactos. Referências personalizadas ou compartilhadas exigem revisão
manual; o Assistente não apaga credenciais que podem atender outra conexão.

Para guardar uma cópia recuperável, use a exportação de Dados, marque a opção
de incluir credenciais e informe uma senha. Os tokens ficam juntos dentro do
bloco cifrado; uma exportação sem essa opção não os inclui. Guarde a senha para
restaurar o arquivo em uma versão compatível com credenciais compostas.
Na restauração, uma conexão nova começa desativada. Se já houver um Slack
cadastrado, escolha explicitamente se deseja sobrescrever a credencial.
Esse backup de credenciais não inclui os contatos e as demais configurações do
canal; revise-os e reconecte o Slack após restaurar.

Backups antigos com tokens separados também podem ser restaurados: o Assistente
os reúne na credencial da conexão após a decisão de conflito. Se o backup trouxer
apenas um token, o outro é preservado. Caso a entrada atual tenha sido perdida
ou esteja ilegível, use um backup completo ou informe novamente os dois tokens
no editor; uma alteração parcial não consegue recuperar um segredo perdido.

## 6) O que são xoxb- e xapp-
- xoxb-: Bot Token do Slack (token do bot da app)
- xapp-: App‑Level Token para Socket Mode (recebimento de eventos)

## 7) Dicas
- Para responder apenas quando for mencionado, use app_mention e ignore message.*
- Para usar mensagens privadas, mantenha im:* e mpim:*
- Sem `files:read`, o Assistente conecta e troca texto normalmente, mas ignora anexos de entrada (com aviso no log). Sem `files:write`, o envio de anexos falha.
- Após alterar scopes, reinstale o app no workspace para o Bot Token refletir as novas permissões.
