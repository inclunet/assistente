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

Use um ponto de exclamação seguido de um comando entre crases para declarar uma execução. A forma legada sem crases é reconhecida quando começa pelo nome de um programa declarado em `bashCommands.allowed`. Imagens, exclamações no texto e exemplos em blocos cercados por crases ou tis permanecem no Markdown, inclusive quando o bloco está dentro de uma lista. Exemplos em blocos HTML `<pre>` também preservam as linhas de comando como texto. O comando precisa
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

O orçamento da base reserva as instruções, os títulos e os arquivos de apoio antes de autorizar comandos. Se o espaço restante for insuficiente para os resultados, o Assistente conserva o texto que couber e omite as execuções. A skill base é preparada uma vez a cada tentativa de envio, antes do primeiro comando da skill por barra. Se o programa falhar, atingir o timeout de cinco segundos ou produzir saída excessiva, o Assistente omite a linha da base e conserva as demais instruções. Uma falha na execução da skill por barra interrompe a invocação. Em ambos os casos, uma recusa do usuário ou negativa vigente interrompe o turno.

O orçamento de **Slash Skill** reserva espaço para todas as instruções estáticas, os argumentos, a lista de arquivos e uma parcela da saída de cada comando. Quando essa reserva não cabe, a preparação termina antes da autorização. Uma saída que excede a parcela disponível aparece abreviada com reticências. Os comandos seguintes podem usar o espaço que sobrou dos anteriores. O recuo de exemplos segue o Markdown relativo ao item da lista. Uma cerca aberta se estende até o fim do bloco.

Mensagens de canais conectados também podem chegar à preparação do turno. Nas linhas de shell que usam argumentos, cada palavra começa com letra, número, ponto ou sublinhado e pode conter hífen e barra internamente. Escreva as opções fixas na própria skill. O Assistente valida os argumentos antes da autorização, inclusive nos canais externos. Os argumentos de caminho recusam segmentos de diretório pai. Se a confirmação expirar ou faltar um interlocutor, o comando fica sem execução. A skill base omite a linha e continua; a invocação por barra termina. O editor grava as permissões no formato estruturado `tools`, inclusive ao salvar uma skill importada que usava `allowed-tools`.
