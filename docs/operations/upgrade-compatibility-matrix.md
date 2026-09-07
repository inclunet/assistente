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
| Migrações numeradas v1–v15 | `database.Init`, antes/depois de `AutoMigrate` | 78c26b94 (2026-06-21) | 0.2.0 (v1–v12) | registry, idempotência, fixtures 0.1.9–0.5.0 e diagnóstico local | Crítico: quebra bancos de qualquer release anterior ao passo removido |
| `refresh_url` plaintext → campo cifrado | migração v9 + recriptografia do cofre | 5381ff3b (2026-06-10) | 0.2.0 | `bloco6_migrations_test.go`, `reencrypt_legacy_test.go` | Crítico: perda de refresh token ou segredo em claro |
| Export de conversas com `metadata.version="2.0"` e IDs numéricos | `AnalyzeImportData`/`ImportData` → `parseExportFile` | f3737a82 (2026-01-26) | 0.1.9 | fixture realista 0.1.9, adaptador determinístico e teste idempotente | Alto: backups gerados pela 0.1.9 ficam inutilizáveis |
| Export canônico `version: 2` | mesmos call sites de análise/importação | AEP-0047 (2026-04) | 0.2.0 | fixture comum e parse parametrizado para 0.2.0–0.5.0 | Crítico: backups de todas as releases atuais |
| JSON MCP em disco | pós-login → `ImportLegacyMCPServersWithContext` | 2812b2bb (2026-05-12) | 0.1.9 (arquivo produzido); importador desde 0.2.0 | corpus 0.1.9, import direto/idempotente, item inválido e fonte intacta | Alto: servidores configurados deixam de aparecer |
| Jobs em arquivos | pós-login → `Manager.ImportLegacyDefinitions` | c39059d5 (2026-05-13) | nenhuma release publicada produziu YAML de jobs | fixture pré-release aceita, import idempotente, item inválido e fonte intacta | Médio para releases; alto para builds pré-release |
| Skills `SKILL.md` em disco | carregamento/importação de skills | presente em 0.1.9 | 0.1.9 | fixture comum 0.1.9–0.5.0, equivalência do parser, rejeição isolada e fonte intacta | Alto: skills de instalações antigas somem |
| `channels/*.json` e `contacts.json` | pós-login → `ImportLegacyChannelsWithContext` | a893d4de (2026-07-27) | 0.1.9 (arquivos produzidos); importador desde 0.2.0 | corpus 0.1.9, IDs numéricos, contato, item inválido, idempotência e fontes intactas | Crítico: canais, contatos e referências de segredo somem |
| Cleanup opt-in de canais | UI/API → `CleanupLegacyChannelJSON` | 7bfc1bf5 (2026-07-27) | 0.2.0 | dry-run, backup e confirmação testados | Alto se automatizado: pode destruir a única cópia antes de import concluído |
| Editor SQLite 0.1.9 e filesystem 0.2.0–0.5.0 → storage por usuário | login/abertura do editor → `migrateLegacyEditorData` | 0.1.9 | 0.1.9 | fixture SQL 0.1.9 + fixtures de diretório 0.2.0–0.5.0; upgrade direto, idempotência, modo/merge, permissões e adoção exclusiva | Alto: drafts, modo e sessões de merge deixam de ser encontrados ou vazam entre contas |
| Workspaces e remaps publicados | startup do workspace manager | 0.2.0 | 0.2.0 | fixtures YAML + remap por release 0.2.0–0.5.0; conversa/tasklist, editor, terminal, perfil e segunda passagem | Médio/alto: abas e vínculos ficam órfãos |
| ACP `models` + `session/set_model` | respostas JSON-RPC e fallback após `-32601` | contrato AEP-0084 | 0.2.0 | fixtures de agente legado e efeito no turno seguinte | Alto: agentes publicados no protocolo anterior perdem troca de modelo |
| Perfis JSON e política de tools | `profiles.Profile.UnmarshalJSON` + `ResolveEffectiveToolPolicy` (`legacyAllPreloaded`/tri-state) | formato original anterior a 0.1.9; política estruturada na 0.2.0 | 0.1.9 | fixtures 0.1.9–0.5.0; adaptação voice/interaction/MCP, nil/lista/política, wildcard e idempotência | Alto: configurações de voz/STT/canais ou tools mudam silenciosamente |

## Corpus por release e equivalência

O inventário abaixo foi reconstruído diretamente das cinco tags publicadas. O
OID é o blob Git do arquivo que define ou serializa o formato; OIDs iguais
provam que as tags carregavam bytes idênticos, portanto uma fixture comum é
mais fiel que cinco cópias artificiais.

