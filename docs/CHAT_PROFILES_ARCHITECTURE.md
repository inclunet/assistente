# Arquitetura de Perfis de Conversa (Chat Profiles)

## Visão Geral

O sistema de **Perfis de Conversa** permite configurar diferentes modos de interação com o assistente, centralizando configurações de modelo, parâmetros, ferramentas disponíveis e system prompt em perfis reutilizáveis.

Este sistema substitui a configuração de modelo dispersa em `config.json` e nas preferências de conversa, oferecendo uma experiência unificada similar aos Perfis de Voz e Perfis de Interação.

```
┌─────────────────────────────────────────────────────────────────────┐
│                        ChatProfile                                   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Configurações de Modelo                                     │   │
│  │  • Modelo (gpt-4o, gpt-oss, claude-3, etc.)                 │   │
│  │  • Temperature, Max Tokens, Top P                            │   │
│  │  • Timeout de resposta                                       │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Ferramentas/Agentes                                         │   │
│  │  • UseTools (ativar/desativar ferramentas)                   │   │
│  │  • Lista de agentes permitidos (whitelist)                   │   │
│  │  • Ou lista de agentes bloqueados (blacklist)                │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  System Prompt                                               │   │
│  │  • Prompt customizado do perfil                              │   │
│  │  • Concatenado com prompts do sistema (memórias, etc.)       │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │  Preferências de UI                                          │   │
│  │  • ShowInternalMessages (mostrar tool calls)                 │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Motivação

### Problemas Atuais

1. **Configuração dispersa**: Modelo padrão em `config.json`, modelo por conversa em `Preferences`, toggle de tools na sessão
2. **Sem reutilização**: Cada conversa precisa ser configurada individualmente
3. **Ferramentas tudo-ou-nada**: Não é possível ter perfis com conjuntos diferentes de ferramentas
4. **System prompt fixo**: Não há como customizar o comportamento do assistente por contexto

### Benefícios dos Perfis

1. **Configuração centralizada**: Um lugar para definir todas as preferências
2. **Reutilização**: Mesmo perfil em múltiplas conversas
3. **Flexibilidade de ferramentas**: Perfil "Programação" com file_manager, perfil "Pesquisa" com web_search
4. **Personas**: System prompts diferentes para contextos diferentes
5. **Compatibilidade com modelos locais**: Perfis sem tools para modelos que não suportam

---

## Modelo de Dados

### ChatProfile

| Campo | Tipo | Null | Default | Descrição |
|-------|------|------|---------|-----------|
| `id` | uint | N | auto | PK |
| `created_at` | datetime | N | now | |
| `updated_at` | datetime | N | now | |
| `name` | string | N | | Nome do perfil (único) |
| `description` | string | S | "" | Descrição |
| `icon` | string | S | "💬" | Emoji/ícone do perfil |
| `is_default` | bool | N | false | Perfil padrão para novas conversas |
| **Modelo** |||||
| `model` | string | N | | Nome do modelo (gpt-4o, gpt-oss, etc.) |
| `temperature` | float | N | 0.7 | 0.0 a 2.0 |
| `max_tokens` | int | N | 4096 | Limite de tokens |
| `top_p` | float | N | 1.0 | 0.0 a 1.0 |
| `response_timeout` | int | N | 180 | Timeout em segundos |
| **Ferramentas** |||||
| `use_tools` | bool | N | true | Habilitar ferramentas |
| `tools_mode` | string | N | "all" | "all", "whitelist", "blacklist" |
| `tools_list` | string | S | "" | JSON array de nomes de agentes |
| **System Prompt** |||||
| `system_prompt` | string | S | "" | Prompt customizado (concatenado com sistema) |
| `system_prompt_position` | string | N | "before" | "before" ou "after" do prompt do sistema |
| **UI** |||||
| `show_internal_messages` | bool | N | false | Mostrar tool calls na UI |

### Relacionamento com Conversation

A tabela `Conversation` terá um novo campo:

| Campo | Tipo | Null | Default | Descrição |
|-------|------|------|---------|-----------|
| `chat_profile_id` | uint | S | null | FK → ChatProfile (null = usar padrão) |

O campo `Preferences` JSON existente será **mantido** para overrides pontuais, mas o perfil será a fonte primária.

---

## Hierarquia de Configuração

A configuração efetiva segue esta ordem de prioridade (maior para menor):

```
1. Conversation.Preferences (override pontual)
   ↓
