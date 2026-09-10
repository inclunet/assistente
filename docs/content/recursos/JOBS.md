---
title: "Jobs — Automação"
weight: 5
---

# Jobs (automação)

Jobs são automações do tipo "uma ferramenta por disparo": cada job chama uma tool do catálogo quando um gatilho acontece, com entradas fixas ou por template.

## Conceito

- Cada job tem nome, pipeline opcional, entradas e política de erro (`retry`/`skip`/`stop`).
- Armazenado no SQLite, com retenção de 30 dias e isolado por usuário.
- Gerenciado em **Jobs**: grid com busca, criar/editar, ativar/desativar, ver logs e timeline.

## Navegação e ações pelo teclado

- Navegue pelas células da grade com as setas e pressione `Enter` para abrir os logs do job focado.
- Pressione `Delete` para excluir o job focado. A exclusão sempre pede confirmação.
- Jobs não usam seleção múltipla. Depois de excluir, ativar/desativar, executar ou fechar um menu/modal, o foco retorna a uma célula válida da grade.
- Se um filtro remover o job focado, a referência é reconciliada com a lista visível. Quando não houver jobs, o foco retorna a um controle utilizável da página.

## Gatilhos

- `cron` (expressão), `interval` (`a cada 30m`), `event` (reage a `on_success`/`on_failure` de outro job), `hotkey`, `manual` e `webhook`.
- Vários por job; campo `when` filtra o disparo.

## Execução e encadeamento

Jobs publicam eventos (`on_success`/`on_failure`) no barramento interno; outros jobs com gatilho `event` reagem, formando pipelines. Logs registram `resolved_inputs`, saída e erro, com replay em `dry-run` para testar sem efeito.
