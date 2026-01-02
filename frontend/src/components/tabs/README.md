# Tab Panel Component

Painel de abas flexível e acessível com suporte a orientação horizontal e vertical.

## Componentes

| Componente | Descrição |
|------------|-----------|
| `TabPanel` | Container de abas com suporte a múltiplas variantes e recursos |

## Features

- ✅ **Orientação flexível** - Horizontal (padrão) ou vertical
- ✅ **Tabs dinâmicas** - Adicionar, remover e reordenar tabs
- ✅ **Tabs fecháveis** - Botão de fechar em cada tab
- ✅ **Navegação por teclado** - Setas, Home, End, Delete
- ✅ **ARIA completo** - role="tablist", role="tab", role="tabpanel"
- ✅ **Variantes visuais** - Default, Pills, Underline
- ✅ **Drag & Drop** - Reordenação de tabs via arrastar
- ✅ **Reduced motion** - Respeita preferências do usuário
- ✅ **Responsivo** - Adapta-se ao container

## Instalação

```javascript
import { TabPanel } from './components/tabs';
```

## Uso Básico

```svelte
<script>
  import { TabPanel } from './components/tabs';
  
  let tabs = [
    { id: 'home', label: 'Home', icon: '🏠' },
    { id: 'profile', label: 'Perfil', icon: '👤' },
    { id: 'settings', label: 'Configurações', icon: '⚙️' }
  ];
  
  let activeTab = 'home';
</script>

<TabPanel 
  {tabs} 
  bind:activeTab
  on:change={({ detail }) => console.log('Tab changed:', detail.tabId)}
>
  {#if activeTab === 'home'}
    <div>Conteúdo da Home</div>
  {:else if activeTab === 'profile'}
    <div>Conteúdo do Perfil</div>
  {:else if activeTab === 'settings'}
    <div>Conteúdo das Configurações</div>
  {/if}
</TabPanel>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `tabs` | `Tab[]` | `[]` | Array de objetos de aba |
| `activeTab` | `string` | `''` | ID da aba ativa |
| `orientation` | `'horizontal' \| 'vertical'` | `'horizontal'` | Orientação das abas |
| `closableTabs` | `boolean` | `false` | Se as abas podem ser fechadas |
| `addable` | `boolean` | `false` | Se mostra botão de adicionar |
| `addLabel` | `string` | `'Nova aba'` | Label do botão de adicionar |
| `reorderable` | `boolean` | `false` | Se permite reordenar via drag |
| `align` | `'start' \| 'center' \| 'end' \| 'stretch'` | `'start'` | Alinhamento das abas |
| `size` | `'sm' \| 'md' \| 'lg'` | `'md'` | Tamanho das abas |
| `variant` | `'default' \| 'pills' \| 'underline'` | `'default'` | Estilo visual |
| `ariaLabel` | `string` | `'Abas'` | Label acessível para a lista de abas |

### Objeto Tab

| Propriedade | Tipo | Required | Descrição |
|-------------|------|----------|-----------|
| `id` | `string` | ✅ | Identificador único |
| `label` | `string` | ✅ | Texto exibido na aba |
| `icon` | `string` | ❌ | Emoji ou caractere para ícone |
| `closable` | `boolean` | ❌ | Override de fechável (default: segue `closableTabs`) |
| `disabled` | `boolean` | ❌ | Se a aba está desabilitada |
| `data` | `any` | ❌ | Dados customizados associados |

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `change` | `{ tabId, tab, previousTabId, source }` | Quando aba ativa muda (qualquer origem) |
| `close` | `{ tabId, tab, index }` | Quando aba é fechada (via Delete ou clique no ×) |
| `closeRequest` | `{ tabId, tab, index }` | Quando Ctrl+W/Ctrl+F4 é pressionado (componente pai decide se fecha) |
| `add` | - | Quando nova aba é solicitada (botão + ou Ctrl+T) |
| `reorder` | `{ fromIndex, toIndex, tab }` | Quando tab é reordenada |

### Evento `change`

O evento `change` é disparado **sempre** que a aba ativa muda, independente da origem:

- Clique do usuário em uma aba
- Navegação por teclado (setas, Home, End)
- Atalhos Ctrl+Tab / Ctrl+Shift+Tab
- Fechamento da aba ativa (seleciona próxima automaticamente)
- Mudança via binding externo (`activeTab = 'outro-id'`)
- Seleção automática inicial (primeira aba)

```svelte
<TabPanel
  tabs={chatTabs}
  bind:activeTab
  on:change={handleTabChange}
