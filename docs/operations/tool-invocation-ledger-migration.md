# Migração do ledger de invocações

Este runbook acompanha a AEP-0104. A migração é entregue em sete versões
empilhadas e não deve ser antecipada manualmente em bancos de usuário.

## Estados

- `pending`: schema aditivo presente e backfill ainda incompleto;
- `backfilled`: legado representado e validado no ledger;
- `canonical`: schema legado removido; rollback exige restaurar backup.

## Baseline

Execute o benchmark reproduzível antes e depois de cada mudança de projeção:

```powershell
go test ./internal/app -run '^$' -bench BenchmarkConversationMessageWindowBaseline -benchmem -count 5
```

Ele cobre 100, 500 e 1.000 mensagens com resultados de 1 KiB, 100 KiB e
10 MiB pelo caminho real de consulta SQLite, hidratação do ledger, montagem da
timeline e serialização Wails. Registre `ns/op`, `B/op`, alocações e
`window_bytes`. O gate de latência admite no máximo 20% de regressão sobre a
mediana da fase 1; janela e `turnPatch` finais admitem até 2 KiB adicionais por
resumo de invocação, sem output integral.

## Diagnóstico

O executável aceita:

- `--tool-ledger-log-level=info` (padrão);
- `--tool-ledger-log-level=debug`;
- `--tool-ledger-log-level=trace`.

Use `trace` somente em diagnóstico temporário. O nível elevado é limitado aos
componentes `toolinvocations.*` e `database.tool-ledger.*`; outros componentes
permanecem em `info`. Logs da migração registram IDs, fase, linhas, bytes e
duração, nunca conteúdo de conversa, input/output de tools, credenciais ou
paths retornados.

## Cutover

O cutover final só pode ocorrer quando não houver pendências ou ambiguidades,
contagens e hashes conferirem, `foreign_key_check` estiver vazio e
`integrity_check` retornar `ok`. O app cria e verifica backup antes do rebuild.
Após remoção física, o único downgrade suportado é restaurar esse backup.
