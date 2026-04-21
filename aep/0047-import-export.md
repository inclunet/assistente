# AEP-0047 — Importação e Exportação de Conteúdo

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Esta AEP foi desenhada para funcionar em conjunto com a AEP-0046. O export não inclui IDs internos (uint ou UUID) justamente para que arquivos exportados antes da migração de IDs possam ser reimportados após a migração sem conflito. Na importação, novos UUIDv7 são gerados automaticamente pelo hook `BeforeCreate` definido na AEP-0046.

## Resumo

Sistema de importação e exportação que permite transferir qualquer recurso do Assistente — desde o sistema inteiro até conversas, credenciais, profiles ou tasks individuais — em formato JSON portável **sem IDs internos**. O export gera um arquivo reutilizável entre instâncias e sobrevive à migração de IDs da AEP-0046. Credenciais criptografadas e áudio de mensagens são excluídos por padrão.

## Motivação

1. **Portabilidade**: Usuários precisam migrar dados entre máquinas, reinstalações ou instâncias do Assistente sem perda de conteúdo.

2. **Sobrevivência à AEP-0046**: A migração de IDs sequenciais para UUIDv7 invalida IDs existentes. Um export sem IDs permite reimportar os dados em um banco com schema novo, gerando UUIDs frescos na importação.

3. **Backup**: Não existe mecanismo de backup além de copiar manualmente o banco SQLite e os arquivos de configuração. Um export JSON estruturado é mais portável e inspecionável.

4. **Compartilhamento**: Profiles, skills, allowlists e jobs podem ser compartilhados entre usuários. Um formato de export padronizado viabiliza isso.

5. **Preparação para recursos futuros**: Conforme recursos migram de disco para banco (AEP-0046 D11), o sistema de export deve acomodá-los sem reestruturação.

## Decisões

### D1 — Formato: JSON

O arquivo de export é um único JSON com estrutura definida:

```json
{
  "version": 1,
  "exportedAt": "2026-04-20T15:30:00Z",
  "appVersion": "1.2.0",
  "options": {
    "includeAudio": false,
    "includeCredentials": false
  },
  "resources": {
    "conversations": [...],
    "providers": [...],
    "profiles": [...],
    ...
  }
}
```

Cada seção em `resources` é um array de objetos do respectivo tipo, **sem campos de ID interno** (PK/FK). Relações são expressas por referências naturais (slug, título, posição ordinal).

### D2 — Sem IDs internos

Nenhum ID de banco (uint ou UUID) é incluído no export. Motivos:

- IDs auto-increment são específicos da instância — não portáveis
- Após AEP-0046, IDs serão UUIDv7 — incompatíveis com exports antigos
- Na importação, IDs são gerados novos pelo banco (via hook `BeforeCreate` da AEP-0046)

**Relações entre recursos** são resolvidas por chaves naturais:

| Relação | Chave natural no export |
|---|---|
| `ChatMessage → Conversation` | Mensagens embutidas dentro da conversa (posição no array) |
| `ChatMessage → ChatMessage` (parent/turn) | Índice ordinal no array de mensagens da conversa |
| `Task → TaskList` | Tasks embutidas dentro da TaskList |
| `Task → Task` (subtask) | Índice ordinal ou agrupamento hierárquico via `children` |
| `TaskNote → Task` | Notes embutidas dentro da Task |
| `TaskListWorkflow → TaskList` | Workflow embutido dentro da TaskList |
| `LLMProvider ← Profile` | `provider.id` (slug) referenciado por nome no profile |
| `Allowlist ← Profile` | `allowlist` referenciado por slug no profile |
| `Skill ← Profile` | `skills` referenciados por slug no profile |
| `CredentialEntry ← LLMProvider` | `credentialPattern` referenciado por pattern |
| `Conversation ← Channel` | Mapeamento `contactId → conversationTitle` (resolvido na importação) |
| `Profile ← Workspace` | `profile` referenciado por slug |

### D3 — Estrutura hierárquica (embed, não flat)

Recursos dependentes são embutidos no pai, não exportados como arrays separados com FKs:

