# AEP-0047 — Importação e Exportação de Conteúdo

**Status:** Done

## Dependências

- **AEP-0046** (Migração de IDs sequenciais para UUIDv7): Esta AEP agora assume que a migração para UUIDv7 já aconteceu e passa a usar esses IDs estáveis como contrato portável. O formato legado sem IDs deixa de ser suportado.
- **AEP-0052** (Sistema de Contas de Usuário): etapa posterior que adiciona `user_id` aos recursos DB-only atuais. A partir dela, export/import deve operar no escopo do usuário autenticado e não mais no namespace global da instância.
- **AEP-0048**, **AEP-0050** e **AEP-0051**: recursos que hoje ainda vivem fora do banco ou dependem de migrações estruturais complementares ficam para etapas posteriores, já nascendo com `user_id` após a AEP-0052.

## Resumo

Sistema de importação e exportação em formato JSON portável com **IDs UUID estáveis**. O desenho desta AEP cobre a evolução do mecanismo para diferentes tipos de recurso, mas **a implementação desta etapa continua restrita ao que já está persistido no banco hoje**, com foco em conversas, mensagens, providers, servidores MCP, tasklists e no bloco portátil de credenciais. O export passa a carregar os IDs persistidos como contrato canônico, permitindo importação idempotente, deduplicação determinística e sobrescrita direta por `id` quando aplicável. O JSON é o formato canônico para importação; HTML e PDF são formatos derivados apenas para exportação rica de conversas. Servidores MCP também podem ser exportados no formato `mcpServers` compatível com Cursor/Claude. Credenciais sensíveis continuam excluídas por padrão, mas podem ser incluídas opcionalmente em um bloco criptografado com senha de exportação.

Recursos que ainda dependem de migração para o banco ou de ajustes estruturais adicionais permanecem fora do escopo imediato e serão tratados após as migrações propostas nas AEP-0052, AEP-0048, AEP-0050 e AEP-0051.

## Motivação

1. **Portabilidade**: Usuários precisam migrar dados entre máquinas, reinstalações ou instâncias do Assistente sem perda de conteúdo.

2. **Idempotência**: Com UUIDv7 estável no banco, o mesmo arquivo deve poder ser importado repetidas vezes sem duplicar dados. A identificação por `id` simplifica fortemente deduplicação, referências internas e sobrescrita.

3. **Backup**: Não existe mecanismo de backup além de copiar manualmente o banco SQLite e os arquivos de configuração. Um export JSON estruturado é mais portável e inspecionável.

4. **Compartilhamento**: Profiles, skills, allowlists e jobs podem ser compartilhados entre usuários. Um formato de export padronizado viabiliza isso.

5. **Preparação para recursos futuros**: Conforme recursos migram de disco para banco (AEP-0046 D11), o sistema de export deve acomodá-los sem reestruturação.

## Decisões

### D0 — Implementação incremental desta fase: DB-only com IDs estáveis

Embora o formato tenha sido desenhado para acomodar múltiplos tipos de recurso, esta entrega implementa apenas o subconjunto já viável com os dados persistidos no banco na arquitetura atual.

Escopo implementado nesta fase:

- conversas
- mensagens
- providers persistidos
- servidores MCP persistidos
- tasklists persistidas
- bloco portátil de credenciais criptografadas
- exportação derivada em HTML/PDF para conversas

Ficam explicitamente fora do escopo desta PR os recursos que ainda vivem em arquivo, os que dependem de migração para o banco e os que exigem reestruturações previstas nas AEP-0046, AEP-0048, AEP-0050, AEP-0051 e AEP-0052. Servidores MCP entram no escopo quando migrados para banco pela AEP-0049.

O pacote `internal/portability` também é responsável por orquestrar importações legadas de arquivos quando um recurso passa a ser persistido no banco. Cada recurso fornece apenas source/parser/importer específicos; o loop de descoberta, leitura read-only, idempotência e relatório de resultado permanece compartilhado para ser reaproveitado por recursos futuros, como skills. O gatilho dessas importações fica em uma fase global pós-login no app, antes dos managers carregarem seus runtimes do banco.

A AEP-0048 usa esse mecanismo para importar definições de jobs, tags, pipelines implícitas e triggers do filesystem para o banco. Logs legados de runs e eventos são descartados e não entram na migração. Tags passam a ser recurso compartilhado do app e podem ser exportadas/importadas como catálogo + associações por recurso. A AEP-0063 não exporta `tool_invocations` como histórico permanente por padrão: são logs técnicos efêmeros, sujeitos a retenção.

