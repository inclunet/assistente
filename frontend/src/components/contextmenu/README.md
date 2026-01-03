# Context Menu Components

Sistema de menu de contexto acessível e altamente customizável para Svelte.

## Features

- ✅ **Navegação completa por teclado** (↑↓←→, Home, End, Escape, Tab)
- ✅ **Submenus aninhados** com navegação intuitiva
- ✅ **Type-ahead** (digite a primeira letra para navegar)
- ✅ **Posicionamento automático** (evita sair da tela)
- ✅ **Restauração de foco** ao fechar
- ✅ **Anúncios para leitores de tela** (aria-live)
- ✅ **prefers-reduced-motion** respeitado
- ✅ **Registro global** para fechar outros menus abertos
- ✅ **Separadores, ícones, atalhos de teclado**
- ✅ **Estilo "danger"** para ações destrutivas
- ✅ **ARIA completo** (role="menu", aria-activedescendant, etc.)

## Instalação

```javascript
import { ContextMenu, ContextMenuTrigger } from './components/contextmenu';
```

## Componentes

### ContextMenu

Menu de contexto programático. Você controla quando abrir/fechar.

```svelte
<script>
  import { ContextMenu } from './components/contextmenu';
  
  let contextMenu;
  let menuItems = [
    { id: 'copy', label: 'Copiar', icon: '📋', shortcut: 'Ctrl+C' },
    { id: 'paste', label: 'Colar', icon: '📄', shortcut: 'Ctrl+V' },
    { separator: true },
    { id: 'delete', label: 'Excluir', icon: '🗑️', danger: true },
  ];
  
  function handleContextMenu(event) {
    event.preventDefault();
    contextMenu.open(event.clientX, event.clientY);
  }
  
  function handleSelect(event) {
    const { id, item } = event.detail;
    console.log('Selecionado:', id);
    // Execute a ação
  }
</script>

<div on:contextmenu={handleContextMenu}>
  Clique direito aqui
</div>

<ContextMenu
  bind:this={contextMenu}
  items={menuItems}
  ariaLabel="Menu de ações"
  on:select={handleSelect}
  on:close={() => console.log('Menu fechado')}
/>
```

### ContextMenuTrigger

Wrapper que adiciona menu de contexto automaticamente a qualquer elemento.

```svelte
<script>
  import { ContextMenuTrigger } from './components/contextmenu';
  
  let items = [
    { id: 'edit', label: 'Editar', icon: '✏️' },
    { id: 'share', label: 'Compartilhar', icon: '🔗' },
  ];
  
  function handleSelect(event) {
    console.log('Ação:', event.detail.id);
  }
</script>

<ContextMenuTrigger {items} ariaLabel="Opções do card" on:select={handleSelect}>
  <div class="card">
    <h3>Meu Card</h3>
    <p>Clique direito para opções</p>
  </div>
</ContextMenuTrigger>
```

## Estrutura de MenuItem

```typescript
interface MenuItem {
  // Identificador único (obrigatório se não for separator)
  id: string;
  
  // Texto exibido
  label: string;
  
  // Emoji ou ícone (opcional)
  icon?: string;
  
  // Atalho de teclado exibido (apenas visual, não funcional)
  shortcut?: string;
  
  // Item desabilitado
  disabled?: boolean;
  
  // Separador (ignora outros campos)
  separator?: boolean;
  
  // Estilo vermelho para ações perigosas
  danger?: boolean;
  
  // Submenu (array de MenuItems)
  submenu?: MenuItem[];
  
  // Função a executar (alternativa a on:select)
  action?: () => void;
}
```

## Props

### ContextMenu

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `items` | `MenuItem[]` | `[]` | Array de itens do menu |
| `x` | `number` | `0` | Posição X inicial |
| `y` | `number` | `0` | Posição Y inicial |
| `visible` | `boolean` | `false` | Visibilidade do menu |
| `ariaLabel` | `string` | `'Menu de contexto'` | Label para acessibilidade |

### ContextMenuTrigger

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `items` | `MenuItem[]` | `[]` | Array de itens do menu |
| `ariaLabel` | `string` | `'Menu de contexto'` | Label para acessibilidade |
| `disabled` | `boolean` | `false` | Desabilita o trigger |

## Eventos

### ContextMenu

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `select` | `{ id: string, item: MenuItem }` | Item selecionado |
| `close` | - | Menu fechado |

### ContextMenuTrigger

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `select` | `{ id: string, item: MenuItem }` | Item selecionado |

## Métodos (ContextMenu)

```javascript
// Abre o menu na posição especificada
contextMenu.open(x, y);

// Fecha o menu
contextMenu.close();
```

## Navegação por Teclado

