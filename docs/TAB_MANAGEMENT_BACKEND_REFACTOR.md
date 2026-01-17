# Refatoração: Gerenciamento de Abas no Backend

## 🎯 Objetivo

**Mover toda a lógica de gerenciamento de abas de chat do frontend (localStorage + ChatTabsContainer) para o backend (banco de dados + API)**

O frontend deve ser "burro" - apenas renderizar o estado que vem do backend e enviar comandos.

---

## ❌ Problema Atual

### Onde as coisas estão hoje:

1. **Frontend gerencia tudo** (`ChatTabsContainer.svelte`):
   - Estado das abas em `localStorage` (linhas 32-165)
   - Validação de conversas órfãs
   - Criação/fechamento de abas
   - Ordem das abas
   - Aba ativa
   - Limite de 20 abas
   - Persistência via `localStorage.setItem(STORAGE_KEY, ...)`

2. **Problemas**:
   - ❌ Estado complexo no frontend (stores, props, reatividade problemática)
   - ❌ Perda de estado ao resetar o banco (localStorage fica obsoleto)
   - ❌ Sincronização impossível (multi-window)
   - ❌ Validação de conversas órfãs no frontend (ineficiente + desnecessária)
   - ❌ Limite artificial (MAX_TABS = 20)
   - ❌ Debugging difícil (estado espalhado + localStorage)
   - ❌ Reactivity bugs (vide toda essa conversa!)

---

## ✅ Solução Proposta

### Arquitetura Backend-First

```
┌─────────────────────────────────────────────────────────────┐
│                         BACKEND                              │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Banco de Dados (SQLite/PostgreSQL)                   │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │                                                       │   │
│  │  Tabela: chat_tabs                                   │   │
│  │  ┌─────────────────────────────────────────────┐    │   │
│  │  │ id (PK)          INTEGER                    │    │   │
│  │  │ user_id          INTEGER (FK, futuro)       │    │   │
│  │  │ conversation_id  INTEGER (FK, nullable)     │    │   │
│  │  │ title            TEXT                        │    │   │
│  │  │ icon             TEXT                        │    │   │
│  │  │ position         INTEGER (ordem)            │    │   │
│  │  │ is_active        BOOLEAN (aba ativa)        │    │   │
│  │  │ created_at       TIMESTAMP                   │    │   │
│  │  │ updated_at       TIMESTAMP                   │    │   │
│  │  └─────────────────────────────────────────────┘    │   │
│  │                                                       │   │
│  │  Índices:                                            │   │
│  │  - conversation_id (FK para conversations)          │   │
│  │  - position (para ordenação)                        │   │
│  │  - is_active (para busca rápida da aba ativa)      │   │
│  │                                                       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ API (Go)                                             │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │                                                       │   │
│  │  func GetTabs() ([]ChatTab, error)                  │   │
│  │  func GetActiveTab() (*ChatTab, error)              │   │
│  │  func CreateTab(title, icon) (*ChatTab, error)      │   │
│  │  func CloseTab(id uint) error                       │   │
│  │  func SetActiveTab(id uint) error                   │   │
│  │  func UpdateTabTitle(id uint, title) error          │   │
│  │  func ReorderTabs(positions []uint) error           │   │
│  │  func LoadConversationInTab(tabId, convId) error    │   │
│  │  func ClearTab(id uint) error                       │   │
│  │                                                       │   │
│  │  // Lógica de negócio:                              │   │
│  │  - Validação (max 20 abas)                          │   │
│  │  - Garantir sempre 1 aba ativa                      │   │
│  │  - Remover conversationId se conversa deletada      │   │
│  │  - Auto-ativação ao fechar aba ativa                │   │
│  │                                                       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ Eventos (WebSocket/Wails Events)                     │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │                                                       │   │
│  │  tabs:updated        → Lista completa de abas       │   │
│  │  tabs:created        → Nova aba criada              │   │
│  │  tabs:closed         → Aba fechada                  │   │
│  │  tabs:activated      → Aba ativada                  │   │
│  │  tabs:title_updated  → Título da aba mudou          │   │
│  │                                                       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
└─────────────────────────────────────────────────────────────┘
                              ▲
                              │ API Calls + Events
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                         FRONTEND                             │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  ┌──────────────────────────────────────────────────────┐   │
│  │ ChatTabsContainer.svelte (SIMPLIFICADO)             │   │
│  ├──────────────────────────────────────────────────────┤   │
│  │                                                       │   │
│  │  let tabs = [];           // ← vem do backend       │   │
│  │  let activeTabId = null;  // ← vem do backend       │   │
│  │                                                       │   │
│  │  onMount(() => {                                     │   │
│  │    loadTabs();            // API call                │   │
│  │    listenToEvents();      // Eventos do backend      │   │
│  │  });                                                  │   │
│  │                                                       │   │
│  │  function onNewTab() {                               │   │
│  │    CreateTab('Nova conversa', '💬');  // API call   │   │
│  │  }                                                    │   │
│  │                                                       │   │
│  │  function onCloseTab(id) {                           │   │
│  │    CloseTab(id);          // API call                │   │
│  │  }                                                    │   │
│  │                                                       │   │
│  │  function onSelectTab(id) {                          │   │
│  │    SetActiveTab(id);      // API call                │   │
│  │  }                                                    │   │
│  │                                                       │   │
│  │  EventsOn('tabs:updated', (data) => {               │   │
│  │    tabs = data.tabs;                                 │   │
│  │    activeTabId = data.activeTabId;                   │   │
│  │  });                                                  │   │
│  │                                                       │   │
│  └──────────────────────────────────────────────────────┘   │
│                                                               │
│  ✅ ZERO localStorage                                        │
│  ✅ ZERO lógica de estado complexo                           │
│  ✅ ZERO validação de conversas                              │
│  ✅ ZERO gerenciamento de limite de abas                     │
│  ✅ APENAS renderiza + envia comandos                        │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

---

## 📋 Plano de Implementação

### FASE 1: Backend - Database Schema

**Arquivo**: `internal/database/models.go`

```go
// ==================== Chat Tabs ====================