```json
{
  "conversations": [
    {
      "title": "Conversa sobre UUIDs",
      "channel": "",
      "summary": "...",
      "createdAt": "2026-04-20T10:00:00Z",
      "messages": [
        {
          "role": "user",
          "content": "Como funciona UUIDv7?",
          "createdAt": "2026-04-20T10:00:01Z",
          "parentIndex": null,
          "turnIndex": null
        },
        {
          "role": "assistant",
          "content": "UUIDv7 usa timestamp nos primeiros 48 bits...",
          "model": "gpt-4o",
          "promptTokens": 150,
          "completionTokens": 200,
          "createdAt": "2026-04-20T10:00:05Z",
          "parentIndex": 0,
          "turnIndex": 0
        }
      ]
    }
  ],
  "taskLists": [
    {
      "title": "Sprint 42",
      "slug": "sprint-42",
      "description": "...",
      "workflow": {
        "statuses": [...],
        "allowedTransitions": {...},
        "initialStatusIndex": 0
      },
      "tasks": [
        {
          "title": "Implementar export",
          "description": "...",
          "statusIndex": 1,
          "order": 0,
          "children": [...],
          "notes": [...]
        }
      ]
    }
  ]
}
```

Hierarquia self-referencial (`ChatMessage.parentId`, `Task.parentId`) é expressa via `parentIndex` (índice no array do pai) ou aninhamento em `children`.

### D4 — Credenciais excluídas por padrão

Campos criptografados (`tokenEnc`, `passwordEnc`, `headersEnc`, `refreshTokenEnc`, `clientIdEnc`, `clientSecretEnc`) são **omitidos** do export por padrão. Motivos:

- A DEK é específica da instância — valores criptografados são inúteis em outra instância
- Descriptografar exigiria a master password no momento do export
- Risco de vazamento de credenciais em arquivos JSON compartilhados

O que **é** exportado da `CredentialEntry`:
- `pattern` (identificador natural)
- `authType`
- `username` (não criptografado)
- `expiresAt`

`CredentialKeyWrap` **nunca** é exportado — é material criptográfico específico da instância.

Na importação, credenciais são criadas como "esqueleto" sem valores sensíveis. O usuário precisa reconfigurar tokens/senhas manualmente.

### D5 — Áudio excluído por padrão

O campo `audio` (base64) das mensagens é **omitido** por padrão:

- Uma mensagem TTS de ~30s gera ~500KB em base64
- 100 mensagens com áudio = ~50MB, tornando o export impraticável
- Áudio pode ser regenerado via TTS

O campo `audioMimeType` é preservado como indicação de que havia áudio originalmente.

Futuramente, uma opção `includeAudio: true` pode ser adicionada para exports completos.

### D6 — Seleção múltipla de recursos

O export suporta três modos:

1. **Sistema inteiro**: exporta todos os recursos de todos os tipos
2. **Por tipo**: exporta todos os recursos de um ou mais tipos (ex: todas as conversas + todos os profiles)
3. **Individual**: exporta recursos específicos selecionados (ex: conversa "X" + profile "Y" + tasklist "Z")

Na UI, o usuário seleciona o que exportar via checkboxes. Na API Go, a função recebe um `ExportRequest`:

```go
type ExportRequest struct {
    All              bool     // exportar tudo
    ConversationIDs  []string // IDs específicos (ou vazio = todos do tipo)
    ProviderIDs      []string
    ProfileSlugs     []string
    SkillSlugs       []string
    AllowlistSlugs   []string
    MCPServerSlugs   []string
    JobIDs           []string
    TaskListIDs      []string
    ChannelNames     []string
    IncludeContacts  bool
    IncludeWorkspace bool
    IncludeAudio     bool
}
```

### D7 — Importação com resolução de conflitos

Na importação, cada recurso é verificado contra existentes:

| Recurso | Chave de conflito | Estratégia |
|---|---|---|
| `Conversation` | título + channel + createdAt | Perguntar ao usuário |
| `LLMProvider` | `id` (slug) | Perguntar: pular, sobrescrever, renomear |
| `Profile` | slug | Perguntar: pular, sobrescrever, renomear |
| `Skill` | slug | Perguntar: pular, sobrescrever, renomear |
| `Allowlist` | slug | Perguntar: pular, sobrescrever, renomear |
| `MCP Server` | slug | Perguntar: pular, sobrescrever, renomear |
| `Job` | id | Perguntar: pular, sobrescrever, renomear |
| `TaskList` | slug (ou título se sem slug) | Perguntar: pular, sobrescrever, renomear |
| `CredentialEntry` | pattern | Perguntar: pular, sobrescrever (só metadados) |
| `Channel` | nome do canal | Perguntar: pular, sobrescrever |
| `Contact` | id + canal | Merge (adicionar novos, manter existentes) |
| `Workspace` | nome | Perguntar: pular, sobrescrever, renomear |