2. ChatProfile (perfil selecionado)
   ↓
3. ChatProfile padrão (is_default=true)
   ↓
4. Valores hardcoded do sistema
```

### Exemplo de Resolução

```go
func GetEffectiveConfig(conversation *Conversation) *EffectiveConfig {
    // 1. Começa com valores padrão do sistema
    config := &EffectiveConfig{
        Model:       "gpt-4o-mini",
        Temperature: 0.7,
        MaxTokens:   4096,
        UseTools:    true,
    }
    
    // 2. Aplica perfil padrão (se existir)
    if defaultProfile := GetDefaultChatProfile(); defaultProfile != nil {
        applyProfile(config, defaultProfile)
    }
    
    // 3. Aplica perfil da conversa (se especificado)
    if conversation.ChatProfileID != nil {
        if profile := GetChatProfile(*conversation.ChatProfileID); profile != nil {
            applyProfile(config, profile)
        }
    }
    
    // 4. Aplica overrides da conversa (se existirem)
    if prefs := conversation.GetPreferences(); prefs != nil {
        applyOverrides(config, prefs)
    }
    
    return config
}
```

---

## Ferramentas/Agentes

### Modos de Seleção

| Modo | Descrição | `tools_list` |
|------|-----------|--------------|
| `all` | Todas as ferramentas disponíveis | Ignorado |
| `whitelist` | Apenas as ferramentas listadas | `["file_manager", "web_search"]` |
| `blacklist` | Todas exceto as listadas | `["memory", "faq"]` |

### Agentes Disponíveis

Lista de agentes que podem ser incluídos/excluídos:

| Nome | Descrição |
|------|-----------|
| `file_manager` | Gerenciamento de arquivos |
| `web_search` | Busca na web |
| `memory` | Memórias do usuário |
| `faq` | Perguntas frequentes |
| `voice_profile` | Gerenciamento de perfis de voz |
| `interaction_profile` | Gerenciamento de perfis de interação |
| `chat_profile` | Gerenciamento de perfis de conversa (este) |
| `http_*` | Agentes HTTP customizados |
| `mcp_*` | Agentes MCP |

### Implementação

```go
func (p *ChatProfile) GetAllowedTools(allTools []Tool) []Tool {
    if !p.UseTools {
        return nil
    }
    
    switch p.ToolsMode {
    case "all":
        return allTools
        
    case "whitelist":
        var allowed []string
        json.Unmarshal([]byte(p.ToolsList), &allowed)
        return filterTools(allTools, allowed, true)
        
    case "blacklist":
        var blocked []string
        json.Unmarshal([]byte(p.ToolsList), &blocked)
        return filterTools(allTools, blocked, false)
    }
    
    return allTools
}
```

---

## System Prompt

### Composição do Prompt Final

O system prompt final é composto por várias partes:

```
┌─────────────────────────────────────────────────────────────────────┐
│  SYSTEM PROMPT FINAL                                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 1. Profile System Prompt (se position="before")               │  │
│  │    "Você é um assistente especializado em programação..."     │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 2. Base System Prompt (sempre presente)                       │  │
│  │    "Você é um assistente inteligente chamado Assistente..."   │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 3. Memórias Relevantes (dinâmico)                             │  │
│  │    "O usuário prefere respostas em português..."              │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 4. Interaction Profile Prompt (se ativo)                      │  │
│  │    "Responda de forma concisa para interação por voz..."      │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐  │
│  │ 5. Profile System Prompt (se position="after")                │  │
│  │    "Lembre-se: sempre termine com uma pergunta..."            │  │
│  └───────────────────────────────────────────────────────────────┘  │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Exemplo de Uso

**Perfil "Programador":**
```
system_prompt: "Você é um assistente especializado em programação. 
Sempre forneça exemplos de código quando relevante.
Use markdown para formatar código.
Prefira soluções simples e idiomáticas."

system_prompt_position: "before"
```

**Perfil "Tutor":**
```
system_prompt: "Ao final de cada resposta, faça uma pergunta 
para verificar o entendimento do usuário."

system_prompt_position: "after"
```