>

<script>
  function handleTabChange({ detail }) {
    console.log('Aba ativa:', detail.tabId);
    console.log('Aba anterior:', detail.previousTabId);
    console.log('Dados da aba:', detail.tab);
    console.log('Origem:', detail.source); // 'reactive'
    
    // Exemplo: salvar estado, carregar dados, etc.
    loadChatData(detail.tabId);
  }
</script>
```

### Diferença entre `close` e `closeRequest`

- **`close`**: Disparado ao clicar no × ou pressionar Delete. Indica ação direta do usuário na aba.
- **`closeRequest`**: Disparado ao pressionar Ctrl+W ou Ctrl+F4. Permite que o componente pai intercepte e confirme (ex: "Tem alterações não salvas, deseja fechar?")

```svelte
<TabPanel
  on:close={handleClose}
  on:closeRequest={handleCloseRequest}
>

<script>
  // Fechamento direto (×, Delete)
  function handleClose({ detail }) {
    chatTabs = chatTabs.filter(t => t.id !== detail.tabId);
  }
  
  // Fechamento via atalho - pode confirmar
  function handleCloseRequest({ detail }) {
    if (hasUnsavedChanges(detail.tabId)) {
      showConfirmDialog(detail);
    } else {
      handleClose({ detail });
    }
  }
</script>
```

## Slots

### Slot padrão

O slot padrão recebe `tab` e `tabId` da aba ativa:

```svelte
<TabPanel {tabs} bind:activeTab>
  <svelte:fragment let:tab let:tabId>
    <h2>{tab.label}</h2>
    <p>Conteúdo de {tabId}</p>
  </svelte:fragment>
