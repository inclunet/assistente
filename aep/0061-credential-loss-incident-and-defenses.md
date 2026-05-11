# AEP-0061 — Incident report: perda de credenciais no boot AEP-0052 e defesas em profundidade

**Status:** Aceito
**Data:** 2026-05-10
**Autor:** Engenharia Assistente
**Relacionado:** AEP-0046 (UUIDv7), AEP-0052 (multi-user accounts)

## Resumo

Em 10/05/2026, durante o primeiro boot da release AEP-0052 (multi-user
accounts), 13 das 14 credenciais persistidas em `credential_entries`
desapareceram do banco do usuário entre o último checkpoint conhecido
(16:45) e a primeira criação do admin (~21:32). Apenas as três rows
`internal-auth:*` e uma row `mcp-tokens:glean` (com `created_at`
reescrito para 21:32:20) sobreviveram. A chave do provedor LiteLLM
(`ist-prod-litellm.nullmplatform.com`), entre outras, ficou ausente; o
runtime falhou com `credencial gerenciada não resolvida para pattern
"ist-prod-litellm.nullmplatform.com" e usuário "019e14c3-…"`.

A recuperação foi feita restaurando 13 rows do backup
`conversations.db.pre-uuid.bak` (gerado em 27/04 pela migração UUIDv7),
após confirmar que os wraps `master`/`recovery` da DEK eram idênticos
nos dois bancos. Este AEP documenta o incidente, registra a investigação
e congela as defesas adotadas para impedir recorrência.

## Cronologia (forensic)

- **27/04/2026** — migração UUIDv7 (AEP-0046) gera `conversations.db.pre-uuid.bak`.
  - 14 credenciais user-scoped/instance + 2 wraps de DEK presentes.
- **10/05/2026 16:45:03** — checkpoint do WAL na base ativa: 14 credenciais
  ainda intactas, schema pré-AEP-0052 (sem `user_id` em `credential_entries`).
- **10/05/2026 ~21:32:20** — primeiro boot com binário AEP-0052.
  - `dedupCredentialEntriesBeforeMigrate` é noop (sem `user_id` na tabela).
  - `AutoMigrate` adiciona `user_id` (NULL) e cria
    `ux_credential_entries_user_pattern (user_id, pattern)`.
  - `mcp-tokens:glean` é reescrito com `created_at=21:32:20` (UPSERT).
  - **13 demais rows desaparecem antes da criação do admin.**
- **10/05/2026 23:00:16** — `CreateAdminUser` insere o primeiro admin.
- **10/05/2026 23:12:53** — commit `b72060ae` corrige `AdoptLegacyData`
  (deleta órfãs antes do UPDATE para evitar `UNIQUE constraint failed`).
  O usuário já havia trombado no bug original.
- **10/05/2026 23:39:35** — backup `before-cred-restore` snapshot do
  estado ruim (13 credenciais ausentes).

## Investigação

A reprodução determinística (ver `internal/database/credential_loss_repro_test.go`)
mostra que o pipeline canônico do boot AEP-0052 sobre o estado de 16:45
**preserva todas as 14 credenciais**: dedup → `AutoMigrate` →
`ensureCredentialEntryUserPatternIndex` → `CreateAdminUser` →
`AdoptLegacyData` finaliza com 14 rows, todas com `token_enc`/`client_secret_enc`
intactos e `user_id` corretamente atribuído.

O wipeout, portanto, foi disparado por um caminho fora do pipeline
canônico. As superfícies auditadas como suspeitas:

1. **`controllers/SettingsController.ClearAllCredentials` e
   `internal/config/SettingsService.ClearAllCredentials`** — ambos
   invocavam `credMgr.DeletePattern(ctx, "")`. Hoje o `Manager` rejeita
   `pattern==""`, mas o erro era engolido como string genérica e o
   chamador podia ter sido disparado por wizard/botão durante o setup
   inicial. Mesmo sem causar perda real (o Manager rejeitava), a forma
   do código era exatamente o vetor que dois reviews de código teriam
   pegado caso houvesse teste de contrato.
2. **Bug histórico em `migrateToUUIDv7`** (commit `5d3d7eb9`, corrigido
   em `227f0333` em 27/04) que dropava colunas `auth_type` / `token_enc`
   ao recriar a tabela durante a migração de PK INTEGER → UUID. Bases
   que rodaram aquela versão chegavam ao boot AEP-0052 com rows já
   parcialmente vazias.
3. **Welcome Wizard / Vault Setup** com um eventual caminho de "começar
   do zero" que dispara `ClearAllCredentials` antes do primeiro login.

A decisão é: **não vamos perseguir mais o callsite exato porque o estado
da telemetria no momento do incidente não permite reconstruir com 100%
de certeza**, e as defesas adotadas neste AEP impedem todas as classes
de bug acima de causarem perda silenciosa novamente.

## Decisões e fixes adotados

### D1. `DBStore.DeleteCredential` falha fechado em `pattern==""`

