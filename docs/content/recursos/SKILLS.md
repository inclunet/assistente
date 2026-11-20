---
title: "Skills"
weight: 6
---

# Skills

Skills são módulos de instrução em `SKILL.md` que estendem o comportamento do assistente sem carregar estado. Cada skill tem nome, descrição de quando usar e conteúdo em Markdown.

## Descoberta e habilitação

O app varre `~/.assistente/skills/{slug}/SKILL.md` (prioridade `workdir > home > exe`). O perfil controla `enabled_skills` ordenada: a primeira entra no prompt como base, as demais aparecem como catálogo leve sob demanda; fora da lista fica desabilitada.

## Catálogo leve

O prompt inicial traz só catálogo (`slug`, nome, descrição) com orçamento; o corpo completo só entra quando a skill é ativada. Isso mantém o contexto enxuto.

## Slash commands

`/skill <nome> [args]` ativa explicitamente uma skill habilitada. O menu `/` lista só skills invocáveis pelo usuário. Com `toolCallingEnabled=false` o modelo não auto-ativa skills, restando só base + `/skill` manual.

No campo de mensagem do chat, digite `/` para abrir o menu de skills e comandos
disponíveis. Continue digitando para filtrar, use as setas para percorrer as
opções, `Enter` para escolher e `Esc` para fechar. O foco permanece no campo
durante a navegação, e a opção ativa é anunciada pelo leitor de telas.

## Gestão

Uma linha iniciada por `!` no corpo de uma skill base ou invocada por barra
pede a execução de um comando durante a preparação do turno. O comando precisa
estar no escopo da skill e o perfil precisa permitir a ferramenta de shell.
O Assistente consulta a política do comando no contexto da conversa. Quando
é preciso decidir, o diálogo de shell mostra o comando, o diretório e as
opções de duração disponíveis. Aprovar permite continuar. Recusar, cancelar
ou encontrar uma negativa vigente interrompe o turno antes do envio ao modelo.
Uma aprovação lembrada segue o mesmo escopo das demais execuções de shell.

Em **Skills**: grid com nome/descrição/modo (Auto/Manual) e origem; crie, edite, duplique ou remova skills. `Ctrl+N` cria um skill quando essa aba de Configurações está ativa.

Toda skill salva pela interface possui uma versão semântica no formato `X.Y.Z`. Novas skills começam em `1.0.0`; ao editar, a versão existente é preservada. Skills legadas sem `version` recebem `1.0.0` no formulário e passam a persistir esse valor quando forem salvas.

Built-ins incluem `coding`, `job-manager` e outras já sem templates.

### Comandos em skills base

Uma imagem Markdown permanece no conteúdo da skill. Nas skills base selecionadas para o turno, quando o perfil não disponibiliza o shell ou o programa solicitado fica fora da política da skill, o Assistente omite a linha de comando e mantém as demais instruções. Uma recusa de autorização interrompe o turno. Para usar um comando, a skill precisa permitir tanto o programa em `bashCommands.allowed` quanto a ferramenta `run_command`, caso declare uma lista de ferramentas permitidas. O editor avisa quando essas permissões entram em conflito. Se a origem impõe uma lista de linhas de comando, apenas as linhas autorizadas por ela podem ser executadas. Uma nova tentativa explícita consulta novamente a política vigente.

Para uma skill invocada por barra, o orçamento de contexto de **Slash Skill** precisa comportar o bloco de instruções, mesmo que o resultado do shell precise ser truncado. Com um orçamento menor, o Assistente interrompe a preparação antes de solicitar autorização ou executar o comando.
