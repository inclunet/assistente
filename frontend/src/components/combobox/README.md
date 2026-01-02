# Combobox Component

Combobox/Select acessível com filtro em tempo real e navegação completa por teclado.

## Features

- ✅ **Filtro em tempo real** - Digite para filtrar opções
- ✅ **Navegação por teclado** (↑↓, Enter, Escape, Home, End)
- ✅ **Anúncios para leitores de tela** - aria-live
- ✅ **Sublabels** - Texto secundário nas opções
- ✅ **Ícone customizável** - Emoji ou ícone no botão
- ✅ **Restauração de foco** - Retorna ao botão ao fechar
- ✅ **ARIA completo** - role="listbox", aria-activedescendant

## Instalação

```javascript
import { Combobox } from './components/combobox';
```

## Uso Básico

```svelte
<script>
  import { Combobox } from './components/combobox';
  
  let selectedModel = '';
  
  const models = [
    { value: 'gpt-4', label: 'GPT-4' },
    { value: 'gpt-3.5', label: 'GPT-3.5 Turbo' },
    { value: 'claude', label: 'Claude 3' },
  ];
</script>

<Combobox
  icon="🤖"
  label="Modelo"
  items={models}
  bind:selected={selectedModel}
  on:change={(e) => console.log('Selecionado:', e.detail)}
/>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `icon` | `string` | `'🔧'` | Ícone exibido no botão |
| `label` | `string` | `'Selecionar'` | Label quando nenhum item selecionado |
| `items` | `Array` | `[]` | Array de opções `[{value, label, sublabel?}]` |
| `selected` | `string` | `''` | Value do item selecionado |
| `placeholder` | `string` | `'Filtrar...'` | Placeholder do campo de filtro |
| `disabled` | `boolean` | `false` | Desabilita o combobox |
| `maxWidth` | `string` | `'180px'` | Largura máxima do botão |

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `change` | `{ value, label, item }` | Item selecionado mudou |
| `open` | - | Dropdown abriu |
| `close` | - | Dropdown fechou |

## Estrutura de Item

```typescript
interface ComboboxItem {
  value: string;      // Valor único (usado em selected)
  label: string;      // Texto principal
  sublabel?: string;  // Texto secundário (opcional)
}
```

## Navegação por Teclado

### Botão Fechado

| Tecla | Ação |
|-------|------|
| `Enter` / `Space` | Abre o dropdown |
| `↓` | Abre o dropdown |

### Dropdown Aberto

| Tecla | Ação |
|-------|------|
| `↑` / `↓` | Navega entre opções |
| `Home` | Primeira opção |
| `End` | Última opção |
| `Enter` | Seleciona opção destacada |
| `Escape` | Fecha sem selecionar |
| `Tab` | Fecha e move foco |
| `A-Z` | Filtra opções |

## Filtro

O campo de filtro busca em `label` e `sublabel`:

```svelte
<script>
  const items = [
    { value: 'br', label: 'Brasil', sublabel: 'América do Sul' },
    { value: 'us', label: 'Estados Unidos', sublabel: 'América do Norte' },
    { value: 'jp', label: 'Japão', sublabel: 'Ásia' },
  ];
</script>

<!-- Digitar "sul" encontra "Brasil" pelo sublabel -->
<Combobox {items} bind:selected />
```

## Sublabels

Use sublabels para informações adicionais:

```svelte
<script>
  const models = [
    { value: 'gpt-4', label: 'GPT-4', sublabel: 'Mais capaz, mais lento' },
    { value: 'gpt-3.5', label: 'GPT-3.5', sublabel: 'Rápido, econômico' },
  ];
</script>

<Combobox
  icon="🤖"
  label="Modelo"
  items={models}
  bind:selected
/>

<!-- Renderiza:
┌─────────────────────────┐
│ GPT-4                   │
│ Mais capaz, mais lento  │ ← sublabel em cinza
├─────────────────────────┤
│ GPT-3.5                 │
│ Rápido, econômico       │
└─────────────────────────┘
-->
```

## Métodos

```javascript
let combobox;

// Abre o dropdown programaticamente
combobox.open();

// Fecha o dropdown
combobox.close();
```

## Estilização

O componente usa CSS custom properties:

```css
/* Cores */
--color-bg-secondary    /* Fundo do botão */
--color-bg-tertiary     /* Fundo do dropdown */
--color-bg-primary      /* Fundo do item hover */
--color-border          /* Bordas */
--color-accent          /* Destaque de seleção */
--color-text-primary    /* Texto principal */
--color-text-muted      /* Sublabel */

/* Espaçamentos */
--spacing-xs            /* Padding pequeno */
--spacing-sm            /* Padding médio */

/* Outros */
--border-radius         /* Arredondamento */
--font-size-sm          /* Tamanho da fonte */
--font-size-xs          /* Tamanho do sublabel */
```

## Exemplos Avançados

### Seletor de Modelo com Ícones

```svelte
<script>
  const models = [
    { value: 'gpt-4', label: '🧠 GPT-4', sublabel: 'OpenAI' },
    { value: 'claude', label: '🎭 Claude 3', sublabel: 'Anthropic' },
    { value: 'gemini', label: '💎 Gemini', sublabel: 'Google' },
  ];
</script>

<Combobox
  icon="🤖"
  label="Selecionar Modelo"
  items={models}
  bind:selected={model}
  maxWidth="220px"
/>
```

### Seletor de Idioma

```svelte
<script>
  const languages = [
    { value: 'pt-BR', label: 'Português', sublabel: 'Brasil' },
    { value: 'en-US', label: 'English', sublabel: 'United States' },
    { value: 'es-ES', label: 'Español', sublabel: 'España' },
  ];
</script>

<Combobox
  icon="🌐"
  label="Idioma"
  items={languages}
  bind:selected={locale}
/>
```

### Com Validação

```svelte
<script>
  let selected = '';
  let error = '';
  
  function validate() {
    if (!selected) {
      error = 'Selecione uma opção';
      return false;
    }
    error = '';
    return true;
  }
</script>

<div class:has-error={error}>
  <Combobox
    items={options}
    bind:selected
    on:change={() => error = ''}
  />
  {#if error}
    <span class="error">{error}</span>
  {/if}
</div>
```

### Carregamento Dinâmico

```svelte
<script>
  let items = [];
  let loading = true;
  
  onMount(async () => {
    items = await fetchOptions();
    loading = false;
  });
</script>

<Combobox
  items={items}
  bind:selected
  disabled={loading}
  label={loading ? 'Carregando...' : 'Selecionar'}
/>
```

## Acessibilidade

O componente implementa:

- `role="listbox"` - Semântica de lista de opções
- `role="option"` - Cada item
- `aria-activedescendant` - Item destacado
- `aria-selected` - Item selecionado
- `aria-expanded` - Estado do dropdown
- `aria-live="polite"` - Anúncios de navegação
- **Navegação completa** por teclado
- **Restauração de foco** ao fechar

## Migração

Se você estava importando de `./components/ComboboxPicker.svelte`, atualize para:

```javascript
// Antes
import ComboboxPicker from './components/ComboboxPicker.svelte';

// Depois
import { Combobox } from './components/combobox';
// ou para compatibilidade:
import { ComboboxPicker } from './components/combobox';
```


