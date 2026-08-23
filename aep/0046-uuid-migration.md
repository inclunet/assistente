# AEP-0046 — Migração de IDs Sequenciais para UUIDv7

**Status:** Done

## Resumo

Todas as tabelas do SQLite que usam `uint` auto-increment como chave primária serão migradas para `string` com UUIDv7 (RFC 9562). Os dados existentes serão preservados através de uma migração automática no startup do app — ao detectar o schema antigo (colunas `id` INTEGER), o banco é convertido in-place dentro de uma transação atômica. Todas as entidades mudam em um único esforço (big bang). Recursos armazenados em disco (profiles, skills, allowlists, etc.) ficam fora do escopo — receberão UUIDv7 quando forem migrados para o banco em AEPs futuros.

## Motivação

### Problemas com IDs sequenciais

1. **Previsibilidade**: IDs auto-increment (`1`, `2`, `3`...) expõem ordenação e volume de dados. Em deep links (`assistente://conversation/3`) e canais externos (Telegram, Signal), isso vaza informação desnecessariamente.

2. **Conflito em cenários futuros**: Com a perspectiva de migrar recursos em disco para o banco (profiles, skills, jobs, allowlists, MCP configs, channels, contacts) — e possivelmente sincronizar dados entre dispositivos no futuro — IDs sequenciais gerados localmente colidem inevitavelmente.

3. **Inconsistência no codebase**: Algumas entidades já usam string como PK (`LLMProvider` usa slug, `EditorDocument` usa string, `Workspace` usa UUID, `Tab` usa string gerada), enquanto as entidades core de chat e tarefas usam `uint`. Isso cria dois regimes de tratamento de IDs no frontend e backend.

4. **Fragilidade do FTS5 sync**: Os triggers de full-text search referenciam `chat_messages.id` como integer. Unificar o padrão de IDs simplifica a manutenção.

5. **Preparação para recursos em banco**: O projeto planeja trazer profiles, skills, allowlists, MCP configs, jobs, channels e contacts do disco para o banco de dados. Adotar UUIDv7 agora estabelece o padrão antes que essas migrações aconteçam.

### Por que UUIDv7

- **Ordenável por tempo**: Os primeiros 48 bits são um timestamp Unix em milissegundos. Isso preserva ordenação cronológica natural sem precisar de coluna `created_at` para ORDER BY.
- **Unicidade global**: 128 bits com componente aleatório eliminam colisões mesmo em cenários distribuídos.
- **Compatível com SQLite TEXT**: Armazenado como string de 36 caracteres, indexável, funciona com GORM sem adaptações especiais.
- **RFC 9562**: Padrão IETF formalizado, suportado nativamente por `github.com/google/uuid` v1.6+.

## Decisões

### D1 — Formato: UUIDv7 (RFC 9562)

Todos os IDs de entidades no banco serão UUIDv7 gerados via `uuid.NewV7()` do pacote `github.com/google/uuid`.

Formato resultante: `0193a5e8-7c2b-7def-8a1b-3c4d5e6f7890` (36 chars, lowercase, com hífens).

### D2 — Geração: hook GORM `BeforeCreate`

Um model base `UUIDModel` substitui o `gorm.Model` padrão nas entidades migradas:

```go
type UUIDModel struct {
    ID        string    `gorm:"type:text;primaryKey" json:"id"`
    CreatedAt time.Time `json:"createdAt"`
    UpdatedAt time.Time `json:"updatedAt"`
}

func (u *UUIDModel) BeforeCreate(tx *gorm.DB) error {
    if u.ID == "" {
        id, err := uuid.NewV7()
        if err != nil {
            return err
        }
        u.ID = id.String()
    }
    return nil
}
```

O hook gera o UUID automaticamente se `ID` estiver vazio. Permite também atribuir IDs manualmente em testes ou importações.

### D3 — Escopo: todas as tabelas com `uint` PK

| Tabela | PK atual | PK nova |
|---|---|---|
| `conversations` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `chat_messages` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `credential_entries` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `credential_key_wraps` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `task_lists` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `tasks` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `task_notes` | `id uint` (auto-increment) | `id text` (UUIDv7) |
| `task_list_workflows` | `id uint` (auto-increment) | `id text` (UUIDv7) |

> **Nota**: `task_list_workflow_statuses` **não é uma tabela persistida** — é um
> struct embutido em JSON dentro de `task_list_workflows.statuses_json`. Os IDs de
> status permanecem numéricos (`int`) e não são migrados para UUIDv7.