| Tecla | Ação |
|-------|------|
| `↑` / `↓` | Navegar entre itens |
| `→` | Abrir submenu |
| `←` | Fechar submenu |
| `Enter` / `Space` | Selecionar item |
| `Escape` | Fechar menu/submenu |
| `Tab` | Fechar menu |
| `Home` | Ir para primeiro item |
| `End` | Ir para último item |
| `A-Z` | Type-ahead (primeira letra) |

## Submenus

```svelte
<script>
  let items = [
    { id: 'file', label: 'Arquivo', icon: '📁' },
    { 
      id: 'export', 
      label: 'Exportar', 
      icon: '📤',
      submenu: [
        { id: 'export-pdf', label: 'PDF', icon: '📕' },
        { id: 'export-docx', label: 'Word', icon: '📘' },
        { id: 'export-txt', label: 'Texto', icon: '📄' },
      ]
    },
    { separator: true },
    { id: 'settings', label: 'Configurações', icon: '⚙️' },
  ];
</script>
```

## Estilização

O componente usa CSS custom properties para estilização:

```css
/* Cores */
--color-bg-secondary    /* Fundo do menu */
--color-bg-tertiary     /* Fundo do item hover */
--color-bg-primary      /* Fundo do shortcut */
--color-border          /* Bordas */
--color-text-primary    /* Texto principal */
--color-text-muted      /* Texto secundário */
--color-error           /* Texto danger */

/* Espaçamentos */
--spacing-xs            /* Padding pequeno */
--spacing-sm            /* Gap entre elementos */
--spacing-md            /* Padding médio */

/* Outros */
--border-radius         /* Arredondamento */
--font-size-sm          /* Tamanho da fonte */
--font-size-xs          /* Tamanho do shortcut */
```

## Acessibilidade

O componente implementa:

- `role="menu"` e `role="menuitem"`
- `aria-activedescendant` para indicar item focado
- `aria-disabled` para itens desabilitados
- `aria-haspopup` e `aria-expanded` para submenus
- `aria-live="polite"` para anúncios
- Foco restaurado ao elemento anterior ao fechar
- Navegação completa por teclado

## Registro Global

O componente automaticamente fecha outros menus abertos quando um novo é aberto.
Para uso avançado, você pode registrar sua própria função de fechamento:

```javascript
import { registerCloseAll } from './components/contextmenu';

const unregister = registerCloseAll(() => {
  // Sua lógica de fechamento
});

// Quando não precisar mais
unregister();
```

## Exemplos Avançados

### Com Ações Diretas

```svelte
<script>
  let items = [
    { 
      id: 'copy', 
      label: 'Copiar', 
      icon: '📋',
      action: () => navigator.clipboard.writeText(selectedText)
    },
    { 
      id: 'search', 
      label: 'Pesquisar no Google', 
      icon: '🔍',
      action: () => window.open(`https://google.com/search?q=${selectedText}`)
    },
  ];
</script>

<ContextMenu 
  {items} 
  on:select={(e) => e.detail.item.action?.()} 
/>
```

### Menu Dinâmico

```svelte
<script>
  let selectedItem = null;
  
  $: menuItems = [
    { id: 'view', label: 'Visualizar', icon: '👁️' },
    { id: 'edit', label: 'Editar', icon: '✏️', disabled: !selectedItem?.editable },
    { separator: true },
    { 
      id: 'delete', 
      label: 'Excluir', 
      icon: '🗑️', 
      danger: true,
      disabled: selectedItem?.protected 
    },
  ];
</script>
```

### Integração com Chat Components

```svelte
<script>
  import { ContextMenu } from './components/contextmenu';
  import { ChatContainer, getMessageMenuItems } from './components/chat';
  
  let contextMenu;
  let menuItems = [];
  
  function handleContextMenu(event) {
    const { message, x, y } = event.detail;
    
    menuItems = getMessageMenuItems(message, {
      config: { showCopy: true, showEdit: true },
      extraItems: [
        { id: 'tts', label: 'Ouvir', icon: '🔊', position: 'afterCopy' }
      ],
      handlers: {
        onCopied: () => showToast('Copiado!'),
      }
    });
    
    contextMenu.open(x, y);
  }
</script>

<ChatContainer on:contextMenu={handleContextMenu} />

<ContextMenu
  bind:this={contextMenu}
  {menuItems}
  on:select={(e) => e.detail.item.action?.()}
/>
```

## Migração

Se você estava importando de `./components/ContextMenu.svelte`, atualize para:

```javascript
// Antes
import ContextMenu from './components/ContextMenu.svelte';
import ContextMenuTrigger from './components/ContextMenuTrigger.svelte';

// Depois
import { ContextMenu, ContextMenuTrigger } from './components/contextmenu';
```