A UI mostra um resumo dos conflitos detectados antes de confirmar a importação. Recursos sem conflito são importados automaticamente.

### D8 — Referências cruzadas na importação

Quando um recurso importado referencia outro por slug/nome:

- Se o referenciado **também está no export**: a referência é resolvida após importar ambos
- Se o referenciado **não está no export mas existe na instância**: usa o existente
- Se o referenciado **não existe em lugar nenhum**: campo fica vazio com warning ao usuário

Exemplo: Profile "meu-perfil" referencia `llm_provider: "openai-custom"`. Se "openai-custom" não existe na instância nem no export, o profile é importado com `llm_provider: ""` e o usuário é avisado.

### D9 — Dados sensíveis em recursos de arquivo

Além das credenciais do banco, recursos em arquivo podem conter dados sensíveis:

| Recurso | Campo sensível | Tratamento no export |
|---|---|---|
| `MCP Server` | `env` (pode conter API keys) | Incluir com warning na UI |
| `MCP Server` | `oauth2_client_id` | Incluir com warning na UI |
| `Channel` | `bot_token`, `app_token`, `api_token` | **Excluir** (mesmo tratamento de credenciais) |
| `Channel` | `*_token_ref` | Incluir (é só referência) |
| `Job` | `inputs` (pode conter dados sensíveis) | Incluir (responsabilidade do usuário) |

Na UI de export, ao detectar que recursos selecionados contêm campos potencialmente sensíveis (MCP `env`, Channel tokens), mostrar aviso antes de confirmar.

### D10 — Versionamento do formato

O campo `version: 1` no export permite evolução futura:

- Importação sempre verifica a versão antes de processar
- Versões futuras podem adicionar campos sem quebrar compatibilidade (campos desconhecidos são ignorados)
- Se uma versão futura fizer mudanças incompatíveis, incrementa o número e o importador antigo rejeita com mensagem clara

### D11 — Interface: menu principal

Export e import são acessíveis via menu principal do app:

- **Exportar dados...** → abre modal de seleção de recursos → gera arquivo `.json` → diálogo "Salvar como"
- **Importar dados...** → diálogo "Abrir arquivo" → parse do JSON → tela de preview com conflitos → confirmação → importação

### D12 — Campos excluídos do export (computados/runtime)

Campos que **não** são exportados por serem computados ou estado de runtime:

| Modelo | Campo | Motivo |
|---|---|---|
| `Conversation` | `messageCount` | Computado (gorm:"-") |
| `Conversation` | `summarizingInProgress` | Estado runtime |
| `ChatMessage` | `audio` | Excluído por padrão (D5) |
| `Job` | `filePath`, `lastRun`, `status` | Estado runtime |
| Todos | `id` (PK) | Excluído por design (D2) |
| Todos | `updatedAt` | Pode ser regenerado; `createdAt` é preservado |

### D13 — Extensibilidade para recursos futuros

Quando recursos em disco migrarem para o banco (AEP-0046 D11), o export deve acomodá-los:

1. Cada novo tipo de recurso ganha uma seção em `resources` (ex: `"profiles"`, `"skills"`)
2. O campo `version` do export pode ser incrementado se necessário
3. Importadores de versões anteriores ignoram seções desconhecidas gracefully
4. O `ExportService` no Go é implementado como registry de handlers por tipo de recurso:

```go
type ResourceExporter interface {
    Type() string
    Export(req ExportRequest) ([]any, error)
}

type ResourceImporter interface {
    Type() string
    DetectConflicts(data []json.RawMessage) ([]Conflict, error)
    Import(data []json.RawMessage, resolutions map[int]ConflictResolution) error
}
```

Cada recurso implementa essas interfaces. Adicionar um novo tipo requer apenas registrar um novo handler.

## Fases