---

## Perfis Padrão (Seed)

Na primeira execução, criar perfis iniciais via migration:

### 1. Perfil "Padrão"
```json
{
  "name": "Padrão",
  "description": "Configuração padrão com todas as ferramentas",
  "icon": "💬",
  "is_default": true,
  "model": "", // Será preenchido com primeiro modelo disponível
  "temperature": 0.7,
  "max_tokens": 4096,
  "use_tools": true,
  "tools_mode": "all",
  "show_internal_messages": false
}
```

### 2. Perfil "Modelo Local"
```json
{
  "name": "Modelo Local",
  "description": "Para modelos locais que não suportam ferramentas",
  "icon": "🏠",
  "is_default": false,
  "model": "",
  "temperature": 0.7,
  "max_tokens": 4096,
  "use_tools": false,
  "tools_mode": "all",
  "show_internal_messages": false
}
```

### 3. Perfil "Programação"
```json
{
  "name": "Programação",
  "description": "Focado em desenvolvimento de software",
  "icon": "💻",
  "is_default": false,
  "model": "",
  "temperature": 0.3,
  "max_tokens": 8192,
  "use_tools": true,
  "tools_mode": "whitelist",
  "tools_list": "[\"file_manager\"]",
  "system_prompt": "Você é um assistente especializado em programação...",
  "system_prompt_position": "before"
}
```

---

## Auto-seleção de Modelo

### Fluxo de Primeira Configuração

```
1. Usuário configura API Key na página de Settings
2. Sistema testa conexão e obtém lista de modelos
3. Se perfil padrão não tem modelo definido:
   a. Procura modelo preferido (gpt-4o, gpt-4o-mini, gpt-4)
   b. Se não encontrar, usa primeiro da lista
   c. Atualiza perfil padrão com o modelo
4. Notifica usuário: "Modelo X selecionado automaticamente"
```

### Implementação

```go
func AutoSelectModelForDefaultProfile(availableModels []string) error {
    profile := GetDefaultChatProfile()
    if profile == nil || profile.Model != "" {
        return nil // Já tem modelo ou não tem perfil padrão
    }
    
    // Ordem de preferência
    preferred := []string{"gpt-4o", "gpt-4o-mini", "gpt-4", "gpt-3.5-turbo"}
    
    var selectedModel string
    for _, pref := range preferred {
        for _, model := range availableModels {
            if strings.Contains(model, pref) {
                selectedModel = model
                break
            }
        }
        if selectedModel != "" {
            break
        }
    }
    
    // Fallback: primeiro modelo disponível
    if selectedModel == "" && len(availableModels) > 0 {
        selectedModel = availableModels[0]
    }
    
    if selectedModel != "" {
        profile.Model = selectedModel
        return UpdateChatProfile(profile)
    }
    
    return nil
}
```

---

## API Backend (Go)

### CRUD de ChatProfile

```go
// Listar todos os perfis
func (a *App) GetChatProfiles() ([]database.ChatProfile, error)

// Obter perfil por ID
func (a *App) GetChatProfile(id uint) (*database.ChatProfile, error)

// Obter perfil padrão
func (a *App) GetDefaultChatProfile() (*database.ChatProfile, error)

// Criar perfil
func (a *App) CreateChatProfile(profile database.ChatProfile) (*database.ChatProfile, error)

// Atualizar perfil
func (a *App) UpdateChatProfile(id uint, profile database.ChatProfile) (*database.ChatProfile, error)

// Excluir perfil
func (a *App) DeleteChatProfile(id uint) error

// Definir como padrão
func (a *App) SetDefaultChatProfile(id uint) error
```

### Perfil da Conversa

```go
// Obter perfil efetivo de uma conversa
func (a *App) GetEffectiveChatProfile(conversationID uint) (*database.ChatProfile, error)

// Definir perfil de uma conversa
func (a *App) SetConversationChatProfile(conversationID uint, profileID uint) error

// Remover perfil customizado (usar padrão)
func (a *App) ClearConversationChatProfile(conversationID uint) error
```

### Configuração Efetiva