// ChatTab representa uma aba de chat aberta na interface
// Seguindo o padrão das tabelas existentes: Conversation, ChatMessage, Memory, FAQ, AgentConfig
// 
// IMPORTANTE: Substitui config.LastConversationID - cada aba tem sua própria conversa ativa
type ChatTab struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ConversationID *uint     `json:"conversation_id,omitempty" gorm:"index"` // FK para Conversation (nullable)
	Title          string    `json:"title" gorm:"default:'Nova conversa'"`
	Icon           string    `json:"icon" gorm:"default:'💬'"`
	Position       int       `json:"position" gorm:"index;default:0"` // Ordem de exibição
	IsActive       bool      `json:"is_active" gorm:"index;default:false"` // Aba ativa no momento
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	
	// Relacionamento (eager loading opcional)
	Conversation *Conversation `json:"conversation,omitempty" gorm:"foreignKey:ConversationID"`
}
```

**Mudanças no design**:
- ❌ Removido `UserID` (não é multi-user ainda)
- ✅ Nome da tabela: `chat_tabs` (plural, como `conversations`, `chat_messages`, `memories`, `faqs`)
- ✅ Padrão consistente com os models existentes
- ✅ **`IsActive` + `ConversationID` substituem `config.LastConversationID`**
  - A aba ativa (`IsActive=true`) contém a última conversa vista pelo usuário
  - Múltiplas abas podem ter conversas diferentes abertas simultaneamente

**Migrations**:
```go
// internal/database/database.go - adicionar ao AutoMigrate
db.AutoMigrate(
	&Conversation{},
	&ChatMessage{},
	&Memory{},
	&FAQ{},
	&ChatTab{},  // ← NOVO
)
```

---

### FASE 2: Backend - Repository Layer

**Arquivo**: `internal/database/tabs_repository.go` (NOVO)

```go
package database

import (
	"errors"
	"gorm.io/gorm"
)

// Constantes
const MaxTabs = 20

// GetAllTabs retorna todas as abas ordenadas por posição
func GetAllTabs() ([]ChatTab, error) {
	var tabs []ChatTab
	err := db.Preload("Conversation").
		Order("position ASC").
		Find(&tabs).Error
	return tabs, err
}

// GetActiveTab retorna a aba ativa (ou cria uma se não existir)
func GetActiveTab() (*ChatTab, error) {
	var tab ChatTab
	err := db.Where("is_active = ?", true).First(&tab).Error
	
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Não há aba ativa - cria uma nova
		return CreateTab("Nova conversa", "💬", true)
	}
	
	return &tab, err
}