### Fase 1 — Backend: estrutura do export

1. Criar pacote `internal/portability/` com tipos `ExportFile`, `ExportRequest`, `ExportResult`
2. Criar `ExportService` com registry de `ResourceExporter` handlers
3. Implementar exporters para recursos do banco: Conversation (com mensagens embutidas), LLMProvider, CredentialEntry (sem campos criptografados), TaskList (com workflow, tasks e notes embutidos)
4. Implementar exporters para recursos em arquivo: Profile, Skill, Allowlist, MCP Server, Job, Channel (sem tokens), Contact, Workspace
5. Implementar função `ExportAll` que chama todos os exporters
6. Implementar serialização JSON com `version`, `exportedAt`, `appVersion`, `options`

### Fase 2 — Backend: estrutura do import

7. Criar `ImportService` com registry de `ResourceImporter` handlers
8. Implementar detecção de conflitos para cada tipo de recurso
9. Implementar importação com resolução: pular, sobrescrever, renomear
10. Implementar resolução de referências cruzadas (D8)
11. Implementar importação de cada tipo de recurso (ordem de dependência):
    - Independentes: Allowlist, Skill, MCP, Job, Contact
    - Credenciais (esqueleto)
    - LLM Providers
    - Profiles (referencia providers, skills, allowlists)
    - TaskLists (com workflow, tasks, notes)
    - Conversations (com mensagens)
    - Channels (referencia profiles, conversations)
    - Workspaces (referencia profiles, conversations, tasklists)

### Fase 3 — App layer (Wails)

12. Expor funções Wails: `ExportData(req)`, `ImportPreview(filePath)`, `ImportData(filePath, resolutions)`
13. `ImportPreview` retorna: recursos encontrados, conflitos detectados, warnings de dados sensíveis
14. `ImportData` executa a importação com resoluções de conflito fornecidas pelo usuário

### Fase 4 — Frontend: UI

15. Menu principal: adicionar "Exportar dados..." e "Importar dados..."
16. Modal de export: tree de checkboxes por tipo de recurso, com seleção individual
17. Modal de import: preview dos recursos, lista de conflitos com opções (pular/sobrescrever/renomear), warnings de dados sensíveis
18. Progress feedback via eventos ou loading state
19. i18n: adicionar chaves nos 3 locales (pt-BR, en, es)

### Fase 5 — Testes

20. Testes Go: export/import roundtrip para cada tipo de recurso
21. Testes Go: detecção de conflitos, resolução de referências cruzadas
22. Testes Go: versionamento (rejeitar versão desconhecida, ignorar campos extras)
23. Testes Vitest: modais de export/import, interação com conflitos

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Export de sistema grande é lento | Média | Médio | Streaming JSON ou progress bar; excluir áudio por padrão |
| R2 | Referências cruzadas quebradas na importação | Alta | Médio | Warnings ao usuário; campos vazios não impedem import |
| R3 | Vazamento de dados sensíveis em MCP `env` | Média | Alto | Warning explícito na UI antes de confirmar export |
| R4 | Conflito de slugs gera confusão | Média | Médio | Preview detalhado antes de importar; opção renomear |
| R5 | Formato JSON sem IDs dificulta debug | Baixa | Baixo | Índices ordinais e `createdAt` permitem rastreio |
| R6 | Áudio excluído causa perda silenciosa | Média | Baixo | Campo `audioMimeType` preservado como indicador |

## Critérios de aceitação

1. **Export completo** do sistema gera JSON válido com todos os tipos de recurso
2. **Export seletivo** permite escolher tipos e recursos individuais
3. **Import roundtrip**: export → import em instância limpa → dados equivalentes (exceto IDs e credenciais)
4. **Conflitos** são detectados e apresentados ao usuário antes da importação
5. **Referências cruzadas** são resolvidas corretamente quando ambos os lados estão no export
6. **Credenciais** criptografadas nunca aparecem no JSON de export
7. **Tokens de canais** nunca aparecem no JSON de export
8. **Áudio** é excluído por padrão; `audioMimeType` preservado
9. **Versionamento**: import rejeita `version > 1` com mensagem clara
10. **i18n**: todas as strings de UI nos 3 locales
11. **Acessibilidade**: modais de export/import navegáveis por teclado, com announcements
