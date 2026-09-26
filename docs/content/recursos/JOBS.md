---
title: "Jobs — Automação"
weight: 5
---

# Jobs (automação)

Jobs são automações do tipo "uma ferramenta por disparo": cada job chama uma tool do catálogo quando um gatilho acontece, com entradas fixas ou por template.

## Definições de saída e atalhos

A definição completa usada para identificar o job aceita até **1 MiB** de JSON
serializado, incluindo schema de saída, entradas e demais campos de execução. O limite vale tanto antes quanto depois da canonicalização JSON (que pode expandir números em notação científica).
Esse limite é da configuração; não define o tamanho máximo da resposta da tool.

Jobs sem hotkey não participam da montagem dos atalhos globais. Se a definição
de um job com hotkey for inválida ou exceder esse limite, os atalhos desse job
ficam indisponíveis e um aviso identifica o job no log. Os demais atalhos e o
envio de mensagens continuam disponíveis. Corrija a configuração e recarregue
os jobs para reavaliar seus atalhos; o conteúdo persistido não é apagado.

Veja também [Comandos e acionadores](../COMANDOS/).

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

Na atualização que introduz esse controle, jobs `subagent` antigos com um
`profile` explícito são desativados até que cada perfil necessário seja
autorizado. Jobs que omitem esse input e herdam o profile continuam inalterados.

## Navegação e ações pelo teclado

- Navegue pelas células da grade com as setas e pressione `Enter` para abrir os logs do job focado.
- Pressione `Delete` para excluir o job focado. A exclusão sempre pede confirmação.
- Jobs não usam seleção múltipla. Depois de excluir, ativar/desativar, executar ou fechar um menu/modal, o foco retorna a uma célula válida da grade.
- Se um filtro remover o job focado, a referência é reconciliada com a lista visível. Quando não houver jobs, o foco retorna a um controle utilizável da página.

## Gatilhos

- `cron` (expressão), `interval` (`a cada 30m`), `event` (reage a `on_success`/`on_failure` de outro job), `hotkey`, `manual` e `webhook`.
- Vários por job; campo `when` filtra o disparo.

## Execução e encadeamento

Jobs publicam eventos (`on_success`/`on_failure`) no barramento interno; outros
jobs com gatilho `event` reagem, formando pipelines. O histórico do run guarda
estado, trigger, duração, erro e a timeline operacional. Ao abrir os detalhes,
tool, entradas redigidas e saída são lidas da invocação técnica associada; não
há uma segunda cópia desses payloads no run. O replay em `dry-run` usa essa
invocação canônica e fica indisponível quando a entrada foi redigida.

Eventos não ficam em fila. Quando nenhum consumidor está habilitado — porque o
job ou a pipeline consumidora foi desativado — o evento é descartado e não
aparece no run como emitido. O log registra um aviso limitado por evento para
facilitar o diagnóstico sem repetir a mesma mensagem continuamente. Reative o
consumidor antes do próximo disparo do produtor para retomar o encadeamento.

### Origem dos eventos de tasklists

As mutações de tasklists usam uma entrada interna autenticada no desktop.
Quando há um job inscrito, o backend identifica a origem e vincula o disparo
ao usuário e à sessão atuais. Campos recebidos no payload não concedem essa
autoridade. Jobs encadeados preservam a origem, sem reiniciar o histórico.

Se a sessão mudar antes da execução, a origem antiga não autoriza ativação de
camadas de comandos. Isso não transforma todos os eventos do barramento em
eventos confiáveis nem concede permissões às custom actions. A publicação de
eventos de domínio continua best-effort: uma falha é registrada no log e não
desfaz a mutação da tasklist já concluída.