// CreateTab cria uma nova aba
func CreateTab(title, icon string, setActive bool) (*ChatTab, error) {
	// Verifica limite
	var count int64
	if err := db.Model(&ChatTab{}).Count(&count).Error; err != nil {
		return nil, err
	}
	if count >= MaxTabs {
		return nil, errors.New("limite de abas atingido")
	}
	
	// Se setActive=true, desativa outras abas
	if setActive {
		db.Model(&ChatTab{}).Where("is_active = ?", true).Update("is_active", false)
	}
	
	// Calcula próxima posição
	var maxPos int
	db.Model(&ChatTab{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	
	tab := &ChatTab{
		Title:    title,
		Icon:     icon,
		Position: maxPos + 1,
		IsActive: setActive,
	}
	
	if err := db.Create(tab).Error; err != nil {
		return nil, err
	}
	
	return tab, nil
}

// CloseTab fecha uma aba (remove do banco)
func CloseTab(id uint) error {
	var tab ChatTab
	if err := db.First(&tab, id).Error; err != nil {
		return err
	}
	
	wasActive := tab.IsActive
	
	// Deleta aba
	if err := db.Delete(&tab).Error; err != nil {
		return err
	}
	
	// Se era a aba ativa, ativa outra
	if wasActive {
		var nextTab ChatTab
		err := db.Where("id != ?", id).
			Order("position ASC").
			First(&nextTab).Error
		
		if err == nil {
			db.Model(&nextTab).Update("is_active", true)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Se não há mais abas, tudo bem (será criada uma nova no GetActiveTab)
	}
	
	return nil
}

// SetActiveTab define a aba ativa
func SetActiveTab(id uint) error {
	// Desativa todas
	db.Model(&ChatTab{}).Update("is_active", false)
	
	// Ativa a selecionada
	return db.Model(&ChatTab{}).Where("id = ?", id).Update("is_active", true).Error
}

// UpdateTabTitle atualiza o título de uma aba
func UpdateTabTitle(id uint, title string) error {
	return db.Model(&ChatTab{}).Where("id = ?", id).Update("title", title).Error
}

// LoadConversationInTab carrega uma conversa em uma aba
func LoadConversationInTab(tabId, conversationId uint) error {
	// Valida que a conversa existe
	var conv Conversation
	if err := db.First(&conv, conversationId).Error; err != nil {
		return err
	}
	
	// Atualiza aba
	return db.Model(&ChatTab{}).Where("id = ?", tabId).Updates(map[string]interface{}{
		"conversation_id": conversationId,
		"title":          conv.Title,
	}).Error
}

// ClearTab limpa a conversa de uma aba (reseta para nova conversa)
func ClearTab(id uint) error {
	return db.Model(&ChatTab{}).Where("id = ?", id).Updates(map[string]interface{}{
		"conversation_id": nil,
		"title":          "Nova conversa",
	}).Error
}

// Nota: Não precisa de CleanupOrphanedTabs()
// Motivo: Quando o banco é resetado, as abas são deletadas junto.
// Quando uma conversa é deletada, DeleteConversation() já limpa as abas automaticamente.

// ReorderTabs reordena as abas
func ReorderTabs(orderedIds []uint) error {
	for i, id := range orderedIds {
		if err := db.Model(&ChatTab{}).Where("id = ?", id).Update("position", i).Error; err != nil {
			return err
		}
	}
	return nil
}

// InitializeDefaultTab cria uma aba padrão se não existir nenhuma
func InitializeDefaultTab() error {
	var count int64
	if err := db.Model(&ChatTab{}).Count(&count).Error; err != nil {
		return err
	}
	
	if count == 0 {
		_, err := CreateTab("Nova conversa", "💬", true)
		return err
	}
	
	return nil
}
```

---

### FASE 3: Backend - Wails API

**Arquivo**: `db.go` (adicionar métodos)

```go
// ==================== Chat Tabs ====================

type TabsResponse struct {
	Tabs        []database.ChatTab `json:"tabs"`
	ActiveTabId uint               `json:"active_tab_id"`
}

// GetTabs retorna todas as abas de chat
func (a *App) GetTabs() (TabsResponse, error) {
	tabs, err := database.GetAllTabs()
	if err != nil {
		return TabsResponse{}, err
	}
	
	// Se não há abas, cria uma padrão
	if len(tabs) == 0 {
		if err := database.InitializeDefaultTab(); err != nil {
			return TabsResponse{}, err
		}
		tabs, err = database.GetAllTabs()
		if err != nil {
			return TabsResponse{}, err
		}
	}
	
	// Encontra aba ativa
	var activeId uint
	for _, tab := range tabs {
		if tab.IsActive {
			activeId = tab.ID
			break
		}
	}
	
	return TabsResponse{
		Tabs:        tabs,
		ActiveTabId: activeId,
	}, nil
}

// GetActiveTab retorna a aba ativa
func (a *App) GetActiveTab() (*database.ChatTab, error) {
	return database.GetActiveTab()
}

// CreateTab cria uma nova aba de chat
func (a *App) CreateTab(title, icon string) (*database.ChatTab, error) {
	tab, err := database.CreateTab(title, icon, true)
	if err != nil {
		return nil, err
	}
	
	// Emite evento
	a.emitTabsUpdatedEvent()
	
	return tab, nil
}

// CloseTab fecha uma aba de chat
func (a *App) CloseTab(id uint) error {
	if err := database.CloseTab(id); err != nil {
		return err
	}
	
	// Emite evento
	a.emitTabsUpdatedEvent()
	
	return nil
}

// SetActiveTab define a aba ativa
func (a *App) SetActiveTab(id uint) error {
	if err := database.SetActiveTab(id); err != nil {
		return err
	}
	
	// Emite evento
	runtime.EventsEmit(a.ctx, "tabs:activated", map[string]interface{}{
		"tab_id": id,
	})
	
	return nil
}

// UpdateTabTitle atualiza o título de uma aba
func (a *App) UpdateTabTitle(id uint, title string) error {
	if err := database.UpdateTabTitle(id, title); err != nil {
		return err
	}
	
	// Emite evento
	runtime.EventsEmit(a.ctx, "tabs:title_updated", map[string]interface{}{
		"tab_id": id,
		"title":  title,
	})
	
	return nil
}

// LoadConversationInTab carrega uma conversa em uma aba
func (a *App) LoadConversationInTab(tabId, conversationId uint) error {
	if err := database.LoadConversationInTab(tabId, conversationId); err != nil {
		return err
	}
	
	// Emite evento
	a.emitTabsUpdatedEvent()
	
	return nil
}

// ClearTab limpa uma aba (nova conversa)
func (a *App) ClearTab(id uint) error {
	if err := database.ClearTab(id); err != nil {
		return err
	}
	
	// Emite evento
	a.emitTabsUpdatedEvent()
	
	return nil
}

// ReorderTabs reordena as abas
func (a *App) ReorderTabs(orderedIds []uint) error {
	if err := database.ReorderTabs(orderedIds); err != nil {
		return err
	}
	
	// Emite evento
	a.emitTabsUpdatedEvent()
	
	return nil
}

// Helper para emitir evento de atualização de abas
func (a *App) emitTabsUpdatedEvent() {
	tabs, err := a.GetTabs()
	if err != nil {
		return
	}
	
	runtime.EventsEmit(a.ctx, "tabs:updated", tabs)
}
```

---

### FASE 4: Backend - Integração com Operações Existentes

#### 4.1. DeleteConversation
**Arquivo**: `db.go` (modificar método existente)

```go
func (a *App) DeleteConversation(id uint) error {
	// Antes de deletar, limpa as abas que referenciam essa conversa
	// (conversation_id vira null, título vira "Nova conversa")
	tabs, err := database.GetAllTabs()
	if err == nil {
		for _, tab := range tabs {
			if tab.ConversationID != nil && *tab.ConversationID == id {
				database.ClearTab(tab.ID) // Não deleta a aba, só limpa a referência
			}
		}
	}
	
	// Deleta conversa normalmente
	if err := database.DeleteConversation(id); err != nil {
		return err
	}
	
	// Emite eventos
	a.emitTabsUpdatedEvent() // Abas atualizadas
	runtime.EventsEmit(a.ctx, "conversation:deleted", map[string]interface{}{
		"conversation_id": id,
	})
	
	return nil
}
```

**Comportamento**:
- ✅ Quando uma conversa é deletada, as abas que a referenciavam **continuam existindo**
- ✅ A aba vira "Nova conversa" (conversation_id = null)
- ✅ O usuário pode então fechar a aba manualmente ou começar uma nova conversa nela

---

#### 4.2. Migração do config.LastConversationID

**Arquivo**: `internal/config/config.go` (remover campo)

```go
type Config struct {
	APIKey             string           `json:"api_key"`
	APIBaseURL         string           `json:"api_base_url"`
	// ... outros campos ...
	// LastConversationID uint          `json:"last_conversation_id,omitempty"` // ❌ REMOVER - agora gerenciado em ChatTab
}
```

**Arquivo**: `llm.go` (remover referências)

Localizar e remover:
- `cfg.LastConversationID = conversationID` (linha ~532)
- `cfg.LastConversationID = 0` (linha ~678)
- `LastConversationID: existing.LastConversationID` (linha ~501)

**Substituir por**: Nada! O backend agora usa `GetActiveTab()` para saber qual conversa está ativa.

---

#### 4.3. GetConfig - Remover completamente LastConversationID

**Arquivo**: `db.go` (remover do response)

```go
type ConfigResponse struct {
	APIKey           string      `json:"api_key"`
	APIBaseURL       string      `json:"api_base_url"`
	ImageModel       string      `json:"image_model,omitempty"`
	ChatParams       ModelParams `json:"chat_params"`
	// ... outros campos do config ...
	// ❌ SEM LastConversationID - isso não é configuração, é estado de sessão!
}

func (a *App) GetConfig() (ConfigResponse, error) {
	cfg, err := config.Load()
	if err != nil {
		return ConfigResponse{}, err
	}
	
	return ConfigResponse{
		APIKey:     cfg.APIKey,
		APIBaseURL: cfg.APIBaseURL,
		// ... outros campos ...
		// ❌ NÃO retorna last_conversation_id
	}, nil
}
```

**Comportamento**:
- ✅ Config JSON **limpo** - apenas configurações, zero estado
- ✅ Frontend **não recebe** `last_conversation_id` do config
- ✅ Frontend carrega abas do backend via `GetTabs()` - conversas já vêm de lá

---

### FASE 5: Frontend - ChatTabsContainer Simplificado

**Arquivo**: `frontend/src/pages/chat/ChatTabsContainer.svelte` (REESCREVER COMPLETAMENTE)

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { TabPanel } from '../../components/tabs';
  import ChatTab from './ChatTab.svelte';
  import { 
    GetTabs, 
    CreateTab, 
    CloseTab, 
    SetActiveTab,
    LoadConversationInTab
  } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';

  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  
  // ❌ SEM initialConversationId - backend já gerencia tudo!

  // ========================================
  // Estado (vem do backend)
  // ========================================
  
  let tabs = [];
  let activeTabId = null;
  let chatTabRefs = {};
  
  // ========================================
  // Lifecycle
  // ========================================
  
  onMount(async () => {
    await loadTabs();
    
    // Backend já retorna as abas com conversation_id correto
    // Não precisa de lógica de "carregar conversa inicial"
    
    // Escuta eventos do backend
    EventsOn('tabs:updated', handleTabsUpdated);
    EventsOn('tabs:activated', handleTabActivated);
    EventsOn('conversation:deleted', handleConversationDeleted);
  });
  
  onDestroy(() => {
    EventsOff('tabs:updated');
    EventsOff('tabs:activated');
    EventsOff('conversation:deleted');
  });
  
  // ========================================
  // Backend Communication
  // ========================================
  
  async function loadTabs() {
    try {
      const response = await GetTabs();
      tabs = response.tabs || [];
      activeTabId = response.active_tab_id || null;
      
      console.log('[ChatTabs] Abas carregadas do backend:', tabs.length);
    } catch (e) {
      console.error('[ChatTabs] Erro ao carregar abas:', e);
      tabs = [];
    }
  }
  
  // ========================================
  // Event Handlers
  // ========================================
  
  function handleTabsUpdated(data) {
    tabs = data.tabs || [];
    activeTabId = data.active_tab_id || null;
  }
  
  function handleTabActivated(data) {
    activeTabId = data.tab_id;
  }
  
  function handleConversationDeleted(data) {
    // Backend já limpou as abas (conversation_id = null)
    loadTabs();
  }
  
  // ========================================
  // User Actions
  // ========================================
  
  async function addNewTab() {
    try {
      await CreateTab('Nova conversa', '💬');
      // Backend emite tabs:updated automaticamente
    } catch (e) {
      console.error('[ChatTabs] Erro ao criar aba:', e);
      if (e.message?.includes('limite')) {
        alert('Limite de 20 abas atingido');
      }
    }
  }
  
  async function closeTab(tabId) {
    try {
      await CloseTab(tabId);
      // Backend emite tabs:updated automaticamente
    } catch (e) {
      console.error('[ChatTabs] Erro ao fechar aba:', e);
    }
  }
  
  async function selectTab(tabId) {
    try {
      await SetActiveTab(tabId);
      // Backend emite tabs:activated automaticamente
      
      // Foca input
      setTimeout(() => {
        const ref = chatTabRefs[tabId];
        if (ref?.focusInput) ref.focusInput();
      }, 100);
    } catch (e) {
      console.error('[ChatTabs] Erro ao selecionar aba:', e);
    }
  }
  
  // ========================================
  // Public API (usado pelo App.svelte)
  // ========================================
  
  export async function openConversation(conversationId) {
    if (!activeTabId) return;
    
    try {
      // Atualiza backend
      await LoadConversationInTab(activeTabId, conversationId);
      // Backend emite tabs:updated
      
      // Carrega no componente Chat
      const ref = chatTabRefs[activeTabId];
      if (ref?.loadConversation) {
        await ref.loadConversation(conversationId);
      }
    } catch (e) {
      console.error('[ChatTabs] Erro ao abrir conversa:', e);
    }
  }
  
  export function startNewConversation() {
    const ref = chatTabRefs[activeTabId];
    if (ref?.clear) ref.clear();
  }
  
  export function focusInput() {
    const ref = chatTabRefs[activeTabId];
    if (ref?.focusInput) ref.focusInput();
  }
</script>

<div class="chat-tabs-container">
  <TabPanel
    tabs={tabs.map(t => ({
      id: String(t.id),
      label: t.title,
      icon: t.icon,
      closeable: tabs.length > 1
    }))}
    activeTabId={String(activeTabId)}
    on:tabSelect={e => selectTab(Number(e.detail))}
    on:tabClose={e => closeTab(Number(e.detail))}
    on:newTab={addNewTab}
  >
    {#each tabs as tab (tab.id)}
      <div class="tab-content" class:active={tab.id === activeTabId}>
        <ChatTab
          bind:this={chatTabRefs[tab.id]}
          tabId={String(tab.id)}
          conversationId={tab.conversation_id}
          {hasApiKey}
          {defaultModel}
          {defaultChatParams}
        />
      </div>
    {/each}
  </TabPanel>
</div>

<style>
  .chat-tabs-container {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  
  .tab-content {
    display: none;
    height: 100%;
  }
  
  .tab-content.active {
    display: flex;
  }
</style>
```

**Mudanças vs código antigo**:
- ❌ **REMOVIDO** `initialConversationId` prop - backend já gerencia
- ❌ **REMOVIDO** lógica de carregar conversa inicial - ChatTab faz isso
- ❌ **REMOVIDO** localStorage (530 → 140 linhas, **-73%**)
- ✅ **PURO**: Apenas renderiza backend + envia comandos

---

### FASE 6: Frontend - Remover localStorage e config.LastConversationID

**Ações em `ChatTabsContainer.svelte`**:
1. ❌ Deletar `const STORAGE_KEY = 'chat_tabs_state'`
2. ❌ Deletar `loadTabsState()`
3. ❌ Deletar `saveTabsState()`
4. ❌ Deletar `clearTabsState()`
5. ❌ Deletar `validateConversations()` (não precisa mais validar órfãs)
6. ❌ Deletar `handleDatabaseReset()` (backend cria aba padrão automaticamente)
7. ❌ Deletar toda lógica de detecção de "guias mortas"

**Ações em `App.svelte`**:
- ✅ **Manter** `initialConversationId` (continua funcionando)
- ✅ **Manter** leitura de `config.last_conversation_id` (agora vem da aba ativa)
- ✅ Nenhuma mudança necessária! 🎉

**Comportamento final**:
```
Ap**Última conversa ativa persistida no banco** (não mais em config JSON)  
✅ Performance melhor (backend mais rápido que localStorage)  
✅ Menos bugs de reatividade  
✅ Ao resetar o banco, tudo começa limpo (sem "guias mortas")  
✅ Possibilidade de features futuras: multi-user, compartilhamento de abas  

### Para o Desenvolvedor:
✅ Frontend 70% mais simples (~380 linhas removidas)  
✅ Debugging fácil (estado no banco)  
✅ Lógica de negócio testável (backend)  
✅ Sem problemas de reatividade Svelte  
✅ Arquitetura limpa e escalável  
✅ ZERO localStorage (fonte de verdade única)  
✅ **Config JSON mais limpo** (sem estado de sessão, só configurações)  
✅ **Fonte de verdade única**: aba ativa no banco = conversa ativa

---

## 🎁 Benefícios

### Para o Usuário:
✅ Estado sincronizado entre múltiplas janelas  
✅ Histórico de abas persistente no banco (não localStorage)  
✅ Performance melhor (backend mais rápido que localStorage)  
✅ Menos bugs de reatividade  
✅ Ao resetar o banco, tudo começa limpo (sem "guias mortas")  
✅ Possibilidade de features futuras: multi-user, compartilhamento de abas  

### Para o Desenvolvedor:
✅ Frontend 70% mais simples (~380 linhas removidas)  
✅ Debugging fácil (estado no banco)  
✅ Lógica de negócio testável (backend)  
✅ Sem problemas de reatividade Svelte  
✅ Arquitetura limpa e escalável  
✅ ZERO localStorage (fonte de verdade única)  

---

## 📊 Métricas de Simplificação

| Componente | Antes | Depois | Redução |
|------------|-------|--------|---------|
| **ChatTabsContainer.svelte** | 530 linhas | **140 linhas** | **-73%** |
| **App.svelte** | ~220 linhas | **~200 linhas** | **-10%** |
| **config.go** | 182 linhas | **~170 linhas** | **-7%** |
| **localStorage logic** | 400+ linhas | **0 linhas** | **-100%** |
| **initialConversationId** | 19 referências | **0 referências** | **-100%** |
| **last_conversation_id** | 6 referências | **0 referências** | **-100%** |

**Total código deletado: ~450 linhas**  
**Total código novo: ~350 linhas** (repository + API)  
**Saldo: -100 linhas + arquitetura infinitamente melhor**

---

## 🚀 Ordem de Implementação

### Sprint 1: Backend Foundation (2-3 horas)
1. ✅ Criar `ChatTab` model
2. ✅ Criar `tabs_repository.go` com todos os métodos
3. ✅ Adicionar migrations
4. ✅ Testar repository (unit tests)

### Sprint 2: Backend API + Limpeza (2-3 horas)
5. ✅ Adicionar métodos Wails em `db.go`
6. ✅ Integrar eventos (`tabs:updated`, etc)
7. ✅ **DELETAR completamente `config.LastConversationID`** de `config.go`
8. ✅ **DELETAR do `ConfigResponse`** em `db.go` (sem shims de compatibilidade!)
9. ✅ **DELETAR todas referências em `llm.go`** (linhas 501, 532, 678)
10. ✅ Modificar `DeleteConversation` para limpar abas
11. ✅ Testar API manual (Wails dev tools)

### Sprint 3: Frontend Refactor (2-3 horas)
12. ✅ **REESCREVER** `ChatTabsContainer.svelte` (530 → 140 linhas)
13. ✅ **DELETAR** todo localStorage (STORAGE_KEY, load/save/clear)
14. ✅ **DELETAR** validateConversations, handleDatabaseReset
15. ✅ **SIMPLIFICAR** `App.svelte` - remover `initialConversationId`
16. ✅ **DELETAR** prop `initialConversationId` de ChatTabsContainer
17. ✅ **DELETAR** leitura de `config.last_conversation_id`
18. ✅ Testar UI (criar/fechar/sele+ Limpeza Final (1-2 horas)
20. ✅ Testar fluxo completo
21. ✅ Testar edge cases (deletar conversa, limite de abas, reset banco)
22. ✅ **Testar persistência**: Fechar app → abrir → abas voltam do banco
23. ✅ **Buscar e deletar** qualquer referência restante a `initialConversationId`
24. ✅ **Buscar e deletar** qualquer referência restante a `last_conversation_id`
25. ✅ Adicionar limpeza de localStorage obsoleto (opcional)

**Total estimado: 7-10 horas de desenvolvimento**

**Arquivos modificados/deletados**:
```
BACKEND:
+ internal/database/models.go          (adicionar ChatTab)
+ internal/database/tabs_repository.go (novo arquivo)
  internal/database/database.go        (migration)
  internal/config/config.go            (remover LastConversationID)
  db.go                                (adicionar API tabs, remover do ConfigResponse)
  llm.go                               (remover 3 linhas)

FRONTEND:
  src/App.svelte                       (remover initialConversationId)
  src/pages/chat/ChatTabsContainer.svelte  (reescrever: 530 → 140 linhas)

DELETADO:
- Toda lógica de localStorage (400+ linhas)
- config.LastConversationID (backend + frontend)
- Props initialConversationId (3 arquivos)
```re sessões**
20. ✅ Limpar localStorage obsoleto

**Total estimado: 7-10 horas de desenvolvimento**

---

## 🔄 Migração de Dados Existentes
**Não precisa!**

Motivos:
- ✅ Quando o usuário atualizar a aplicação, o localStorage com abas antigas **simplesmente será ignorado**
- ✅ Backend cria uma aba padrão vazia automaticamente (`InitializeDefaultTab()`)
- ✅ Usuário continua com acesso às suas conversas (não deletamos nada do banco)
- ✅ Usuário pode reabrir conversas manualmente nas novas abas (gerenciadas pelo backend)

**Comportamento na primeira vez após atualização**:
1. Frontend tenta carregar abas do backend
2. Backend retorna lista vazia (ou cria aba padrão)
3. Usuário vê interface limpa com 1 aba "Nova conversa"
4. localStorage antigo fica lá (mas nunca mais é usado)

**Opcional**: Adicionar lógica de limpeza em `onMount`:
```javascript
// Limpa localStorage obsoleto (uma vez)
if (localStorage.getItem('chat_tabs_state')) {
  localStorage.removeItem('chat_tabs_state');
  console.log('🧹 localStorage de abas antigas removidos_state');
  console.log('✅ Migração concluída');
}
```

---

## 🎯 Resultado FinallocalStorage + validação + migração
- Backend: 0 linhas (não gerencia abas)
- config.json: Tem estado de sessão (last_conversation_id)
- localStorage: Fonte de verdade crítica
- initialConversationId: Prop passada por 3 componentes
- Bugs: Reatividade, sincronização, validação, "guias mortas"
- Multi-window: Impossível (conflitos de localStorage)

DEPOIS:
+ Frontend: 140 linhas (apenas renderiza + comanda)
+ Backend: 350 linhas (models + repository + API + eventos)
+ config.json: LIMPO - apenas configurações
+ Banco de dados: Fonte de verdade única (chat_tabs table)
+ ZERO props de estado (tudo vem do backend)
+ Bugs: ELIMINADOS (backend imutável + SQL transacional)
+ Multi-window: FUNCIONA (todos leem do mesmo banco)
+ ZERO código legado (refatoração completa)apenas UI + comandos)
- Backend: 250 linhas (lógica + validação + eventos)
+ Banco de dados: Fonte de verdade única
+ Bugs: Eliminados (backend imutável)
+ Multi-device: Possível
```

---

## ❓ FAQs

**P: E se o usuário fechar todas as abas?**  
R: `CloseTab()` garante que sempre há pelo menos 1 aba. Se fechar a última, `GetActiveTab()` cria uma nova automaticamente.

**P: O que acontece com `config.LastConversationID`?**  
R: É **removido do config JSON**. A última conversa ativa agora é a `conversation_id` da aba ativa (`is_active=true`). Mais semântico e sincronizado com o estado real das abas.

**P: O frontend precisa de mudanças grandes?**  
R: Não! `App.svelte` continua lendo `last_conversation_id` de `GetConfig()`, mas agora esse valor vem da aba ativa no banco, não do config JSON. Migração transparente.

**P: E a performance? Não vai ficar lento?**  
R: Não. Banco SQLite local é mais rápido que localStorage para operações complexas. Além disso, usamos eventos para atualizar UI apenas quando necessário.
for resetado?**  
R: Todas as abas são deletadas junto com as conversas. Na primeira inicialização após reset, `InitializeDefaultTab()` cria uma aba vazia automaticamente.

**P: O que acontece com abas quando deleto uma conversa?**  
R: A aba **não é deletada**. Apenas o `conversation_id` vira `null` e o título vira "Nova conversa". O usuário pode então fechar a aba manualmente ou começar uma nova conversa nela
R: Mesma situação que corrupted localStorage, mas mais fácil de recuperar (backups, SQL dumps).

**P: Precisa refatorar Chat.svelte também?**  
R: Não imediatamente. Chat.svelte pode continuar como está. A simplificação dele vem como benefício indireto (menos props, menos sincronização).

---

## 🎊 Conclusão

Esta refatoração resolve o problema raiz de reatividade eliminando estado complexo no frontend. O frontend vira um "thin client" que apenas:
- Renderiza estado do backend
- Envia comandos
- Escuta eventos

**Toda lógica de negócio fica no backend**, onde é:
- ✅ Testável
- ✅ Debugável
- ✅ Escalável
- ✅ Sincronizável
- ✅ Imutável

---

**Próximo Passo**: Aprovar arquitetura e começar Sprint 1 (Backend Foundation)
