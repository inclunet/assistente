# Matriz de compatibilidade de upgrades

Data da revisão: 2026-09-06  
Releases publicados verificados: `0.1.9`, `0.2.0`, `0.3.0`, `0.4.0`, `0.5.0`

## Política

O Assistente suporta upgrade direto de qualquer versão publicada para a versão
atual. Não há versão intermediária obrigatória. Migrações e adaptadores legados
não têm prazo de expiração: só podem ser removidos quando todos os estados
publicados que aceitam forem absorvidos por outro caminho testado, sem reduzir
o conjunto de origens suportadas.

## Matriz versionada

| Caminho legado | Call site de produção | Introduzido | Primeira release que depende dele | Cobertura verificável | Risco de remoção |
|---|---|---:|---:|---|---|
| Banco com PK `INTEGER` → UUIDv7 | `database.Init` → migração v1 | 5d3d7eb9 (2026-04-26) | 0.2.0 | fixture SQL 0.1.9 + teste de upgrade direto; testes de relações em `migration_uuid_test.go` | Crítico: 0.1.9 não inicia/preserva relações |
| Adoção de rows sem `user_id` | login/refresh → `AdoptLegacyData` | AEP-0052 (2026-05) | 0.2.0 | `multiuser_migration_test.go` e `credential_loss_repro_test.go` | Crítico: dados pré-multiusuário ficam invisíveis |
| Migrações numeradas v1–v14 | `database.Init`, antes/depois de `AutoMigrate` | 78c26b94 (2026-06-21) | 0.2.0 (v1–v12) | registry, idempotência, fixture 0.1.9 e diagnóstico local | Crítico: quebra bancos de qualquer release anterior ao passo removido |
| `refresh_url` plaintext → campo cifrado | migração v9 + recriptografia do cofre | 5381ff3b (2026-06-10) | 0.2.0 | `bloco6_migrations_test.go`, `reencrypt_legacy_test.go` | Crítico: perda de refresh token ou segredo em claro |
| Export de conversas com `metadata.version="2.0"` e IDs numéricos | `AnalyzeImportData`/`ImportData` → `parseExportFile` | f3737a82 (2026-01-26) | 0.1.9 | fixture realista 0.1.9, adaptador determinístico e teste idempotente | Alto: backups gerados pela 0.1.9 ficam inutilizáveis |
| Export canônico `version: 2` | mesmos call sites de análise/importação | AEP-0047 (2026-04) | 0.2.0 | fixture comum e parse parametrizado para 0.2.0–0.5.0 | Crítico: backups de todas as releases atuais |
| JSON MCP em disco | pós-login → `ImportLegacyMCPServersWithContext` | 2812b2bb (2026-05-12) | 0.2.0 | parser, import idempotente e continuação após item inválido | Alto: servidores configurados deixam de aparecer |
| Jobs em arquivos | pós-login → `Manager.ImportLegacyDefinitions` | c39059d5 (2026-05-13) | 0.2.0 | import idempotente e migração de slug | Alto: automações deixam de existir no runtime |
| Skills `SKILL.md` em disco | carregamento/importação de skills | b69cb1c1 (2026-06-08) | 0.2.0 | testes de import e catálogo | Alto: skills de instalações antigas somem |
| `channels/*.json` e `contacts.json` | pós-login → `ImportLegacyChannelsWithContext` | a893d4de (2026-07-27) | 0.2.0 | smoke automático do AEP-0083, idempotência e arquivos intactos | Crítico: canais, contatos e referências de segredo somem |
| Cleanup opt-in de canais | UI/API → `CleanupLegacyChannelJSON` | 7bfc1bf5 (2026-07-27) | 0.2.0 | dry-run, backup e confirmação testados | Alto se automatizado: pode destruir a única cópia antes de import concluído |
| Estado de editor anterior por usuário | abertura do editor → `migrateLegacyEditorData` | anterior a 0.2.0 | 0.2.0 | testes do pacote `wailsapi` | Médio: layout/documentos locais deixam de ser encontrados |
| Workspaces e remap anteriores | startup do workspace manager | anterior a 0.2.0 | 0.2.0 | testes do manager | Médio/alto: abas e vínculos ficam órfãos |
| ACP `models` + `session/set_model` | respostas JSON-RPC e fallback após `-32601` | contrato AEP-0084 | 0.2.0 | fixtures de agente legado e efeito no turno seguinte | Alto: agentes publicados no protocolo anterior perdem troca de modelo |
| Perfil sem política estruturada de tools | `ResolveEffectiveToolPolicy` (`legacyAllPreloaded`) | AEP-0081 | 0.2.0 | testes do planner/política | Alto: tools deixam de carregar em perfis existentes |

## Fixtures e limites reais

- Banco 0.1.9: `internal/database/testdata/published/0.1.9.sql`, reconstruído
  do model e do `AutoMigrate` da tag, com conversa, mensagens e relações sem PII.
- Export 0.1.9:
  `internal/portability/testdata/published/0.1.9-conversations.json`.
- Export 0.2.0–0.5.0:
  `internal/portability/testdata/published/0.2.0-0.5.0-portable-v2.json`.
  As quatro tags usam o mesmo `ExportVersion = 2`; o teste varia
  `appVersion` e verifica todas.
- Bancos binários completos 0.2.0–0.5.0 não são versionados nesta rodada.
  Um dump vazio não exercitaria dados nem relações e daria falsa confiança.
  Lacuna: [#684](https://github.com/inclunet/assistente/issues/684).
- Corpus por release para MCP, jobs, skills, channels e contacts:
  [#686](https://github.com/inclunet/assistente/issues/686).
- Layouts publicados de editor, workspaces e perfis/tools:
  [#685](https://github.com/inclunet/assistente/issues/685).

Até essas issues serem fechadas, todos os caminhos legados correspondentes
permanecem obrigatórios.

## Regra para evolução

Toda release nova deve acrescentar sua tag ao teste de compatibilidade e,
quando alterar schema ou formato portável, incluir fixture sintética sem PII.
Uma fixture prova apenas os recursos nela presentes; não autoriza remover
outros importadores, parsers ou migrações.