```go
// Obter configuração efetiva (resolve hierarquia)
func (a *App) GetEffectiveConfig(conversationID uint) (*EffectiveConfig, error)

// Estrutura retornada
type EffectiveConfig struct {
    Model                string   `json:"model"`
    Temperature          float64  `json:"temperature"`
    MaxTokens            int      `json:"max_tokens"`
    TopP                 float64  `json:"top_p"`
    ResponseTimeout      int      `json:"response_timeout"`
    UseTools             bool     `json:"use_tools"`
    AllowedTools         []string `json:"allowed_tools"` // Nomes dos agentes permitidos
    ShowInternalMessages bool     `json:"show_internal_messages"`
    SystemPrompt         string   `json:"system_prompt"` // Prompt do perfil (não o completo)
}
```

---

## Interface do Usuário

### Página de Perfis de Conversa

```
┌─────────────────────────────────────────────────────────────────────┐
│  Perfis de Conversa                                       [+ Novo]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ ⭐ 💬 Padrão                                    gpt-4o-mini │   │
│  │    Configuração padrão com todas as ferramentas              │   │
│  │    Tools: ✅ Todas                                           │   │
│  │                                        [Editar] [Excluir]    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │   🏠 Modelo Local                               gpt-oss-20b │   │
│  │   Para modelos locais que não suportam ferramentas           │   │
│  │   Tools: ❌ Desativadas                                      │   │
│  │                                        [Editar] [Excluir]    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │   💻 Programação                                    gpt-4o  │   │
│  │   Focado em desenvolvimento de software                      │   │
│  │   Tools: file_manager                                        │   │
│  │                                        [Editar] [Excluir]    │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Modal de Edição

```
┌─────────────────────────────────────────────────────────────────────┐
│  Editar Perfil de Conversa                                    [X]   │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  Nome: [Programação                                          ]      │
│  Descrição: [Focado em desenvolvimento de software           ]      │
│  Ícone: [💻]                                                        │
│  [✓] Definir como perfil padrão                                     │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  MODELO                                                             │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  Modelo: [gpt-4o                                           ▼]       │
│                                                                      │
│  Temperature: [0.3    ]  Max Tokens: [8192   ]  Top P: [1.0  ]     │
│                                                                      │
│  Timeout de Resposta: [180    ] segundos                            │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  FERRAMENTAS                                                        │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  [✓] Habilitar ferramentas                                          │
│                                                                      │
│  Modo: ( ) Todas as ferramentas                                     │
│        (•) Apenas selecionadas (whitelist)                          │
│        ( ) Todas exceto selecionadas (blacklist)                    │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ [✓] file_manager - Gerenciamento de arquivos                  │ │
│  │ [ ] web_search - Busca na web                                 │ │
│  │ [ ] memory - Memórias do usuário                              │ │
│  │ [ ] faq - Perguntas frequentes                                │ │
│  │ [ ] voice_profile - Perfis de voz                             │ │
│  │ [ ] interaction_profile - Perfis de interação                 │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  SYSTEM PROMPT                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  Posição: (•) Antes do prompt do sistema                            │
│           ( ) Depois do prompt do sistema                           │
│                                                                      │
│  ┌───────────────────────────────────────────────────────────────┐ │
│  │ Você é um assistente especializado em programação.            │ │
│  │ Sempre forneça exemplos de código quando relevante.           │ │
│  │ Use markdown para formatar código.                            │ │
│  │ Prefira soluções simples e idiomáticas.                       │ │
│  └───────────────────────────────────────────────────────────────┘ │
│                                                                      │
│  ═══════════════════════════════════════════════════════════════    │
│  INTERFACE                                                          │
│  ═══════════════════════════════════════════════════════════════    │
│                                                                      │
│  [ ] Mostrar mensagens internas (tool calls)                        │
│                                                                      │
│                                          [Cancelar] [Salvar]        │
└─────────────────────────────────────────────────────────────────────┘
```

### Picker na Toolbar do Chat

Substituir o ModelPicker atual por um ChatProfilePicker:

```
┌──────────────────────────────────────────────────────────────────────────┐
│  [➕ Nova] [📜 Histórico ▼] │ [💬 Padrão ▼] [🔊 Voz ▼] [🎙️ Interação ▼] │
└──────────────────────────────────────────────────────────────────────────┘
                                    ↑
                            ChatProfilePicker
                            (substitui ModelPicker + Tools toggle)