</TabPanel>
```

### Slot nomeado `tab-content`

Para renderizar conteúdo específico por aba:

```svelte
<TabPanel {tabs} bind:activeTab>
  <svelte:fragment slot="tab-content" let:tab let:tabId>
    {#if tabId === 'home'}
      <HomeContent />
    {:else if tabId === 'profile'}
      <ProfileContent />
    {/if}
  </svelte:fragment>
</TabPanel>
```

## Navegação por Teclado

### Atalhos Globais (funcionam de qualquer lugar no componente)

| Tecla | Ação | Evento |
|-------|------|--------|
| `Ctrl+Tab` | Próxima aba | `change` |
| `Ctrl+Shift+Tab` | Aba anterior | `change` |
| `Ctrl+T` | Nova aba (se `addable=true`) | `add` |
| `Ctrl+W` | Solicita fechar aba atual | `closeRequest` |
| `Ctrl+F4` | Solicita fechar aba atual | `closeRequest` |

### Na Lista de Abas (Horizontal)

| Tecla | Ação |
|-------|------|
| `→` | Próxima aba (foca e seleciona) |
| `←` | Aba anterior (foca e seleciona) |
| `Home` | Primeira aba |
| `End` | Última aba |
| `Delete` | Fecha aba (se fechável) |
| `Enter` / `Space` | Ativa aba focada |
| `Tab` | Sai da lista de abas (vai para o conteúdo) |

### Na Lista de Abas (Vertical)

| Tecla | Ação |
|-------|------|
| `↓` | Próxima aba (foca e seleciona) |
| `↑` | Aba anterior (foca e seleciona) |
| `Home` | Primeira aba |
| `End` | Última aba |
| `Delete` | Fecha aba (se fechável) |
| `Enter` / `Space` | Ativa aba focada |
| `Tab` | Sai da lista de abas (vai para o conteúdo) |

### Comportamento do Tab

- **Tab** navega normalmente para fora da lista de abas
- Ao entrar na lista de abas com Tab, o foco vai para a aba ativa
- Use **setas** para navegar entre abas
- O botão de fechar **não** recebe foco via Tab (use Delete ou clique)

## Exemplos

### Tabs Verticais

```svelte
<TabPanel 
  {tabs} 
  bind:activeTab
  orientation="vertical"
>
  <div class="content">{activeTab}</div>
</TabPanel>
```

### Tabs Fecháveis com Botão Adicionar

Ideal para múltiplas conversas de chat:

```svelte
<script>
  let chatTabs = [
    { id: 'chat-1', label: 'Chat 1', icon: '💬' }
  ];
  let activeChatTab = 'chat-1';
  let nextId = 2;
  
  function handleAdd() {
    const newTab = {
      id: `chat-${nextId}`,
      label: `Chat ${nextId}`,
      icon: '💬'
    };
    chatTabs = [...chatTabs, newTab];
    activeChatTab = newTab.id;
    nextId++;
  }
  
  function handleClose({ detail }) {
    chatTabs = chatTabs.filter(t => t.id !== detail.tabId);
  }
</script>

<TabPanel 
  tabs={chatTabs}
  bind:activeTab={activeChatTab}
  closableTabs
  addable
  addLabel="Nova conversa"
  on:add={handleAdd}
  on:close={handleClose}
>
  <ChatComponent chatId={activeChatTab} />
</TabPanel>
```

### Tabs Reordenáveis

```svelte
<script>
  let tabs = [...];
  
  function handleReorder({ detail }) {
    const { fromIndex, toIndex } = detail;
    const newTabs = [...tabs];
    const [removed] = newTabs.splice(fromIndex, 1);
    newTabs.splice(toIndex, 0, removed);
    tabs = newTabs;
  }
</script>

<TabPanel 
  {tabs}
  bind:activeTab
  reorderable
  on:reorder={handleReorder}
>
  <Content />
</TabPanel>
```

### Variante Pills

```svelte
<TabPanel 
  {tabs}
  bind:activeTab
  variant="pills"
  align="center"
>
  <Content />
</TabPanel>
```

### Variante Underline

```svelte
<TabPanel 
  {tabs}
  bind:activeTab
  variant="underline"
>
  <Content />
</TabPanel>
```

### Tabs Verticais para Sidebar

```svelte
<div class="app-layout">
  <TabPanel 
    {tabs}
    bind:activeTab
    orientation="vertical"
    variant="pills"
  >
    <MainContent tabId={activeTab} />
  </TabPanel>
</div>

<style>
  .app-layout {
    height: 100vh;
  }
</style>
```

## Estilização

O componente usa CSS custom properties do sistema de design:

```css
/* Cores */
--color-bg-primary      /* Fundo do conteúdo */
--color-bg-secondary    /* Fundo da lista de tabs */
--color-bg-tertiary     /* Hover das tabs */
--color-text-primary    /* Texto ativo */
--color-text-muted      /* Texto inativo */
--color-border          /* Bordas */
--color-accent          /* Focus ring e underline */
--color-accent-dark     /* Tab ativa em pills */
--color-error           /* Botão fechar hover */

/* Espaçamentos */
--spacing-xs            /* Gap entre tabs */
--spacing-sm            /* Padding interno */
--spacing-md            /* Padding tabs */
--spacing-lg            /* Padding tabs grandes */

/* Outros */
--border-radius         /* Arredondamento padrão */
--border-radius-lg      /* Arredondamento pills */
--font-size-sm          /* Fonte tabs sm */
--font-size-base        /* Fonte tabs lg */
--transition-fast       /* Animações */
```

## Acessibilidade

O componente implementa o padrão WAI-ARIA Tabs com suporte completo a leitores de tela (NVDA, JAWS, VoiceOver):

### Atributos ARIA

| Elemento | Atributos | Descrição |
|----------|-----------|-----------|
| Lista de abas | `role="tablist"`, `aria-label`, `aria-orientation` | Container semântico com label e orientação |
| Cada aba | `role="tab"`, `aria-selected`, `aria-controls`, `aria-posinset`, `aria-setsize` | Botão de aba com estado e posição |
| Painel | `role="tabpanel"`, `aria-labelledby`, `tabindex="0"` | Área de conteúdo navegável |
| Live region | `role="status"`, `aria-live="polite"` | Anúncios de mudanças para screen readers |

### Anúncios para Leitores de Tela

O componente anuncia automaticamente:

- **Ao selecionar aba**: "Chat 1, aba 2 de 5 selecionada"
- **Ao fechar aba**: "Aba Chat 1 fechada. 4 abas restantes."
- **Ao focar em aba**: "Chat 1, aba 2 de 5, fechável, pressione Delete para fechar"

### Labels Acessíveis

Cada aba tem um `aria-label` completo que inclui:
- Nome da aba
- Posição (ex: "aba 2 de 5")
- Se é fechável (com instrução para Delete)
- Se está desabilitada

### Props de Acessibilidade

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `ariaLabel` | `string` | `'Abas'` | Label para a lista de abas (lido ao focar) |

### Métodos Públicos

| Método | Parâmetros | Descrição |
|--------|------------|-----------|
| `announceNewTab(label)` | `label: string` | Anuncia criação de nova aba |

### Exemplo com Anúncio de Nova Aba

```svelte
<script>
  let tabPanelRef;
  
  function handleAdd() {
    const newTab = { id: `chat-${nextId}`, label: `Chat ${nextId}` };
    chatTabs = [...chatTabs, newTab];
    activeChat = newTab.id;
    nextId++;
    
    // Anuncia para screen readers
    tabPanelRef.announceNewTab(newTab.label);
  }
</script>

<TabPanel 
  bind:this={tabPanelRef}
  tabs={chatTabs}
  bind:activeTab={activeChat}
  ariaLabel="Conversas de chat"
  closableTabs
  addable
  on:add={handleAdd}
>
  ...
</TabPanel>
```

### Dicas de Acessibilidade

1. **Use `ariaLabel`** descritivo: "Conversas de chat" ao invés de apenas "Abas"
2. **Labels descritivos nas abas**: "Configurações gerais" ao invés de "Config"
3. **Ícones são decorativos**: Use `aria-hidden="true"` (já implementado)
4. **Teste com NVDA/JAWS**: Navegue com setas e Tab, verifique anúncios
5. **Cores de alto contraste**: O componente respeita `prefers-contrast: high`
6. **Movimento reduzido**: Animações desabilitadas com `prefers-reduced-motion`

### Comportamento com Leitores de Tela

1. **Tab** para entrar na lista de abas → foca na aba ativa
2. Screen reader anuncia: "Chat 1, aba 2 de 5, selecionada, fechável..."
3. **Setas** para navegar → anuncia cada aba com posição
4. **Delete** para fechar → anuncia "Aba fechada, X abas restantes"
5. **Ctrl+Tab** para alternar → anuncia nova aba selecionada
6. **Tab** para sair → foco vai para o conteúdo do painel

## Migração

Se você tem tabs customizadas, migre para este componente:

```javascript
// Antes (HTML manual)
<div class="tabs">
  <button class="tab">Tab 1</button>
  <button class="tab active">Tab 2</button>
</div>
<div class="content">...</div>

// Depois
import { TabPanel } from './components/tabs';

<TabPanel
  tabs={[
    { id: '1', label: 'Tab 1' },
    { id: '2', label: 'Tab 2' }
  ]}
  activeTab="2"
>
  ...
</TabPanel>
```

## Casos de Uso

1. **Múltiplas conversas de chat** - Com `closableTabs` e `addable`
2. **Configurações em seções** - Com `variant="underline"`
3. **Navegação em sidebar** - Com `orientation="vertical"`
4. **Wizard/Steps** - Com `variant="pills"` e tabs desabilitadas
5. **Editor com múltiplas abas** - Com `reorderable` e `closableTabs`

