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

Em **Skills**: grid com nome/descrição/modo (Auto/Manual) e origem; crie, edite, duplique ou remova skills. `Ctrl+N` cria um skill quando essa aba de Configurações está ativa.

Toda skill salva pela interface possui uma versão semântica no formato `X.Y.Z`. Novas skills começam em `1.0.0`; ao editar, a versão existente é preservada. Skills legadas sem `version` recebem `1.0.0` no formulário e passam a persistir esse valor quando forem salvas.

Built-ins incluem `coding`, `job-manager` e outras já sem templates.