Tabelas **fora do escopo** (já usam string PK):
- `llm_providers` — slug string (sem mudança)
- `editor_documents` — string 128 (sem mudança)
- `editor_session_states` — string 64 (sem mudança)

### D4 — Foreign keys acompanham

Todas as colunas FK que referenciam PKs migradas mudam de `uint` para `string`:

| FK | De | Para |
|---|---|---|
| `chat_messages.conversation_id` | `uint` | `string` |
| `chat_messages.parent_id` | `*uint` | `*string` |
| `chat_messages.turn_id` | `*uint` | `*string` |
| `conversations.summary_up_to_message_id` | `*uint` | `*string` |
| `tasks.task_list_id` | `uint` | `string` |
| `tasks.parent_id` | `*uint` | `*string` |
| `tasks.status_id` | `uint` | `uint` (permanece numérico — referencia status embutido em JSON) |
| `task_notes.task_id` | `uint` | `string` |
| `task_list_workflows.task_list_id` | `uint` | `string` |
| `task_list_workflows.initial_status_id` | `uint` | `uint` (permanece numérico — referencia status embutido em JSON) |

### D5 — Migração automática de dados no startup

Os dados existentes serão preservados. Na primeira execução com o novo schema:

1. `database.Init()` abre o banco e executa `PRAGMA table_info(conversations)`.
2. Se a coluna `id` for do tipo `INTEGER`, dispara `migrateToUUIDv7()` dentro de uma transação.
3. Para cada tabela (na ordem de dependência), a migração:
   a. Cria `_tabela_new` com schema UUIDv7 (`id TEXT PRIMARY KEY`)
   b. Lê todos os registros da tabela antiga
   c. Insere na tabela nova com `uuid.NewV7()` para cada PK
   d. Armazena `map[uint]string` (old→new) em memória para resolver FKs
   e. Dropa a tabela antiga
   f. Renomeia `_tabela_new` → nome original
4. Recria FTS5, triggers e índices parciais.
5. Se qualquer passo falhar, a transação inteira é revertida (banco original intacto).
6. Um backup `conversations.db.pre-uuid.bak` é criado **antes** de iniciar a migração.

**Ordem de migração** (por dependência de FKs):

```
1. credential_entries      (0 FKs — isolada)
2. credential_key_wraps    (0 FKs — isolada)
3. conversations           (0 FKs recebidas nesta fase)
4. chat_messages           (FK: conversation_id, parent_id, turn_id + FTS5)
5. conversations (2° passe) — atualizar summary_up_to_message_id com mapa de chat_messages
6. task_lists              (0 FKs recebidas nesta fase)
7. task_list_workflows     (FK: task_list_id)
8. tasks                   (FK: task_list_id, parent_id self-ref, status_id)
9. task_notes              (FK: task_id)
```

**Detalhes críticos**:
- **Self-referencing FKs** (`chat_messages.parent_id`, `tasks.parent_id`): O mapa `old→new` é populado durante a inserção na tabela nova. FKs que referenciam a mesma tabela são resolvidas no mesmo passe porque todos os IDs da tabela já foram mapeados.
- **`conversations.summary_up_to_message_id`**: Referência lógica para `chat_messages.id`. Requer um 2° passe em `conversations` após migrar `chat_messages`.
- **FTS5**: Dropar antes de migrar `chat_messages`, recriar depois com `content_rowid=rowid`.
- **Volume esperado**: App pessoal com milhares de registros — migração roda em <1s.

**Complexidade estimada**: ~500-600 linhas Go de código de migração one-shot.

### D6 — Estratégia: big bang

Todas as entidades mudam em um único esforço coordenado. Não haverá período de coexistência `uint` + `string`. Isso é possível porque:
- A migração de dados é atômica numa transação SQLite (D5)
- Bindings Wails são regenerados atomicamente
- O app não tem API REST externa (sem versionamento de API)
- Volume de dados é pequeno (app pessoal, milhares de registros)

### D7 — FTS5: usar `rowid` implícito

O SQLite mantém um `rowid` integer implícito para toda tabela, mesmo quando a PK é TEXT. Os triggers do FTS5 serão adaptados:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS chat_messages_fts USING fts5(
    content,
    content=chat_messages,
    content_rowid=rowid
);

