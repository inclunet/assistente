# AEP-0046 — Migração de IDs Sequenciais para UUIDv7

## Resumo

Todas as tabelas do SQLite que usam `uint` auto-increment como chave primária serão migradas para `string` com UUIDv7 (RFC 9562). O banco será recriado limpo (reset aceitável — sem migração de dados existentes). Todas as entidades mudam em um único esforço (big bang). Recursos armazenados em disco (profiles, skills, allowlists, etc.) ficam fora do escopo — receberão UUIDv7 quando forem migrados para o banco em AEPs futuros.

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
| `task_list_workflow_statuses` | `id uint` (auto-increment) | `id text` (UUIDv7) |

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
| `tasks.status_id` | `uint` | `string` |
| `task_notes.task_id` | `uint` | `string` |
| `task_list_workflows.task_list_id` | `uint` | `string` |
| `task_list_workflows.initial_status_id` | `uint` | `string` |
| `task_list_workflow_statuses.workflow_id` | `uint` | `string` |

### D5 — Migração: reset do banco

A migração de dados existentes **não** será implementada. Na primeira execução com o novo schema:

1. O banco antigo (`conversations.db`) será detectado como incompatível (colunas `id` com tipo INTEGER).
2. O banco antigo será renomeado para `conversations.db.bak` com log de aviso.
3. Um banco novo será criado com o schema UUIDv7 via `AutoMigrate`.

Alternativa aceita: o usuário deleta o banco manualmente.

### D6 — Estratégia: big bang

Todas as entidades mudam em um único esforço coordenado. Não haverá período de coexistência `uint` + `string`. Isso é possível porque:
- Reset do banco é aceitável (D5)
- Bindings Wails são regenerados atomicamente
- O app não tem API REST externa (sem versionamento de API)

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

### Fase 1 — Infraestrutura UUID (backend)

1. Adicionar `github.com/google/uuid` ao `go.mod`
2. Criar `UUIDModel` em `internal/database/models.go` com hook `BeforeCreate`
3. Verificar se soft delete (`gorm.DeletedAt`) é usado — se sim, incluir no `UUIDModel`

### Fase 2 — Migrar models GORM (backend)

4. Substituir PKs `uint` → `string` (UUIDModel) em todas as 9 entidades
5. Atualizar todas as FKs correspondentes
6. Atualizar assinaturas CRUD em:
   - `internal/database/database.go` (Conversation, ChatMessage)
   - `internal/database/tasklist_repository.go` (TaskList, Task, TaskNote, Workflow)
   - `internal/credentials/db_store.go` (CredentialEntry, CredentialKeyWrap)
7. Adaptar FTS5 triggers para usar `rowid` implícito (D7)

### Fase 3 — Migrar contratos de eventos (backend)

8. Atualizar todas as structs em `internal/core/ports/chat_events.go`
9. Atualizar emitters em `app_chat.go`, `app_speech.go`, channels, etc.
10. Atualizar interfaces/ports em `internal/core/ports/`

### Fase 4 — Migrar app layer + controllers (backend)

11. Atualizar funções Wails: `SendMessage`, `GetConversation`, `DeleteConversation`, etc.
12. Atualizar controllers de tasklist, credentials, speech
13. Atualizar mapeamento `contactID → conversationID` em channels
14. Regenerar bindings: `wails generate module`

### Fase 5 — Migrar frontend

15. Atualizar stores: `chatStore`, `workspaceStore`, `taskListStore`
16. Atualizar tipos: `tasklist.ts`, event payload types
17. Atualizar deep links: `deepLinks.ts`
18. Verificar componentes que recebem IDs como props
19. Verificar event listeners

### Fase 6 — Testes

20. Testes Go: ajustar seeds e asserções para UUIDv7
21. Testes Vitest: ajustar mocks com IDs string

### Fase 7 — Reset e verificação

22. Implementar detecção de schema antigo em `database.Init()` (renomear para `.bak` ou warning)
23. Rodar `Check: all` (go test + frontend lint+test+build)
24. Teste manual completo

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | FTS5 incompatível com PK string | Média | Alto | Usar `rowid` implícito do SQLite para `content_rowid` (D7) |
| R2 | Performance de JOINs com string PK | Baixa | Baixo | UUIDv7 são 36 chars fixos; volume do app (milhares de registros) torna impacto negligível |
| R3 | Bindings Wails desatualizados | Média | Alto | Regeneração de bindings é passo obrigatório da Fase 4 |
| R4 | Canais com `conversationID` numérico | Alta | Médio | Channels armazenam mapeamento em JSON; precisa atualizar formato. Tratar gracefully: ID não encontrado = criar nova conversa |
| R5 | Workspaces YAML com IDs numéricos | Alta | Baixo | Tabs com `conversationId` inexistente ficam sem conversa; UX graceful |
| R6 | EnrichedMessage.id já é string | Baixa | Baixo | Confirmar que a conversão `int → string` no backend pode ser removida (agora já é string nativo) |

## Critérios de aceitação

1. **Todas as PKs** das 9 tabelas são `TEXT` com valores UUIDv7
2. **Todos os testes** Go e Vitest passam
3. **Build** frontend compila sem erros
4. **Deep links** com UUID funcionam: `assistente://conversation/{uuid}`
5. **FTS5** busca por texto retorna resultados corretos
6. **Eventos** carregam `conversationId` e `messageId` como `string` em ambos os lados
7. **Banco antigo** é detectado e tratado (renomear `.bak` ou warning)
8. **Nenhuma regressão** nos fluxos: criar conversa, enviar mensagem, criar task list, buscar mensagem
