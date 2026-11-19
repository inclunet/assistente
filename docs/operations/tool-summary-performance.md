# Medição da projeção de ferramentas — issue #776

A consulta antiga já foi substituída no PR #944: migração v39, vínculos
canônicos e índice por usuário/origem/turno. Esta medição usa diretamente
`DBRepository.LoadSummariesForTurnIDs`; não introduz uma segunda consulta de
produção nem altera o banco do usuário.

## Como reproduzir

Antes de executar, configure GOTMPDIR, GOCACHE, TEMP, TMP e TMPDIR sob
`build/bin/agent-build` do worktree, conforme AGENTS.md. Não execute ACP local.

```powershell
go test ./internal/toolinvocations -run '^$' -bench '^BenchmarkToolSummaryProjection$' -benchtime=20x -count=1
```

O cenário sintético cria SQLite em disco com 1.000 ou 100.000 invocações,
resultados de aproximadamente 4 KiB e janela fixa de 20 resumos. Preparação,
schema e inserção não entram no tempo medido. O índice corresponde à v39;
a correção da migração e o plano indexado da consulta real são cobertos por
`TestCanonicalHistoryReadsUseScopedIndex` e `migration_chat_tool_links_test.go`.
O teste de isolamento de usuários permanece em `projection_read_test.go`.

Opcionalmente, medir os 20 turnos com invocações mais recentes de um banco
existente, aplicando os filtros de status e execução visível da projeção antes
do limite (somente o usuário do turno mais recente é selecionado):

```powershell
$env:ASSISTENTE_PROJECTION_BENCH_DB = Join-Path $env:USERPROFILE '.assistente/conversations.db'
go test ./internal/toolinvocations -run '^$' -bench '^BenchmarkToolSummaryProjectionReadOnly$' -benchtime=100x -count=1
Remove-Item Env:ASSISTENTE_PROJECTION_BENCH_DB
```

O banco é aberto com `mode=ro` e `query_only`, sem escritas SQL, migração,
VACUUM, checkpoint explícito ou manutenção. Isso não promete zero alterações
em arquivos auxiliares: SQLite pode coordenar leitores pelo WAL/SHM e locks.
A seleção da amostra agrupa e ordena o ledger antes do cronômetro; pode fazer
uma varredura custosa e aquecer páginas. São até 20 turnos, pois turnos de
outros usuários são descartados. Execute em momento de baixa atividade;
não há teste de retenção concorrente e não se usa `immutable=1` em banco ativo. Nenhum conteúdo, ID de usuário ou conversa é
impresso. O cenário é ignorado quando a variável não está definida.

## Resultado de 18/11/2026

Windows/amd64, Go 1.27.3, Intel Core Ultra 7 155H, código baseado na main
`a0552de66`. Execução local sem testes ACP:

- 1.000 invocações: 20 resumos, média **0,973 ms**, p95 **1,348 ms**,
  primeira leitura **1,572 ms**, 126.968 B/op.
- 100.000 invocações: 20 resumos, média **1,579 ms**, p95 **2,708 ms**,
  primeira leitura **2,046 ms**, 128.027 B/op.
- Banco existente, arquivo principal de aproximadamente 629 MiB mais WAL:
  180 resumos, 100 repetições, média **7,000 ms**, p95 **8,817 ms**,
  primeira leitura **8,987 ms**, 818.417 B/op.

Esses cenários ficaram abaixo de 200 ms. São evidência para o problema de
projeção da #776, não um SLA para toda consulta SQL, abertura do chat ou toda
máquina. A primeira leitura não equivale a cache de disco frio: preparação e
seleção da amostra aquecem páginas. Não foi provocada retenção concorrente no
banco pessoal. p95 usa até as primeiras 1.024 iterações; os comandos acima
medem todas as suas 20/100 iterações. O fixture sintético cobre histórico
canônico legado; o banco existente exercita os dados atuais e a seleção por
execução. A medição não compara tempos com a versão antiga, nem justifica
remover guardas de isolamento ou otimizar outra consulta.

`TestBenchmarkSampleSkipsUnselectedExecutionsBeforeLimit` prova que 21 turnos
recentes com invocações de execuções não selecionadas não excluem da amostra
um turno anterior que contém resumos visíveis. O ensaio real foi repetido
após essa correção de seleção; os resultados acima são dessa repetição.
