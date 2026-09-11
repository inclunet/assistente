---
title: "Jobs — Automação"
weight: 5
---

# Jobs (automação)

Jobs são automações do tipo "uma ferramenta por disparo": cada job chama uma tool do catálogo quando um gatilho acontece, com entradas fixas ou por template.

## Conceito

- Cada job tem nome, pipeline opcional, entradas e política de erro (`retry`/`skip`/`stop`).
- A estratégia `retry` repete falhas transitórias até `max_retries`; falhas
  permanentes classificadas pela tool, como argumentos/configuração inválidos,
  profile indisponível ou autorização sem interlocutor, encerram o run na
  primeira tentativa.
- Armazenado no SQLite, com retenção de 30 dias e isolado por usuário.
- Gerenciado em **Jobs**: grid com busca, criar/editar, ativar/desativar, ver logs e timeline.

## Subagentes com perfil específico

Quando a tool de um job é `subagent` e o input `profile` aponta para um perfil
específico, o aplicativo pede uma autorização explícita no desktop. A permissão
vale somente para aquele usuário, job, perfil alvo e configuração da expressão
de `profile`.

- Profile literal: ao salvar ou ativar sem permissão, confirme uma vez. Se
  recusar ou fechar o diálogo, o job fica salvo desativado.
- Profile por template: abra **Perfis autorizados para este job** no editor e
  autorize cada perfil instalado separadamente. O valor recebido de um evento
  nunca é liberado automaticamente. Se o template resolver para vazio, a
  execução falha em vez de herdar outro perfil.
- Se você alterar a expressão de `profile` no editor, salve o job antes de
  autorizar ou revogar perfis; os controles ficam indisponíveis enquanto o
  rascunho diverge da configuração salva.
- Use **Revogar** ao lado de um perfil para impedir execuções futuras. Uma
  execução que já começou não é interrompida.
- Alterar a expressão de `profile`, excluir o job ou excluir o perfil revoga a
  permissão aplicável. Alterações de nome, descrição ou prompt não pedem nova
  autorização.

Importar ou duplicar um job nunca importa permissões. Sem um grant válido,
disparos cron/event/headless falham antes de criar conversa ou run de subagente
e não abrem diálogo.

Na atualização que introduz esse controle, jobs `subagent` antigos são
desativados até que cada perfil necessário seja autorizado explicitamente.

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