-- Trigger de INSERT: usar NEW.rowid em vez de NEW.id
-- Trigger de DELETE: usar OLD.rowid em vez de OLD.id
-- Trigger de UPDATE: usar OLD.rowid/NEW.rowid
```

A busca FTS5 retornará `rowid`, que será juntado com `chat_messages` via `chat_messages.rowid = chat_messages_fts.rowid` para obter o `id` UUID.

### D8 — Contratos de eventos

Todos os event structs em `chat_events.go` mudam de `uint` para `string`:

```go
// Antes
type StreamEvent struct {
    ConversationID uint   `json:"conversationId"`
    MessageID      uint   `json:"messageId,omitempty"`
    // ...
}

// Depois
type StreamEvent struct {
    ConversationID string `json:"conversationId"`
    MessageID      string `json:"messageId,omitempty"`
    // ...
}
```

Isso impacta ~12 event structs e todos os emitters/consumers em ambos os lados.

### D9 — Deep links

O formato de deep links muda para acomodar UUIDs:

```
# Antes
assistente://conversation/3
assistente://conversation/3/send?message=...

# Depois
assistente://conversation/0193a5e8-7c2b-7def-8a1b-3c4d5e6f7890
assistente://conversation/0193a5e8-7c2b-7def-8a1b-3c4d5e6f7890/send?message=...
```

O tipo `DeepLinkAction.conversationId` muda de `number` para `string`. O parser remove a conversão `Number()`.

### D10 — Frontend: tipos unificados como `string`

| Store/tipo | Campo | De | Para |
|---|---|---|---|
| `chatStore` | `activeConversationId` | `number \| null` | `string \| null` |
| `workspaceStore` | `tab.conversationId` | `number?` | `string?` |
| `taskListStore` | `taskLists` | `Map<number, ...>` | `Map<string, ...>` |
| `taskListStore` | `expandedTasks` | `Set<number>` | `Set<string>` |
| `taskListStore` | `activeTaskListId` | `number?` | `string?` |
| `tasklist.ts` | todas as interfaces | `number` | `string` |
| `deepLinks.ts` | `conversationId` | `number` | `string` |

Nota: `EnrichedMessage.id` já é `string` no frontend — sem mudança necessária nesse caso.

### D11 — Recursos em disco: padrão para AEPs futuros

Quando recursos em disco forem migrados para o banco, **DEVEM** usar UUIDv7 como PK. Cada migração será um AEP separado. Recursos candidatos:

| Recurso | Formato atual | Identificação atual |
|---|---|---|
| Profiles | JSON em `.assistente/profiles/` | slug (nome do arquivo) |
| Skills | MD+YAML em `.assistente/skills/<slug>/` | slug (nome do diretório) |
| Allowlists | JSON em `.assistente/allowlists/` | slug (nome do arquivo) |
| MCP Configs | JSON em `.assistente/mcp/` | slug (nome do arquivo) |
| Jobs | YAML em `~/.assistente/jobs/` | campo `id` no YAML |
| Channels | JSON em `.assistente/channels/` | nome do canal |
| Contacts | JSON em `.assistente/contacts.json` | chat ID externo |
| Memory | MD em `.assistente/memory/` | caminho do arquivo |

Ao migrar, cada recurso receberá um `id` UUIDv7 como PK no banco, e o slug atual será preservado como campo indexado separado para compatibilidade com deep links e referências humanas.

## Fases

### Fase 1 — Infraestrutura UUID + migração (backend) ✅

1. Adicionar `github.com/google/uuid` ao `go.mod`
2. Criar `UUIDModel` em `internal/database/models.go` com hook `BeforeCreate`
3. Verificar se soft delete (`gorm.DeletedAt`) é usado — se sim, incluir no `UUIDModel`
4. Implementar `migrateToUUIDv7()` em `internal/database/migration.go`:
   - Detecção de schema antigo via `PRAGMA table_info`
   - Backup do banco antes de migrar
   - Transação atômica com create/copy/drop/rename por tabela
   - Mapas `old→new` em memória para resolução de FKs
   - Drop/recreate de FTS5 + triggers
   - Recreate de índices parciais
5. Testes da migração com banco de teste populado (`migration_test.go`)

### Fase 2 — Migrar models GORM (backend) ✅

6. Substituir PKs `uint` → `string` (UUIDModel) em todas as 9 entidades
7. Atualizar todas as FKs correspondentes
8. Atualizar assinaturas CRUD em:
   - `internal/database/database.go` (Conversation, ChatMessage)
   - `internal/database/tasklist_repository.go` (TaskList, Task, TaskNote, Workflow)
   - `internal/credentials/db_store.go` (CredentialEntry, CredentialKeyWrap)
9. Adaptar FTS5 triggers para usar `rowid` implícito (D7)

### Fase 3 — Migrar contratos de eventos (backend) ✅

10. Atualizar todas as structs em `internal/core/ports/chat_events.go`
11. Atualizar emitters em `app_chat.go`, `app_speech.go`, channels, etc.
12. Atualizar interfaces/ports em `internal/core/ports/`

### Fase 4 — Migrar app layer + controllers (backend) ✅

13. Atualizar funções Wails: `SendMessage`, `GetConversation`, `DeleteConversation`, etc.
14. Atualizar controllers de tasklist, credentials, speech
15. Atualizar mapeamento `contactID → conversationID` em channels
16. Regenerar bindings: `wails generate module`

### Fase 5 — Migrar frontend ✅

17. Atualizar stores: `chatStore`, `workspaceStore`, `taskListStore`
18. Atualizar tipos: `tasklist.ts`, event payload types
19. Atualizar deep links: `deepLinks.ts`
20. Verificar componentes que recebem IDs como props
21. Verificar event listeners

### Fase 6 — Testes ✅

22. Testes Go: ajustar seeds e asserções para UUIDv7
23. Testes Vitest: ajustar mocks com IDs string

### Fase 7 — Verificação final ✅

24. Validar Go, frontend e bindings no fluxo de CI da entrega.
25. A antiga validação manual de um banco INTEGER foi substituída por regressões
    automatizadas abrangentes em `migration_uuid_test.go`, incluindo schema
    antigo populado, FKs, hierarquias, credenciais, FTS5 e compatibilidade GORM.

### Evidências

- Geração e models: `internal/database/models.go` (`UUIDModel`/`BeforeCreate`).
- Migração e backup best-effort:
  `internal/database/migration_uuid.go` (`migrateToUUIDv7`/`createBackup`).
- Regressão principal: `internal/database/migration_uuid_test.go` cobre banco
  vazio e populado, no-op após migração, FKs diretas e autorreferentes,
  preservação de dados, credenciais, FTS5 e schemas parciais.
- Deep links e contratos string: `frontend/src/lib/deepLinks.test.ts`,
  `internal/tools/deeplink/open_deep_link_test.go` e testes dos eventos de chat.

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | FTS5 incompatível com PK string | Média | Alto | Usar `rowid` implícito do SQLite para `content_rowid` (D7). Drop/recreate FTS5 durante migração |
| R2 | Performance de JOINs com string PK | Baixa | Baixo | UUIDv7 são 36 chars fixos; volume do app (milhares de registros) torna impacto negligível |
| R3 | Bindings Wails desatualizados | Média | Alto | Regeneração de bindings é passo obrigatório da Fase 4 |
| R4 | Canais com `conversationID` numérico | Alta | Médio | A migração converte IDs nos JSONs de channels. Tratar gracefully: ID não encontrado = criar nova conversa |
| R5 | Workspaces YAML com IDs numéricos | Alta | Baixo | Tabs com `conversationId` inexistente ficam sem conversa; UX graceful |
| R6 | EnrichedMessage.id já é string | Baixa | Baixo | Confirmar que a conversão `int → string` no backend pode ser removida (agora já é string nativo) |
| R7 | Migração falha no meio | Baixa | Alto | Transação atômica SQLite: qualquer erro reverte tudo. Backup `.bak` criado antes de iniciar |
| R8 | Self-referencing FKs (parent_id) | Média | Médio | Mapa `old→new` populado durante inserção, resolvido no mesmo passe |
| R9 | summary_up_to_message_id cross-table | Média | Médio | 2° passe em conversations após migrar chat_messages, usando mapa de mensagens |

## Critérios de aceitação

- [x] **Todas as PKs** das entidades migradas são `TEXT` com valores UUIDv7.
- [x] **Testes Go e Vitest** foram adaptados para IDs string; regressões focadas estão nos caminhos acima.
- [x] **Frontend** usa contratos de ID string e bindings regenerados.
- [x] **Deep links** com UUID funcionam: `assistente://conversation/{uuid}`.
- [x] **FTS5** retorna resultados após a migração.
- [x] **Eventos** carregam `conversationId` e `messageId` como `string` em ambos os lados.
- [x] **Migração automática** detecta PKs INTEGER e preserva os dados.
- [x] **Backup** `.pre-uuid.bak` é tentado antes da migração; falha continua best-effort.
- [x] **Rollback seguro** usa transação para preservar o banco em caso de falha.
- [x] **Fluxos principais** usam os IDs migrados sem conversão numérica.
- [x] **Dados preservados** incluem conversas, mensagens, tasklists e credenciais nos testes de migração.
