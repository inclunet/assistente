# Toolbar Component

Componente de toolbar acessível seguindo o padrão WAI-ARIA para navegação por teclado.

## Features

- ✅ **Roving tabindex** - Um único tab stop, navegação interna com setas
- ✅ **Navegação por teclado** (←→↑↓, Home, End)
- ✅ **MutationObserver** - Detecta mudanças dinâmicas nos itens
- ✅ **role="toolbar"** - Semântica correta para leitores de tela
- ✅ **Slots flexíveis** - Coloque qualquer conteúdo dentro
- ✅ **Estilos globais** - Classes utilitárias para botões e separadores

## Instalação

```javascript
import { Toolbar } from './components/toolbar';
```

## Uso Básico

```svelte
<script>
  import { Toolbar } from './components/toolbar';
</script>

<Toolbar label="Ferramentas do editor">
  <button class="toolbar-btn" on:click={handleNew}>
    📄 Novo
  </button>
  <button class="toolbar-btn" on:click={handleSave}>
    💾 Salvar
  </button>
  <div class="toolbar-separator"></div>
  <button class="toolbar-btn" on:click={handleSettings}>
    ⚙️ Configurações
  </button>
</Toolbar>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `label` | `string` | `'Barra de ferramentas'` | aria-label para acessibilidade |

## Navegação por Teclado

| Tecla | Ação |
|-------|------|
| `Tab` | Entra/sai da toolbar (único tab stop) |
| `←` / `↑` | Item anterior (circular) |
| `→` / `↓` | Próximo item (circular) |
| `Home` | Primeiro item |
| `End` | Último item |
| `Enter` / `Space` | Ativa o item focado |

## Padrão WAI-ARIA: Roving Tabindex

A toolbar implementa o padrão [Roving Tabindex](https://www.w3.org/WAI/ARIA/apg/practices/keyboard-interface/#kbd_roving_tabindex):

1. Apenas **um item** tem `tabindex="0"` por vez
2. Todos os outros têm `tabindex="-1"`
3. Setas movem o foco E atualizam os tabindexes
4. Tab entra/sai da toolbar como um grupo

```
┌─────────────────────────────────────────────────────┐
│ [Tab] →  Toolbar                                     │
│          ↓                                           │
│   ┌─────────────────────────────────────────────┐   │
│   │ [Novo]  [Salvar]  │  [Config]  [Ajuda]      │   │
│   │   ↑        ↓                                 │   │
│   │   ←────────→  (setas navegam)               │   │
│   └─────────────────────────────────────────────┘   │
│          ↓                                           │
│ [Tab] →  Próximo elemento da página                  │
└─────────────────────────────────────────────────────┘
```

## Elementos Focáveis

A toolbar detecta automaticamente elementos focáveis:

- `<button>` (não disabled)
- `[role="button"]` (não aria-disabled)
- `.picker-button` (não disabled)

## Classes CSS Utilitárias

O componente fornece estilos globais para elementos dentro da toolbar:

### `.toolbar-btn`

Botão padrão da toolbar:

```svelte
<Toolbar>
  <button class="toolbar-btn">
    📄 Novo
  </button>
</Toolbar>
```

Estilos aplicados:
- Padding e gap consistentes
- Hover com borda accent
- Focus ring visível
- Estado disabled

### `.toolbar-separator`

Separador visual entre grupos de botões:

```svelte
<Toolbar>
  <button class="toolbar-btn">Arquivo</button>
  <button class="toolbar-btn">Editar</button>
  <div class="toolbar-separator"></div>
  <button class="toolbar-btn">Ajuda</button>
</Toolbar>
```

### `.toolbar-spacer`

Espaçador flexível para empurrar itens para a direita:

```svelte
<Toolbar>
  <button class="toolbar-btn">Esquerda</button>
  <div class="toolbar-spacer"></div>
  <button class="toolbar-btn">Direita</button>
