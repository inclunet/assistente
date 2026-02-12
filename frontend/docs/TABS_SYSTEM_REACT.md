# Sistema de Guias (Tabs) - React

## Visão Geral

O sistema de guias permite ao usuário ter múltiplas conversas abertas simultaneamente, similar a um navegador com abas. Esta implementação segue a arquitetura **backend-driven** definida nos documentos originais do projeto.

## Arquitetura

### 1. Camada de Tipos (`types/tabs.ts`)

Define as interfaces TypeScript:
- `ChatTab`: Representa uma aba individual
- `TabsState`: Estado do store Zustand
- `CreateTabRequest`, `UpdateTabRequest`: DTOs para comunicação com backend

### 2. Camada de Serviço (`services/tabsService.ts`)

Responsável pela comunicação com o backend Wails:
- `getTabs()`: Obtém todas as abas
- `createTab()`: Cria nova aba
- `closeTab()`: Fecha uma aba
- `setActiveTab()`: Define aba ativa
- `updateTabTitle()`: Atualiza título
- `loadConversationInTab()`: Carrega conversa em aba
- `clearTab()`: Limpa conversa de uma aba
- `reorderTabs()`: Reordena abas

**Nota**: Atualmente usa mock data (MOCK_ENABLED = true). Será substituído por chamadas reais ao Wails quando o backend estiver pronto.

### 3. Camada de Estado (`store/tabsStore.ts`)

Store Zustand que gerencia o estado das abas:
- Estado: `tabs`, `activeTabId`, `isLoading`, `error`
- Ações: Wrappers para os métodos do service com atualização de estado

### 4. Camada de Componentes

#### `ChatTabs.tsx`
- Renderiza a barra de abas
- Navegação por teclado (Arrow Left/Right, Home, End, Delete)
- Botão de nova aba (+)
- Botão de fechar (×) em cada aba
- ARIA completo (role="tablist", role="tab", aria-selected)

#### `ChatTabPanel.tsx`
- Renderiza o conteúdo da aba ativa
- ARIA (role="tabpanel", aria-labelledby)
- **TODO**: Implementar ConversationController isolado por aba

### 5. Hooks Globais

#### `useTabsKeyboardShortcuts.ts`
Atalhos de teclado globais:
- `Ctrl+T`: Nova aba
- `Ctrl+W`: Fechar aba atual
- `Ctrl+Tab`: Próxima aba
- `Ctrl+Shift+Tab`: Aba anterior
- `Ctrl+1-9`: Ir para aba N

## Fluxo de Dados

```
Backend (Wails)
     ↕
tabsService.ts (mock temporário)
     ↕
tabsStore.ts (Zustand)
     ↕
ChatTabs.tsx + ChatPage.tsx
     ↕
Usuário
```

## Acessibilidade

### ARIA Implementado
- `role="region"` na barra de abas
- `role="tablist"` na lista de abas
- `role="tab"` em cada aba
- `aria-selected` indica aba ativa
- `aria-controls` conecta aba ao painel
- `role="tabpanel"` no conteúdo
- `aria-labelledby` conecta painel à aba

### Navegação por Teclado
- **Tab**: Entra na lista de abas (foca aba ativa)
- **Arrow Left/Right**: Navega entre abas (circular)
- **Home/End**: Primeira/última aba
- **Delete**: Fecha aba focada
- **Ctrl+T**: Nova aba (global)
- **Ctrl+W**: Fecha aba ativa (global)
- **Ctrl+Tab**: Próxima aba (global)
- **Ctrl+Shift+Tab**: Aba anterior (global)
- **Ctrl+1-9**: Vai para aba N (global)

## Isolamento de Abas

### Status Atual
- ✅ Abas visuais implementadas
- ✅ Navegação por teclado completa
- ✅ Gerenciamento de estado (create, close, activate)
- ⏳ **Pendente**: ConversationController isolado por aba
- ⏳ **Pendente**: Eventos escopados (`chat:stream:<conversationId>`)

### Próximos Passos

1. **Criar ConversationController** (`lib/chat/conversation-controller.ts`):
   - Instanciado por aba
   - Gerencia mensagens, streaming
   - Escuta eventos escopados da conversa
   - Métodos: `loadConversation()`, `send()`, `clear()`, `destroy()`

2. **Criar ConversationStores** (`lib/chat/conversation-stores.ts`):
   - Stores isoladas por aba
   - `messages`, `conversationId`, `isStreaming`, etc.
   - Sem interferência entre abas

3. **Refatorar ChatPage/ChatTabPanel**:
   - Criar controller por aba
   - Passar controller + stores como props
   - Isolar eventos de streaming

4. **Backend Integration**:
   - Remover MOCK_ENABLED do tabsService
   - Implementar chamadas reais ao Wails
   - Conectar com tabela `chat_tabs` do banco

## Compatibilidade com Svelte

Esta implementação segue os mesmos princípios da versão Svelte:
- ✅ Backend-driven (estado no banco, não em localStorage)
- ✅ ARIA completo
- ✅ Navegação por teclado idêntica
- ✅ Atalhos globais (Ctrl+T, Ctrl+W, Ctrl+Tab)
- ⏳ Isolamento por aba (em progresso)
- ⏳ Drag-and-drop para reordenar (futuro)

## Estado Atual

**Sistema de Guias Básico**: ✅ Implementado
**Isolamento de Conversas**: ⏳ Em progresso
**Backend Real**: ❌ Pendente (usando mocks)

O sistema está funcional para desenvolvimento e testes, mas requer:
1. ConversationController para isolamento completo
2. Backend Wails com tabela `chat_tabs` e APIs
3. Eventos escopados para evitar vazamento entre abas