### D1 — Formato: JSON versionado com IDs estáveis

O arquivo de export é um único JSON com estrutura definida:

```json
{
  "version": 2,
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

Cada seção em `resources` é um array de objetos do respectivo tipo, incluindo `id` UUID quando o recurso é persistido no banco. Relações entre recursos persistidos também usam esses IDs.

### D2 — IDs estáveis fazem parte do contrato portátil

Os IDs UUID persistidos no banco **são incluídos no export**. Motivos:

- após AEP-0046, o ID deixa de ser sequencial/local e passa a ser estável o suficiente para portabilidade
- a presença do `id` permite importação idempotente e atualização determinística
- referências internas entre recursos ficam mais simples e explícitas
- o importador deixa de depender de heurísticas frágeis por título/slug/data

**Relações entre recursos** são resolvidas por IDs estáveis sempre que o recurso persistido já existir no banco:

| Relação | Chave natural no export |
|---|---|
| `ChatMessage → Conversation` | `conversationId` |
| `ChatMessage → ChatMessage` (parent/turn) | `parentId` / `turnId` |
| `Task → TaskList` | `taskListId` |
| `Task → Task` (subtask) | `parentId` |
| `TaskNote → Task` | `taskId` |
| `TaskListWorkflow → TaskList` | Workflow embutido dentro da TaskList |
| `LLMProvider ← Profile` | `provider.id` |
| `Allowlist ← Profile` | `allowlist` referenciado por slug no profile |
| `Skill ← Profile` | `skills` referenciados por slug no profile |
| `CredentialEntry ← LLMProvider` | `credentialPattern` referenciado por pattern |
| `Conversation ← Channel` | `conversationId` persistido no canal quando esse recurso migrar para banco |
| `Profile ← Workspace` | `profile` referenciado por slug |

### D3 — Estrutura hierárquica com IDs preservados

Recursos dependentes continuam podendo ser embutidos no pai por ergonomia do arquivo, mas **preservam seus próprios IDs** e suas referências estruturais por ID:

```json
{
  "conversations": [
    {
      "id": "0196abc0-1111-7c00-9000-aaaaaaaaaaaa",
      "title": "Conversa sobre UUIDs",
      "channel": "",
      "summary": "...",
      "createdAt": "2026-04-20T10:00:00Z",
      "messages": [
        {
          "id": "0196abc0-1111-7c00-9000-bbbbbbbbbbbb",
          "conversationId": "0196abc0-1111-7c00-9000-aaaaaaaaaaaa",
          "role": "user",
          "content": "Como funciona UUIDv7?",
          "createdAt": "2026-04-20T10:00:01Z"
        },
        {
          "id": "0196abc0-1111-7c00-9000-cccccccccccc",
          "conversationId": "0196abc0-1111-7c00-9000-aaaaaaaaaaaa",
          "role": "assistant",
          "content": "UUIDv7 usa timestamp nos primeiros 48 bits...",
          "model": "gpt-4o",
          "promptTokens": 150,
          "completionTokens": 200,
          "createdAt": "2026-04-20T10:00:05Z",
          "parentId": "0196abc0-1111-7c00-9000-bbbbbbbbbbbb",
          "turnId": "0196abc0-1111-7c00-9000-bbbbbbbbbbbb"
        }
      ]
    }
  ],
  "taskLists": [
    {
      "id": "0196abc0-1111-7c00-9000-dddddddddddd",
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
          "id": "0196abc0-1111-7c00-9000-eeeeeeeeeeee",
          "taskListId": "0196abc0-1111-7c00-9000-dddddddddddd",
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

Hierarquia self-referencial (`ChatMessage.parentId`, `Task.parentId`) é persistida no arquivo pelo próprio `id` do recurso relacionado.

### D4 — Credenciais: excluídas por padrão, exportáveis com criptografia opcional

Campos sensíveis de credenciais (`token`, `password`, `headers`, `refreshToken`, `clientId`, `clientSecret`) são **omitidos** do export por padrão. Motivos:

- Reduz risco de vazamento em exports compartilhados casualmente
- Mantém o caminho padrão seguro mesmo para usuários sem familiaridade com criptografia
- Evita exigir senha de exportação em todo fluxo de backup simples

Quando o usuário marcar `includeCredentials: true`, o export passa a incluir um bloco **criptografado por senha de exportação**, distinto da DEK local. O fluxo é:

1. O app descriptografa as credenciais usando a DEK local atual
2. Serializa apenas o subconjunto necessário para portabilidade
3. Criptografa esse bloco com uma **senha de exportação** informada pelo usuário, usando Argon2id + AES-256-GCM
4. Salva no JSON apenas o blob criptografado e seus metadados criptográficos

Exemplo:

```json
{
  "options": {
    "includeCredentials": true
  },
  "resources": {
    "credentials": {
      "mode": "encrypted",
      "algorithm": "argon2id-aes-256-gcm",
      "salt": "...",
      "ciphertext": "..."
    }
  }
}
```

`CredentialKeyWrap` **nunca** é exportado — é material criptográfico específico da instância e não participa do formato portável.

Na importação:

- se o arquivo **não** incluir credenciais, nada muda
- se incluir credenciais criptografadas, o usuário precisa informar a **senha de exportação**
- após descriptografar, o app recriptografa os valores com a **DEK local da instância de destino**

Isso mantém portabilidade entre máquinas sem acoplar o arquivo à DEK ou ao keyring da máquina de origem.

### D5 — Áudio excluído por padrão

O campo `audio` (base64) das mensagens é **omitido** por padrão:

- Uma mensagem TTS de ~30s gera ~500KB em base64
- 100 mensagens com áudio = ~50MB, tornando o export impraticável
- Áudio pode ser regenerado via TTS

O campo `audioMimeType` é preservado como indicação de que havia áudio originalmente.

Nesta fase, `includeAudio: true` está disponível para exports completos e inclui
o conteúdo base64 do áudio das mensagens. Como isso pode aumentar muito o tamanho
do arquivo, a opção permanece desligada por padrão e deve ser explícita na UI/CLI.

### D5.1 — HTML e PDF são formatos derivados de exportação

O formato **canônico** do sistema é sempre o JSON versionado desta AEP. HTML e PDF existem apenas como saídas derivadas de exportação rica:

- **JSON**: formato fonte para backup, migração e importação
- **HTML**: formato legível, navegável e compartilhável
- **PDF**: formato fixo para arquivamento, impressão e distribuição

Consequências:

- **Importação aceita apenas JSON**
- HTML/PDF não precisam conter todos os campos necessários para roundtrip
- O pipeline recomendado é: montar modelo canônico → gerar JSON → renderizar representação HTML → opcionalmente converter HTML para PDF

### D5.2 — Markdown e toggles de conteúdo na exportação rica

Além de HTML e PDF, a exportação rica de conversas passa a oferecer **Markdown (`.md`)** como formato derivado adicional, gerado a partir do mesmo modelo canônico de conversas. Markdown continua sendo apenas saída (não é aceito na importação, como HTML/PDF).

Os três formatos derivados (HTML, PDF, Markdown) respeitam **toggles de conteúdo** opcionais, expostos em `ExportOptions`:

- `includeTimestamps`: datas/horários de geração, criação da conversa e de cada mensagem
- `includeReasoning`: bloco de raciocínio (`reasoning`) das mensagens
- `includeMetadata`: metadados como modelo, origem/provider e contagem de tokens

No contrato de requisição (`ExportRequest`), esses toggles são opcionais (ponteiros) e, quando ausentes, assumem o comportamento histórico de **incluir tudo**, preservando compatibilidade. A UI do Histórico apresenta esses toggles como checkboxes acessíveis antes de confirmar a exportação.

Adicionalmente, o HTML exportado passa a aplicar **syntax highlighting real** nos blocos de código (Chroma via goldmark-highlighting, com classes CSS e stylesheet embutida), mantendo o documento autocontido. As cores do tema de highlighting são intrínsecas ao artefato exportado (documento standalone), não à UI temada do app.

### D6 — Seleção múltipla de recursos

O export suporta três modos:

1. **Sistema inteiro**: exporta todos os recursos de todos os tipos
2. **Por tipo**: exporta todos os recursos de um ou mais tipos (ex: todas as conversas + todos os profiles)
3. **Individual**: exporta recursos específicos selecionados (ex: conversa "X" + profile "Y" + tasklist "Z")

**Implementação desta PR:** a UI e a app layer expõem o recorte DB-only já suportado pelo banco atual, isto é, exportação/importação de conversas selecionadas, providers persistidos, tasklists persistidas e do bloco opcional de credenciais portáveis.

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
    IncludeCredentials bool
    CredentialExportPassword string
    OutputFormat    string // "json" | "html" | "pdf"
}
```

### D7 — Importação idempotente por ID

Na importação, cada recurso persistido é resolvido primariamente por `id`:

| Recurso | Chave de conflito | Estratégia |
|---|---|---|
| `Conversation` | `id` | Upsert por ID |
| `LLMProvider` | `id` | Upsert por ID |
| `Profile` | slug | Perguntar: pular, sobrescrever, renomear |
| `Skill` | slug | Perguntar: pular, sobrescrever, renomear |
| `Allowlist` | slug | Perguntar: pular, sobrescrever, renomear |
| `MCP Server` | slug | Perguntar: pular, sobrescrever, renomear |
| `Job` | id | Perguntar: pular, sobrescrever, renomear |
| `TaskList` | `id` | Upsert por ID |
| `CredentialEntry` | `id` | Upsert por ID |
| `Channel` | nome do canal | Perguntar: pular, sobrescrever |
| `Contact` | id + canal | Merge (adicionar novos, manter existentes) |
| `Workspace` | nome | Perguntar: pular, sobrescrever, renomear |

A consequência prática é:

- reimportar o mesmo arquivo deve ser seguro
- o import vira idempotente por padrão
- o modo principal deixa de depender de `skip/overwrite/rename`
- detecção de conflito por chave natural vira fallback excepcional apenas para recursos legados fora do escopo DB atual

**Implementação desta fase:** o recorte DB-only converge para import idempotente
de conversations/messages/providers/tasklists/credentials por `id`. A UI e a CLI
não expõem resolução interativa para esses recursos no caminho principal. A API
`ImportDataWithResolutions` e os tipos de resolução podem permanecer como
compatibilidade transitória para arquivos antigos ou incompletos, mas não fazem
parte do comportamento esperado de importação DB-only desta PR.

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

O campo `version: 2` no export identifica o novo contrato baseado em IDs estáveis:

- Importação sempre verifica a versão antes de processar
- O formato legado `version: 1` deixa de ser suportado
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
| Todos | `id` (PK) | **Mantido** no formato portável para idempotência |
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

Até que as migrações arquiteturais das AEP-0046, AEP-0048, AEP-0050, AEP-0051 e AEP-0052 sejam concluídas, esta extensibilidade permanece como direção de evolução, não como requisito de implementação imediata desta PR.

### D14 — Avisos e erros da importação viajam como código, não como texto

O backend não sabe em que idioma a tela está, e o arquivo importado tampouco.
Por isso aviso, erro e motivo de conflito saem de `ImportResult`,
`ImportAnalysis` e `ImportConflict` como `LocalizedMessage`:

```go
type LocalizedMessage struct {
    Code    string            `json:"code"`   // ex.: "acp.commandNotFound"
    Params  map[string]string `json:"params,omitempty"`
    Message string            `json:"message"` // texto pt-BR de reserva
}
```

1. O **código** é o contrato. A UI traduz por `portability.messages.<código>`
   nos três locales, e a CLI, que fala português, usa o texto de reserva.
2. Os **parâmetros** vêm do arquivo importado (id de provider, caminho de
   binário, pattern de credencial) e são interpolados como texto puro — a UI
   nunca os trata como HTML.
3. O **texto de reserva** existe para código desconhecido: um app cuja tradução
   ainda não conhece o código mostra o texto do backend em vez de lista vazia.
4. Erro que o pacote não previu (falha de banco, JSON quebrado) entra sem
   código, e a UI mostra o texto como veio.
5. Erro de validação é `CodedError`, que carrega o mesmo par e continua sendo um
   `error` comum: embrulhar com contexto não apaga o código.

## Fases

### Fase 1 — Backend DB-only

1. Criar pacote `internal/portability/` com tipos `ExportFile`, `ExportRequest`, `ImportResult` e `ImportAnalysis`.
2. Implementar serialização JSON com `version: 2`, `exportedAt`, `appVersion`, `options` e `resources`.
3. Implementar export/import de conversas e mensagens com IDs UUID preservados, incluindo `parentId` e `turnId`.
4. Implementar export/import de providers persistidos por ID.
5. Implementar export/import de tasklists persistidas, incluindo workflow, tasks, subtasks e notes.
6. Implementar credenciais portáteis como bloco criptografado por senha de exportação; sem senha/bloco válido, o import deve falhar claramente.
7. Rejeitar seleções de recursos fora do escopo DB-only no export para evitar perda silenciosa de expectativa.
8. Ignorar seções importadas fora do escopo DB-only com warning claro.

### Fase 2 — Export rico derivado

9. Gerar HTML a partir do modelo canônico de conversas.
10. Gerar PDF a partir do mesmo modelo canônico de conversas.
11. Manter HTML/PDF como formatos somente de exportação; importação aceita apenas JSON.

### Fase 3 — App layer e CLI

12. Expor funções Wails para `ExportData`, `AnalyzeImportData`, `ImportData`, `ImportDataWithResolutions`, `ExportDataToFile` e `ExportConversationsToFile`.
13. Implementar CLI `asst data export`, `asst data analyze` e `asst data import` para o recorte DB-only.
14. Garantir mensagens claras para recursos fora do escopo e versões incompatíveis.

### Fase 4 — Frontend DB-only

15. Expor fluxo de export/import de dados DB-only na UI.
16. Permitir exportar conversas, providers, tasklists e credenciais sem exigir que todos os tipos estejam presentes.
17. Exigir senha quando credenciais forem incluídas.
18. Exibir preview de import com contagens, warnings, recursos fora do escopo e necessidade de senha.
19. i18n: adicionar chaves nos 3 locales (`pt-BR`, `en`, `es`).
20. Acessibilidade: modais navegáveis por teclado, foco gerenciado e feedback via announcer/toast.

### Fase 5 — Testes

21. Testes Go: export/import roundtrip para conversas, providers, tasklists e credenciais.
22. Testes Go: versionamento (`version: 2` aceito; versões incompatíveis rejeitadas) e campos desconhecidos ignorados quando inofensivos.
23. Testes Go: recursos fora do escopo no import geram warning; recursos fora do escopo no export são rejeitados.
24. Testes frontend: modais de export/import, seleção DB-only, senha de credenciais, preview e warnings.

## Riscos

| # | Risco | Probabilidade | Impacto | Mitigação |
|---|---|---|---|---|
| R1 | Export de sistema grande é lento | Média | Médio | Streaming JSON ou progress bar; excluir áudio por padrão |
| R2 | Referências cruzadas quebradas na importação | Alta | Médio | Warnings ao usuário; campos vazios não impedem import |
| R3 | Vazamento de dados sensíveis em MCP `env` | Média | Alto | Warning explícito na UI antes de confirmar export |
| R4 | Conflito de slugs gera confusão | Média | Médio | Preview detalhado antes de importar; opção renomear |
| R5 | Formato JSON com referências quebradas dificulta debug | Baixa | Baixo | IDs preservados, índices auxiliares e `createdAt` permitem rastreio |
| R6 | Áudio excluído causa perda silenciosa | Média | Baixo | Campo `audioMimeType` preservado como indicador |

## Critérios de aceitação

Os critérios abaixo são os critérios de aceite desta PR, limitada ao recorte
DB-only descrito em D0.

1. **Export JSON DB-only** gera `version: 2` válido com conversas/mensagens, providers, tasklists e credenciais criptografadas quando selecionadas.
2. **IDs preservados**: recursos persistidos exportados mantêm seus UUIDs e referências internas por ID.
3. **Import roundtrip**: export → import em instância limpa → dados equivalentes, com IDs preservados; credenciais só entram quando exportadas com senha e são recriptografadas na instância destino.
4. **Import idempotente por ID**: reimportar o mesmo arquivo não duplica conversas, providers, tasklists ou credenciais.
5. **Recursos fora do escopo no export** são rejeitados com mensagem clara.
6. **Recursos fora do escopo no import** são ignorados com warning claro, sem bloquear os recursos DB-only válidos do arquivo.
7. **Credenciais** ficam excluídas por padrão e, quando incluídas, aparecem apenas em bloco criptografado por senha de exportação; `CredentialKeyWrap` nunca é exportado.
8. **Bloco de credenciais obrigatório**: se `includeCredentials: true`, `resources.credentials` deve existir e ser válido.
9. **Áudio** é excluído por padrão; `audioMimeType` é preservado. Quando `includeAudio` for usado, o comportamento deve ser explícito na UI/CLI.
10. **Versionamento**: import aceita `version: 2` e rejeita versões incompatíveis com mensagem clara.
11. **Importação** aceita apenas o formato JSON canônico; HTML/PDF são apenas formatos derivados de exportação.
12. **HTML/PDF** são gerados a partir do mesmo modelo canônico de conversas do export JSON.
13. **i18n**: todas as strings de UI novas existem nos 3 locales.
14. **Acessibilidade**: modais de export/import são navegáveis por teclado, com foco gerenciado e feedback via announcer/toast.
15. **Avisos e erros traduzíveis**: a lista de avisos, erros e motivos de conflito sai do backend com código e parâmetros (D14), aparece na tela e no announcer no idioma escolhido e cai no texto de reserva quando o código é desconhecido.