</Toolbar>
```

### `.loading-status`

Indicador de status/loading:

```svelte
<Toolbar>
  <button class="toolbar-btn">Salvar</button>
  {#if isSaving}
    <span class="loading-status">
      <Spinner /> Salvando...
    </span>
  {/if}
</Toolbar>
```

## MutationObserver

A toolbar usa `MutationObserver` para detectar mudanças dinâmicas:

- Botões adicionados/removidos
- Mudanças no atributo `disabled`
- Atualização automática dos tabindexes

Isso significa que você pode adicionar/remover botões dinamicamente e a navegação continuará funcionando:

```svelte
<Toolbar>
  {#each tools as tool}
    <button class="toolbar-btn" disabled={tool.disabled}>
      {tool.icon} {tool.label}
    </button>
  {/each}
</Toolbar>
```

## Estilização

O componente usa CSS custom properties:

```css
/* Espaçamentos */
--spacing-sm       /* Gap entre itens */
--spacing-xs       /* Padding interno */
--spacing-lg       /* Padding lateral */

/* Cores */
--color-bg-tertiary   /* Fundo da toolbar */
--color-bg-secondary  /* Fundo dos botões */
--color-bg-primary    /* Fundo hover */
--color-border        /* Bordas */
--color-accent        /* Cor de destaque */
--color-text-primary  /* Texto */
--color-text-muted    /* Texto secundário */

/* Outros */
--border-radius       /* Arredondamento */
--font-size-sm        /* Tamanho da fonte */
```

## Exemplos Avançados

### Com Pickers/Dropdowns

```svelte
<script>
  import { Toolbar } from './components/toolbar';
  import ModelPicker from './ModelPicker.svelte';
  import VoicePicker from './VoicePicker.svelte';
</script>

<Toolbar label="Configurações do chat">
  <button class="toolbar-btn" on:click={newChat}>
    🆕 Nova Conversa
  </button>
  
  <div class="toolbar-separator"></div>
  
  <ModelPicker bind:selectedModel />
  <VoicePicker bind:selectedVoice />
  
  <div class="toolbar-spacer"></div>
  
  <button class="toolbar-btn" on:click={openSettings}>
    ⚙️
  </button>
</Toolbar>
```

### Com Estados Dinâmicos

```svelte
<script>
  import { Toolbar } from './components/toolbar';
  
  let isRecording = false;
  let isSaving = false;
</script>

<Toolbar label="Editor de áudio">
  <button 
    class="toolbar-btn" 
    class:active={isRecording}
    on:click={() => isRecording = !isRecording}
  >
    {isRecording ? '⏹️ Parar' : '🎤 Gravar'}
  </button>
  
  <button 
    class="toolbar-btn" 
    disabled={isSaving}
    on:click={save}
  >
    💾 Salvar
  </button>
  
  {#if isSaving}
    <span class="loading-status">Salvando...</span>
  {/if}
</Toolbar>
```

### Toolbar Responsiva

```svelte
<style>
  .toolbar-responsive {
    flex-wrap: wrap;
  }
  
  @media (max-width: 600px) {
    .toolbar-responsive :global(.toolbar-btn span) {
      display: none; /* Esconde texto, mostra só ícones */
    }
  }
</style>

<Toolbar label="Ferramentas" class="toolbar-responsive">
  <button class="toolbar-btn">
    📄 <span>Novo</span>
  </button>
  <button class="toolbar-btn">
    💾 <span>Salvar</span>
  </button>
</Toolbar>
```

## Acessibilidade

O componente implementa:

- `role="toolbar"` - Semântica correta
- `aria-label` - Descrição para leitores de tela
- **Roving tabindex** - Padrão WAI-ARIA
- **Focus visible** - Indicador visual de foco
- Navegação circular com setas

## Comparação com Outras Implementações

| Feature | Esta Toolbar | Toolbar básica |
|---------|--------------|----------------|
| Roving tabindex | ✅ | ❌ |
| Navegação ←→ | ✅ | ❌ |
| Home/End | ✅ | ❌ |
| MutationObserver | ✅ | ❌ |
| role="toolbar" | ✅ | ❌ |
| Focus tracking | ✅ | ❌ |

## Migração

Se você estava importando de `./components/Toolbar.svelte`, atualize para:

```javascript
// Antes
import Toolbar from './components/Toolbar.svelte';

// Depois
import { Toolbar } from './components/toolbar';
```