```

O picker mostra:
- Ícone + Nome do perfil
- Menu dropdown com todos os perfis
- Opção "Gerenciar perfis..." no final

---

## Integração com Profile Manager

O agente `profile_manager` será estendido para gerenciar perfis de conversa:

### Novas Ferramentas

```json
{
  "name": "list_chat_profiles",
  "description": "Lista todos os perfis de conversa disponíveis"
},
{
  "name": "get_chat_profile",
  "description": "Obtém detalhes de um perfil de conversa",
  "parameters": {
    "profile_id": "ID do perfil"
  }
},
{
  "name": "create_chat_profile",
  "description": "Cria um novo perfil de conversa",
  "parameters": {
    "name": "Nome do perfil",
    "model": "Modelo a usar",
    "use_tools": "Habilitar ferramentas",
    "tools_list": "Lista de ferramentas (se whitelist/blacklist)",
    "system_prompt": "Prompt customizado"
  }
},
{
  "name": "update_chat_profile",
  "description": "Atualiza um perfil de conversa existente",
  "parameters": {
    "profile_id": "ID do perfil",
    "...campos a atualizar..."
  }
},
{
  "name": "delete_chat_profile",
  "description": "Exclui um perfil de conversa",
  "parameters": {
    "profile_id": "ID do perfil"
  }
},
{
  "name": "set_conversation_chat_profile",
  "description": "Define o perfil de conversa para a conversa atual",
  "parameters": {
    "profile_id": "ID do perfil (0 para usar padrão)"
  }
}
```

---

## Migração

### Fase 1: Criar Estrutura

1. Criar tabela `chat_profiles`
2. Adicionar coluna `chat_profile_id` em `conversations`
3. Criar perfis seed via migration

### Fase 2: Migrar Dados

1. Para cada conversa com `Preferences.Model` definido:
   - Se não existe perfil com esse modelo, criar um
   - Associar conversa ao perfil
2. Migrar `config.json`:
   - `default_model` → perfil padrão
   - `chat_params` → perfil padrão
   - `chat_defaults.use_tools` → perfil padrão

### Fase 3: Limpar Código Legado

1. Remover campos de modelo de `config.json`
2. Remover ModelPicker da toolbar (substituir por ChatProfilePicker)
3. Remover toggle de Tools (agora no perfil)
4. Atualizar Settings page (remover seção de modelo)

---

## Considerações de Acessibilidade

- ChatProfilePicker deve seguir padrão ARIA de combobox
- Anunciar mudança de perfil via aria-live
- Atalho de teclado: Ctrl+P para abrir picker (conflito com Voz? avaliar)
- Labels descritivos: "Perfil de conversa: Programação, modelo gpt-4o"

---

## Plano de Implementação

### Sprint 1: Backend Base
- [ ] Criar modelo `ChatProfile` no banco
- [ ] Migration para criar tabela e seed
- [ ] CRUD de ChatProfile
- [ ] Funções de perfil efetivo

### Sprint 2: Integração com Chat
- [ ] Modificar `SendMessage` para usar perfil
- [ ] Filtrar tools baseado no perfil
- [ ] Compor system prompt com perfil

### Sprint 3: Frontend - Página de Perfis
- [ ] Criar `ChatProfilesPage`
- [ ] Componente de listagem
- [ ] Modal de edição
- [ ] Seletor de ferramentas

### Sprint 4: Frontend - Toolbar
- [ ] Criar `ChatProfilePicker`
- [ ] Substituir ModelPicker + Tools toggle
- [ ] Persistir perfil por conversa

### Sprint 5: Profile Manager
- [ ] Adicionar ferramentas ao agente
- [ ] Testes de integração

### Sprint 6: Migração e Limpeza
- [ ] Script de migração de dados
- [ ] Remover código legado
- [ ] Atualizar documentação

---

## Referências

- [INTERACTION_PROFILES_ARCHITECTURE.md](./INTERACTION_PROFILES_ARCHITECTURE.md) - Arquitetura similar
- [SPEECH_ARCHITECTURE.md](./SPEECH_ARCHITECTURE.md) - Perfis de voz
- [AGENTS_ARCHITECTURE.md](./AGENTS_ARCHITECTURE.md) - Sistema de agentes
