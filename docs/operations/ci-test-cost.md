# Custo de testes e CI — issue #1008

## Baseline e escopo

O [run 42487226598](https://github.com/inclunet/assistente/actions/runs/42487226598)
do PR #1040 teve 28 jobs, 23,10 minutos de duração total e 158,17 minutos
somados de execução dos jobs. Esta soma não é faturamento. Dos 28 jobs,
20 executaram partições de race; seus passos de testes somaram 113,40 minutos,
incluindo descoberta/compilação. Não se atribui todo esse tempo aos testes.
O grupo `comandos-interface` executou 56 testes top-level em 594,313 segundos
no Go; começou 10,20 minutos depois da criação do run. A demora de início
inclui o escalonamento da matriz, não prova indisponibilidade de runners.

As contagens e tempos vêm de `gh api repos/inclunet/assistente/actions/runs/42487226598/jobs?per_page=100`
e dos logs dos jobs. Duração total: último `completed_at` menos `created_at`
do run. Minutos agregados: soma de `completed_at - started_at` dos jobs.
Reexecuções devem ser separadas por tentativa; não misturar sua espera com
o tempo de uma tentativa inicial.

## Primeiro recorte: preparação da execução contextual

O perfil local de `TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff`
mostrou repetição de `MutateCommandSettings`, projeção do catálogo e
serialização/fingerprints. As quatro mutações confirmadas eram preparação
repetida para cada comando, não o comportamento sob teste.

A fixture agora insere camada, regra `always` e dois bindings disjuntos numa
transação do banco exclusivo do cenário. Incrementa a geração e chama a
reconstrução real do lifecycle: leitura, validação, projeção e publicação
continuam no percurso. App, banco, sessão, workspace e limpeza continuam
independentes por cenário. Não há cache no produto ou estado global novo.

### Garantias preservadas

- `TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff`: todos os IDs
  anteriores percorrem oferta física → Begin → Take → Complete; recusa de replay
  da oferta e do handoff; ausência de invocação antecipada; exatamente uma
  invocação persistida por comando e origem `streamdeck.key`.
- Outros usuários de `newContextualDeckTestFixture`: recusas de contexto,
  invalidação, cancelamento, expiração e limpeza continuam com as mesmas
  assertions. Nenhuma condição de sucesso foi relaxada.
- `TestCommandDeckVisualConditionsFromSettingsSelectDistinctCommands` e
  fixtures de camadas/páginas mantêm edição confirmada → publicação → input.
- `TestCommandSettingsContractContextualRuleActuallyResolves` e a suíte de
  segurança de settings mantêm confirmação, CAS e reautenticação.
- Frontend: `npm run build` continua executando `tsc && vite build`; foi removida
  apenas a execução anterior idêntica de `tsc --noEmit` no mesmo job. O
  `tsconfig.json` já define `noEmit`. O typecheck de bindings recém-gerados
  permanece em seu job, pois verifica outra entrada. O build fica logo após
  a instalação, preservando a recusa de erros de tipos antes da suíte Vitest.

### Comparação reproduzível

Compilar o pacote `internal/app` nas duas revisões com o mesmo Go e executar
os binários a partir de `internal/app`, alternando baseline e alteração três
vezes, com os argumentos:

```text
-test.run=^TestCommandEditorFormatFamilyContextualDeckUsesCommonHandoff$ -test.v -test.timeout=5m
```

Seguir AGENTS.md: binários, caches e temporários sob `build/bin/` do worktree,
com GOTMPDIR/GOCACHE/TEMP/TMP/TMPDIR configurados antes da compilação.
Não executar ACP localmente. Separar esta medição de compilação e de outras
suítes concorrentes. O baseline é `55de67f0e` (conteúdo de App idêntico ao
binário de `f327e03e6`). Medições locais usam Windows e Go 1.27.1; não representam
uma previsão de tempo do runner Linux com race/Go 1.26.

Três pares locais concluídos com sucesso em 08/11/2026, sem compilação ou
outra suíte iniciada pelo agente concorrendo com a medição:

- Par 1: baseline 37,38 s; alteração 9,10 s.
- Par 2: baseline 41,70 s; alteração 9,48 s.
- Par 3: baseline 34,20 s; alteração 8,09 s.

Medianas: **37,38 s → 9,10 s**, redução de **75,7%** neste teste. Mesmos
subtestes e assertions, três execuções aprovadas em cada versão. Um ensaio
anterior concorreu com instalação/compilação e não integra esta comparação.
O ganho do workflow completo precisa ser medido no CI; não extrapolar esta
porcentagem para a suíte nem para race.

## Próximas frentes da issue

A issue permanece aberta: verificar cache útil/atualização da compilação,
reaproveitamento entre partições, distribuição da matriz, esperas reais,
ambiente DOM em testes puros, seleção por impacto e cancelamento/diagnóstico.
Cache já é restaurado pelos jobs de Go medidos: habilitá-lo novamente não
explica nem resolve o custo de execução. Não reduzir grupos antes de medir
o efeito sobre o caminho crítico, nem usar resultados antigos como aprovação.
