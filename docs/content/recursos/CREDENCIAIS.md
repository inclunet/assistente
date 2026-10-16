---
title: "Credenciais"
weight: 16
---

# Credenciais

Chaves de provedores ficam no cofre criptografado (DEK + pepper), isoladas por usuário. A perda de credenciais é defensiva: sem DEK o cofre não abre e o app pede novo login sem expor segredos. Gerencie em **Configurações → Provedores**.


## Abertura do aplicativo

Enquanto o Assistente inicializa o banco e os serviços, a tela de autenticação
mostra uma espera e continua automaticamente quando a inicialização termina.
Não é necessário clicar repetidamente para entrar. A sessão salva é verificada
normalmente; uma sessão inválida ainda exige login. Se a inicialização falhar,
a interface informa a indisponibilidade, sem abrir o aplicativo parcialmente.

## Fontes e autorizações

Em **Configurações → Credenciais**, cadastre valores salvos ou fontes externas
(env, keyring e comando). Os mesmos campos estão disponíveis ao configurar a
credencial de um MCP. Consulte [fontes de credenciais](../configuracao/CREDENTIAL_SOURCES.md)
para exemplos e detalhes do cache de comando.

A lista também apresenta autorizações OAuth de MCP/ChatGPT e conexões compostas
do Slack. **Visualizar → Configurar autorização** abre o cadastro vinculado para
alterar cliente, permissões ou reconectar pelo fluxo da integração. Os segredos
permanecem no cofre e não são preenchidos de volta no formulário. A lista mostra
registros armazenados; não testa conexões nem confirma a validade dos tokens.

Para nova autorização, escolha a fonte **OAuth gerenciado** e o destino. MCP
abre seu cadastro com descoberta automática. Em Provedores, use a ação de
conectar a conta ChatGPT. A autorização será gravada no mesmo CredManager.

Esta apresentação não exige novo login ou migração das autorizações compostas.
Cadastros antigos ainda pendentes usam o diagnóstico de migração e os snapshots
existentes. Um registro ilegível ou um vínculo removido produz um diagnóstico;
o aplicativo não cria uma autorização substituta silenciosamente.
