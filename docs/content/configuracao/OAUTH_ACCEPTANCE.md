---
title: "Validar autorizações e migração"
weight: 12
---

# Validar autorizações e migração

Este roteiro ajuda a conferir uma instalação depois da migração para credenciais
compostas. Os testes automatizados cobrem os protocolos e a persistência; estes
passos verificam a integração com suas contas e a interface da máquina utilizada.

Registre a versão ou commit do Assistente, o sistema operacional e o resultado de
cada cenário: **passou**, **falhou** ou **não executado**. Uma conexão bem-sucedida
não comprova, sozinha, envio de mensagens ou renovação do token.

## Conta ChatGPT

Use uma conexão configurada conforme [Provedores LLM](../PROVIDER_CONFIGURATION/).

1. Abra o provedor e confira o estado conectado. No perfil de chat, selecione esse
   provedor, carregue o catálogo e escolha um modelo disponível para sua conta.
2. Crie uma conversa e envie uma mensagem curta. Confira que a resposta conclui
   e que o perfil continua usando o provedor escolhido.
3. Use uma ferramenta local de somente leitura disponível no perfil. Confira que
   ela executa e que o chat apresenta o resultado.
4. Feche e abra o Assistente, entre na mesma conta local e repita catálogo e envio.
   O reinício não deve abrir consentimento no navegador por conta própria.
5. Quando o token precisar de renovação, repita uma chamada. Um envio antes da
   expiração não prova refresh. O evento `oauth_refresh`, com integração, motivo
   e resultado, ajuda a confirmar a renovação; registre somente esses campos,
   sem copiar credenciais ou o log completo.
6. Use **Autorizar novamente** para a mesma conta e confirme o envio. Para testar
   outra conta/workspace, crie outra conexão, selecione-a no perfil e confirme
   que as duas permanecem independentes. Reconectar a mesma autorização preserva
   sua identidade; não use essa ação para trocar silenciosamente sua conta.
7. Durante uma resposta, teste o cancelamento pela interface. Se ocorrer limite
   do plano, timeout ou interrupção de rede, registre a mensagem apresentada e
   confira que o app encerra a operação. Falhas que não ocorreram ficam como
   não executadas; não é necessário consumir a cota para provocá-las.

## MCP Slack e Atlassian

Siga [MCP — Exemplos](../MCP_CONFIG_EXAMPLES/) para a configuração do serviço.
Slack MCP e o canal Slack são integrações diferentes: testar uma não valida a outra.

1. Para um cadastro histórico, abra o diagnóstico OAuth e crie um snapshot.
   Em PKCE incompleto, escolha **Reconectar e migrar**. Em Client Credentials,
   confirme o método de autenticação antes de **Converter autorização**.
2. Conclua o consentimento quando solicitado. Confira o estado do servidor e
   execute uma ferramenta de somente leitura autorizada na sua conta.
3. Feche e abra o Assistente. Reconecte o servidor, se necessário, e repita a
   ferramenta. Confirme que a conexão usa a autorização migrada e não pede
   consentimento espontaneamente ao iniciar.
4. Repita uma chamada quando houver renovação necessária e registre seu resultado.
   Se a autorização for recusada definitivamente, a interface deve orientar
   reautorização explícita, sem abrir várias janelas por conta própria.
5. Confira o callback conforme o cadastro: Slack com cliente manual deve manter
   a URL/porta registrada; Atlassian com DCR deve conservar os dados efetivos do
   registro. Não considere sucesso uma troca manual de porta que esconda perda
   desses dados após reinício.
6. Teste o caminho local de ferramentas. Se também usar MCP nativo com um provedor
   que o suporte, repita nele a ferramenta de leitura e registre os dois caminhos.
   A conexão ChatGPT usa o adaptador local; ela não oferece MCP hospedado nativo.
7. Em uma tentativa explícita de reconexão, cancele o consentimento e confira que
   o cadastro e o snapshot continuam disponíveis. Um snapshot local preserva
   dados para recuperação; não desfaz uma revogação realizada pelo provedor.

## Canal Slack

Use uma conexão e conversa de teste de acordo com [Canal Slack](../SLACK_CHANNEL_SETUP/).

1. Salve ou conecte um cadastro histórico com os dois tokens existentes. A migração
   deve reunir os componentes sem pedir que você recopie os tokens.
2. Envie uma mensagem de teste ao Assistente pelo Slack e confira o recebimento
   por Socket Mode. Confira também a resposta enviada pela API do Slack.
3. Feche e abra o Assistente, reconecte o canal e repita a troca de mensagens.
4. Abra o editor, deixe os campos de token vazios e salve. Repita o teste: campos
   vazios preservam os componentes existentes.
5. Se fizer parte da sua rotina testar restauração, use a exportação com
   credenciais e senha, em uma instalação de teste compatível. Após restaurar,
   revise as configurações do canal, habilite a conexão e repita API/Socket Mode.
   Marque a restauração como não executada se esse passo não foi realizado.

## Teclado, leitor de telas e diagnóstico

Nos diálogos usados acima, navegue por Tab, confirme os rótulos e a ordem das
ações, cancele quando permitido e confira a restauração do foco. Com NVDA,
confira anúncios de conexão, falha e cancelamento. Registre o idioma utilizado;
os testes automatizados também conferem mensagens em português, inglês e espanhol.

Ao relatar uma falha, informe cenário, versão/commit, plataforma e mensagem
visível. O esperado é receber orientação compreensível, sem tokens, client
secrets, códigos de autorização ou respostas privadas do provedor. Não inclua
URLs completas de consentimento, arquivos de backup ou o conteúdo do cofre.

Um registro de resultado pode ser: “commit X, Windows, ChatGPT: catálogo/envio/
ferramenta/reinício passaram; refresh não observado; reconexão passou; idioma
pt-BR, navegação e anúncios conferidos com NVDA”. Isso permite concluir apenas
os aceites efetivamente testados e manter os demais pendentes.