| Domínio | Releases que realmente produziram o formato | Evidência entre tags | Fixture/cobertura |
|---|---|---|---|
| Channels JSON | `0.1.9`; `0.2.0+` já usa DB e apenas lê o legado retido | `channels.go`: 0.1.9 `a1161c6`; 0.2.0–0.5.0 `79a44da` | `internal/channels/testdata/published/0.1.9/`; importa config com conversation ID numérico, contato e continua após JSON inválido |
| Contacts JSON | `0.1.9`; `0.2.0+` já usa DB e apenas lê o legado retido | `contacts.go`: 0.1.9 `1e6370b`; 0.2.0–0.5.0 `b5c9977` | compartilhada com channels; preserva contato e arquivo |
| MCP JSON | `0.1.9`; `0.2.0+` já usa DB e apenas lê o legado retido | `mcp/types.go`: 0.1.9 `e59638f`; 0.2.0–0.5.0 `9489ae2` | `internal/portability/testdata/published/legacy/mcp/`; import direto, idempotência, continuação e fonte intacta |
| Jobs YAML | nenhuma tag: `0.1.9` não contém `internal/jobs`; `0.2.0+` nasceu DB-only | parser e tipos ausentes em 0.1.9; blobs idênticos 0.2.0–0.5.0 (`17a6f46`, `0c89934`) | `internal/jobs/testdata/published/legacy/pre-release-v1.yaml`; marcada como pré-release, sem alegar origem publicada |
| Skills `SKILL.md` | `0.1.9`–`0.5.0` | parser idêntico nas cinco tags: `7accf5a` | uma fixture comum em `internal/skills/testdata/published/0.1.9-0.5.0/`; o teste executa as cinco releases, rejeita vizinha inválida e verifica fonte intacta |
| Credencial com refresh legado | estado pode sobreviver ao boot da 0.1.9 e ser herdado por 0.2.0–0.5.0 | reparador idêntico em 0.2.0–0.5.0: `c8f06d2`; 0.1.9 já copiava `refresh_url` para `refresh_token_enc` | uma fixture sintética comum; recriptografia, segunda execução noop, valor preservado e fonte intacta |
| Envelope portável | legado próprio em 0.1.9; v2 comum em 0.2.0–0.5.0 | `portability/types.go` idêntico em 0.2.0–0.5.0: `3e65cc0` | fixtures de #689; este PR amplia para import real/idempotente de cada tag e rejeição sem escrita parcial |

IDs, nomes, URLs, tokens e contatos do corpus são inteiramente sintéticos.
Nenhuma fixture contém segredo, PII ou identificador externo real.

## Fixtures e limites reais

- Banco 0.1.9: `internal/database/testdata/published/0.1.9.sql`, reconstruído
  do model e do `AutoMigrate` da tag, com conversa, mensagens e relações sem PII.
- Export 0.1.9:
  `internal/portability/testdata/published/0.1.9-conversations.json`.
- Export 0.2.0–0.5.0:
  `internal/portability/testdata/published/0.2.0-0.5.0-portable-v2.json`.
  As quatro tags usam o mesmo `ExportVersion = 2`; o teste varia
  `appVersion` e verifica todas.
- Bancos 0.2.0–0.5.0:
  `internal/database/testdata/published/{0.2.0,0.3.0,0.4.0,0.5.0}.sql`.
  São reconstruções determinísticas executando o schema de cada tag, com duas
  pessoas e dados relacionados de conversas, tasklists, MCP/tools, jobs,
  canais/contatos, memória, tags, ACP e credenciais sem segredo. Cada fixture
  atravessa diretamente o pipeline atual duas vezes. O README do diretório
  registra commits, fingerprints e o delta de schema; 0.3.0–0.5.0 são
  semanticamente equivalentes.
- Corpus de MCP, jobs, skills, channels, contacts, credenciais e portabilidade:
  coberto por [#686](https://github.com/inclunet/assistente/issues/686), com a
  ressalva documentada de que nenhuma release publicou YAML de jobs.
- Editor:
  - `0.1.9` persistia sessão e drafts nas tabelas SQLite
    `editor_session_states`/`editor_documents`;
  - `0.2.0`–`0.5.0` persistiam `editor/state.json` e `editor/drafts/`.
  O upgrade atual preserva `fileModeByPath`, sessões de merge e todo markdown.
  Campos da sessão 0.1.9 sem equivalente atual (`version`, `autoSaveEnabled`,
  `activeTabId`, `profileSlug`, lista/metadados das abas e
  `externalConflictLockedByTabId`) são deliberadamente descartados; as tabelas
  de origem permanecem intactas para recuperação.
- Workspaces existem nas releases `0.2.0`–`0.5.0`. A `0.1.9` não continha o
  domínio `internal/workspace` nem `workspace.yaml`, portanto não há fixture
  artificial para essa release.
- Perfis JSON existem em todas as releases `0.1.9`–`0.5.0`. O adaptador da
  `0.1.9` converte voz monolítica, `interaction`, resposta de canais e
  `mcp_mode`; `enable_thinking`, quando presente em JSON histórico, é
  deliberadamente ignorado porque não tem contrato equivalente ao
  `reasoning_effort` atual.

As fixtures acima fecham as lacunas dos bancos publicados
[#684](https://github.com/inclunet/assistente/issues/684), dos layouts
[#685](https://github.com/inclunet/assistente/issues/685) e do corpus legado
[#686](https://github.com/inclunet/assistente/issues/686). Todos os caminhos
correspondentes permanecem obrigatórios pela política universal.

## Regra para evolução

Toda release nova deve acrescentar sua tag ao teste de compatibilidade e,
quando alterar schema ou formato portável, incluir fixture sintética sem PII.
Uma fixture prova apenas os recursos nela presentes; não autoriza remover
outros importadores, parsers ou migrações.
