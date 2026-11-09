# Investigação de polling e foco — issue #1062

Status: em investigação. Não há causa demonstrada para as três ocorrências
registradas na [#1062](https://github.com/inclunet/assistente/issues/1062).
Este registro não reabre o aceite da auditoria #1008 nem conclui o AEP-0103.

## Deck: capturar antes de encerrar

No job `128909061603` do run `42538657025`, o teste
`TestCommandDeckInputRefreshesEquivalentJobProjectionAfterWorkspaceTabSwitch`
falhou aguardando a publicação posterior à liberação da barreira. Os avisos
de `SQLITE_BUSY` posteriores ao timeout são do período de encerramento; não
demonstram o motivo da ausência de publicação.

Em 09/11/2026, 30 repetições locais passaram em Go 1.26/Windows. Outras 30
passaram com `-covermode=atomic -cpu=1,2 -count=15`, aproximando a instrumentação
do job original e variando o paralelismo. Isso não reproduz integralmente o
runner Linux nem demonstra correção. Nenhum ACP foi executado localmente.

O timeout preserva seus 5 s e sua assertion. Antes do `Fatal` e dos defers de
cancelamento, registra filas de status/renderização/entrada, cancelamento do
contexto e stacks das goroutines. Não faz consultas ao banco nem adquire locks
do produto para produzir o diagnóstico. Não altera o caminho de sucesso.
Escreve diretamente em stderr, sem depender do buffer de `t.Logf`: a captura
permanece na saída mesmo sem `-v`/`-json` se o prazo global vencer no cleanup.

Contraprova local: omitir temporariamente o `resume()` imediatamente antes da
espera fez a assertion original falhar; o dump capturou a goroutine do emissor
em `chan receive`, ligada a `runDeckEpoch`, antes do cleanup. A injeção foi
removida. Essa prova valida a captura, não explica o incidente histórico.

Na próxima falha, examinar primeiro as stacks e filas anteriores ao cleanup:
espera por gate/conexão, transporte/renderização, emissor ou ticker. Correlacionar
com os timestamps do JSONL publicado pelo CI antes de atribuir a causa a
contenção, perda de publicação ou atraso do runner. Não aumentar prazos.

## E2E: preservar a tentativa que falhou

Os artefatos do run `42517346349` contêm screenshot e contexto da primeira
falha, mas o único `trace.zip` pertence a `chromium-retry1`, que passou.
`on-first-retry` não registra a execução original e não permite identificar
quem tirou o foco de `Atualizar padrões` naquele episódio.

Somente `e2e/settings/command-managers.spec.ts` passa a usar
`trace: 'retain-on-failure'`. Todas as tentativas são gravadas, mas somente
falhas retêm trace. O restante da suíte conserva a política vigente. Não há
mudança de retry, timeout, seleção ou assertion. Há custo de captura limitado
a esse arquivo, sem alegação de redução de duração.

Contraprova local: inserir uma falha deliberada após a assertion de foco e
executar o cenário com `--retries=0` produziu `trace.zip` da primeira tentativa.
A injeção foi removida. Na recorrência, recuperar o trace da tentativa falha
e correlacionar navegação, fechamento de modal/menu e snapshots. Se isso não
identificar a origem, adicionar instrumentação direcionada; o trace por si só
não garante a stack de cada chamada interna de `focus()`.

## Defeito demonstrado no retorno adiado dos menus

Frames controlados reproduziram a restauração dupla no `MenuButton` (duas
chamadas à área padrão por seleção) e a sobrescrita de foco adquirido após
agendar o retorno no Menu/MenuButton. O caso sem nova navegação do Menu já
passava; os três outros casos falhavam antes da correção. Uma quarta
contraprova, com Modal real aberto pela ação, também falha no código anterior.

O agendamento protegido anteriormente local ao Modal passa a ser compartilhado
por Menu, MenuButton e Modal. A restauração não atua atrás de um modal aberto
nem sobre foco novo conectado; a notificação pós-seleção do MenuButton continua
uma vez. O MenuButton desabilita a restauração do Menu interno e mantém um único
responsável pelo retorno. A prova com um Modal real aberto pela ação também
exige que a área padrão da página não seja invocada.

Essa reprodução demonstra um defeito independente no mesmo percurso visual,
mas não identifica quem tirou o foco no run histórico `42517346349`. Tampouco
abrange todos os callbacks/timers dos demais consumidores do hook de menus.
Uma contraprova adicional demonstrou a perda de foco antes do callback de
10 ms do hook. O fechamento agora conclui após o commit que remove o menu do
DOM, sem timer arbitrário. Reabrir antes desse commit descarta a conclusão
anterior; desmontar não deixa timer pendente. Os callbacks pós-seleção e
pós-dismiss continuam com suas funções atuais, inclusive os que fazem handoff
de criação no Topbar. Novas provas cobrem DOM removido, reabertura e desmontagem.
O trace do incidente permanece necessário; não encerrar o item por essa inferência.

## Topbar e critérios ainda abertos

A ausência de commit na criação de terminal em `Topbar.palette.integration.test.tsx`
continua sem reprodução causal. Nenhuma alteração foi feita nesse contrato.
Reexecuções verdes não encerram esse item.

As capturas acima preparam evidência para a próxima ocorrência. Identificar o
bloqueio real do Deck, atribuir a mudança de foco e resolver ou delimitar a
ocorrência da Topbar continuam pendentes na #1062. Só causas demonstradas
devem gerar correções produtivas e contraprovas específicas.
