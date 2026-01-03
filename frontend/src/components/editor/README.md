# Editor Components

Componentes de editor baseados em Monaco Editor.

## Componentes

| Componente | Descrição |
|------------|-----------|
| `TemplateEditor` | Editor para templates Go com autocomplete de funções e variáveis |

## Instalação

```javascript
import { TemplateEditor } from './components/editor';
```

## Dependências

```bash
npm install monaco-editor
```

---

# TemplateEditor

Editor Monaco especializado para templates Go (`{{.variable}}`) com autocomplete inteligente.

## Features

- ✅ **Monaco Editor** - Editor de código completo
- ✅ **Autocomplete** - Funções de template e variáveis
- ✅ **Tema escuro** - Customizado para templates Go
- ✅ **Schema-aware** - Autocomplete baseado em JSON Schema
- ✅ **Modo single-line** - Para campos inline
- ✅ **Readonly** - Modo somente leitura

## Uso

```svelte
<script>
  import { TemplateEditor } from './components/editor';
  
  let template = 'Hello {{.name}}!';
  
  const schema = {
    properties: {
      name: { type: 'string', description: 'Nome do usuário' },
      email: { type: 'string', description: 'Email do usuário' }
    }
  };
  
  function handleChange(e) {
    console.log('Template:', e.detail.value);
  }
</script>

<TemplateEditor
  bind:value={template}
  {schema}
  height="150px"
  on:change={handleChange}
/>
```

## Props

| Prop | Tipo | Default | Descrição |
|------|------|---------|-----------|
| `value` | `string` | `''` | Conteúdo do editor |
| `schema` | `object` | `{}` | JSON Schema para autocomplete de variáveis |
| `placeholder` | `string` | `''` | Texto placeholder quando vazio |
| `height` | `string` | `'100px'` | Altura do editor |
| `singleLine` | `boolean` | `false` | Modo single-line (sem quebras) |
| `readonly` | `boolean` | `false` | Modo somente leitura |

## Eventos

| Evento | Payload | Descrição |
|--------|---------|-----------|
| `change` | `{ value }` | Conteúdo mudou |
| `blur` | `{ value }` | Editor perdeu foco |

## Métodos

```javascript
let editor;

// Obtém o valor atual
const value = editor.getValue();

// Define um novo valor
editor.setValue('{{.newValue}}');

// Foca no editor
editor.focus();
```

## Autocomplete

O editor oferece autocomplete inteligente:

### Funções de Template (após `|`)

```go
{{.value | urlEncode}}     // Escapa para URL
{{.value | jsonEncode}}    // Converte para JSON
{{.value | base64Encode}}  // Codifica em Base64
{{.value | lower}}         // Minúsculas
{{.value | upper}}         // MAIÚSCULAS
{{.value | trim}}          // Remove espaços
{{.value | default "x"}}   // Valor padrão
{{now | formatDate "2006-01-02"}}  // Formata data
```

### Variáveis Globais (após `.`)

```go
{{.env.VAR_NAME}}       // Variáveis de ambiente
{{.agent.name}}         // Nome do agente
{{.agent.display_name}} // Nome de exibição
{{.request_id}}         // ID único da request
{{.timestamp}}          // Data/hora
```

### Variáveis do Schema

Variáveis definidas no JSON Schema também aparecem no autocomplete:

```javascript
const schema = {
  properties: {
    query: { 
      type: 'string', 
      description: 'Texto da busca' 
    },
    limit: { 
      type: 'integer', 
      description: 'Máximo de resultados' 
    }
  }
};
```

## Modo Single-Line

Para campos inline (sem quebra de linha):

```svelte
<TemplateEditor
  value={headerValue}
  singleLine={true}
  height="36px"
  placeholder="Authorization: Bearer {{.token}}"
/>
```

## Estilização

O componente usa CSS custom properties:

```css
--color-border       /* Borda do editor */
--color-accent       /* Borda quando focado */
--color-text-muted   /* Cor do placeholder */
--border-radius      /* Arredondamento */
```

## Migração

Se você estava importando `MonacoTemplate`:

```javascript
// Antes
import MonacoTemplate from './components/MonacoTemplate.svelte';

// Depois (opção 1 - novo nome)
import { TemplateEditor } from './components/editor';

// Depois (opção 2 - alias de compatibilidade)
import { MonacoTemplate } from './components/editor';
```