`internal/credentials/db_store.go` agora retorna `ErrEmptyPatternDelete`
quando o caller passa pattern vazio (ou apenas whitespace). Sem a
defesa, qualquer refator que mude `WHERE pattern = ?` para suportar
wildcards/SQL building viraria vetor de mass-delete acidental. Cada
chamada bem-sucedida loga `pattern`, escopo (`user=…` ou `instance`) e
`RowsAffected`. `RowsAffected > 1` emite WARN: pattern é exato, então o
único cenário esperado é uma órfã legacy coexistindo com a claimed.

### D2. `DBStore.SaveCredential` falha fechado em `pattern==""`

Simétrica de D1. Sem ela, uma row "fantasma" pode virar target de
qualquer `WHERE pattern = ''` futuro.

### D3. `ClearAllCredentials` itera por pattern em vez de passar string vazia

`controllers/SettingsController.ClearAllCredentials` e
`internal/config/SettingsService.ClearAllCredentials` agora:

1. Exigem `RequireUserID` (não há "limpar credenciais sem dono" pela UI).
2. Listam credenciais visíveis ao usuário (`ListVisibleCredentials`,
   que já filtra `internal-auth:*` / `internal-tls:*` e credenciais de
   outros usuários).
3. Iteram chamando `DeletePattern(ctx, p)` por pattern.
4. Propagam erro do primeiro delete que falhar.

Resultado: o caminho público nunca mais consegue formar uma chamada
"limpa tudo de uma vez" — cada delete é nominal, escopado e logado.

### D4. Contrato `CredentialCleaner` reflete o invariante

`config.CredentialCleaner` agora exige `ListVisible` além de
`DeletePattern`. Mocks de teste obrigatoriamente seguem o contrato e
testes específicos garantem:

- `ClearAllCredentials` nunca emite `pattern=""` para o cleaner.
- Erro do cleaner é propagado, não engolido.

### D5. Testes de regressão

- `internal/database/credential_loss_repro_test.go` reproduz o cenário
  exato do incidente (schema pré-AEP-0052 com 14 credenciais) e prova
  que o pipeline canônico não perde nem corrompe nenhuma row, nem
  zera `token_enc`/`client_secret_enc`.
- `internal/credentials/db_store_safety_test.go` cobre:
  - Pattern vazio em `DeleteCredential` rejeitado.
  - Whitespace em `DeleteCredential` rejeitado.
  - Pattern vazio em `SaveCredential` rejeitado.
  - Delete user-scoped não toca rows de outro usuário.
  - Instance secret deletado pelo escopo `user_id=''`.
  - Caller anônimo não consegue apagar credenciais user-scoped (mesmo órfãs).
- `internal/config/settings_service_test.go` cobre:
  - `ClearAllCredentials` itera N patterns visíveis.
  - Pattern vazio na lista de visíveis é ignorado, não passado adiante.
  - Erro no cleaner é propagado.

## Invariantes congelados

1. **Pattern vazio é sempre erro** — em qualquer escrita ou exclusão
   no `DBStore` de credenciais. Patterns só existem como cordas
   significativas (host, `internal-auth:*`, `mcp-tokens:<slug>`,
   `mcp-client:<slug>`).
2. **Mass-delete é sempre iterativo** — caminhos de "limpar tudo"
   listam, iteram e logam pattern por pattern. Nunca formam uma única
   query DELETE sem `pattern = ?` exato.
3. **Delete user-scoped exige `RequireUserID`** — o `Manager.DeletePattern`
   já fail-close pelos check de scope; `DBStore.DeleteCredential` reforça
   por `ScopeByUser` (que adiciona `ErrUserScopeRequired` à query sem
   user no ctx para patterns não-instance).
4. **Adoção legacy é sempre `UPDATE`, nunca `DELETE`+recreate** —
   `AdoptLegacyData` deleta apenas órfãs cujo pattern já está claimed
   pelo user atual (resolução de duplicata determinística), nunca
   apaga rows com tokens legítimos.
5. **AutoMigrate não pode dropar colunas** — defendido pelo teste
   `TestMigration_CredentialColumnsMatchGORMModel` (já existente) e
   pelo novo teste de reprodução, que verifica `token_enc` /
   `client_secret_enc` intactos depois do migrate.

## Riscos residuais

- O instance secret `mcp-tokens:glean` foi reescrito com `created_at=21:32:20`
  no incidente. O caminho exato dessa rewrite (UPSERT do servidor MCP no
  primeiro boot) não foi instrumentado. Como ele coexiste com instance
  secrets `internal-auth:*`, qualquer regressão futura nesse fluxo de
  bootstrap MCP afeta um conjunto pequeno e isolado (não user-scoped).
- A telemetria de `RowsAffected` em deletes só é coletada via log; um
  atacante com acesso ao processo poderia silenciar logs. Mitigação
  aceita: o invariante de pattern vazio é defendido por código, não por
  log.

## Critérios de aceitação

- [x] `internal/database/credential_loss_repro_test.go` passa.
- [x] `internal/credentials/db_store_safety_test.go` passa.
- [x] `internal/config/settings_service_test.go` cobre os três cenários
      novos e passa.
- [x] `go test ./internal/...` permanece verde.
- [x] Suite de `internal/app` permanece verde após adapter
      `credentialCleanerAdapter`.
- [x] Recuperação operacional do banco do usuário concluída antes deste
      AEP (13 credenciais restauradas a partir de
      `conversations.db.pre-uuid.bak`).
