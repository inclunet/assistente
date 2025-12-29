<script>
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  
  export let value = '';
  export let schema = {};  // JSON Schema dos parâmetros para autocomplete
  export let placeholder = '';
  export let height = '100px';
  export let singleLine = false;
  export let readonly = false;
  
  const dispatch = createEventDispatcher();
  
  let container;
  let editor;
  let monaco;
  
  // Funções disponíveis nos templates Go
  const templateFunctions = [
    { name: 'urlEncode', description: 'Escapa string para URL', example: '{{.value | urlEncode}}' },
    { name: 'jsonEncode', description: 'Converte para JSON', example: '{{.value | jsonEncode}}' },
    { name: 'base64Encode', description: 'Codifica em Base64', example: '{{.value | base64Encode}}' },
    { name: 'base64Decode', description: 'Decodifica Base64', example: '{{.value | base64Decode}}' },
    { name: 'lower', description: 'Converte para minúsculas', example: '{{.value | lower}}' },
    { name: 'upper', description: 'Converte para MAIÚSCULAS', example: '{{.value | upper}}' },
    { name: 'trim', description: 'Remove espaços', example: '{{.value | trim}}' },
    { name: 'replace', description: 'Substitui texto', example: '{{.value | replace "old" "new"}}' },
    { name: 'default', description: 'Valor padrão se vazio', example: '{{.value | default "fallback"}}' },
    { name: 'required', description: 'Erro se vazio', example: '{{.value | required}}' },
    { name: 'now', description: 'Data/hora atual', example: '{{now}}' },
    { name: 'formatDate', description: 'Formata data', example: '{{now | formatDate "2006-01-02"}}' },
    { name: 'add', description: 'Soma números', example: '{{add .a .b}}' },
    { name: 'sub', description: 'Subtrai números', example: '{{sub .a .b}}' },
    { name: 'mul', description: 'Multiplica números', example: '{{mul .a .b}}' },
    { name: 'div', description: 'Divide números', example: '{{div .a .b}}' },
    { name: 'isEmpty', description: 'Verifica se vazio', example: '{{if isEmpty .value}}...{{end}}' },
    { name: 'ternary', description: 'Operador ternário', example: '{{ternary .condition "yes" "no"}}' },
  ];
  
  // Variáveis globais sempre disponíveis
  const globalVariables = [
    { name: 'env', description: 'Variáveis de ambiente', example: '{{.env.VAR_NAME}}' },
    { name: 'agent.name', description: 'Nome do agente', example: '{{.agent.name}}' },
    { name: 'agent.display_name', description: 'Nome de exibição', example: '{{.agent.display_name}}' },
    { name: 'request_id', description: 'ID único da request', example: '{{.request_id}}' },
    { name: 'timestamp', description: 'Data/hora da request', example: '{{.timestamp}}' },
  ];
  
  onMount(async () => {
    // Import Monaco dinamicamente
    monaco = await import('monaco-editor');
    
    // Registra tema escuro customizado
    monaco.editor.defineTheme('gotemplate-dark', {
      base: 'vs-dark',
      inherit: true,
      rules: [
        { token: 'delimiter.bracket', foreground: '58a6ff' },
        { token: 'variable', foreground: '7ee787' },
        { token: 'function', foreground: 'd2a8ff' },
      ],
      colors: {
        'editor.background': '#1e1e1e',
      }
    });
    
    // Cria o editor
    editor = monaco.editor.create(container, {
      value: value,
      language: 'plaintext',
      theme: 'gotemplate-dark',
      minimap: { enabled: false },
      lineNumbers: singleLine ? 'off' : 'on',
      scrollBeyondLastLine: false,
      wordWrap: 'on',
      readOnly: readonly,
      fontSize: 13,
      fontFamily: "'Fira Code', 'Consolas', monospace",
      padding: { top: 8, bottom: 8 },
      lineHeight: 20,
      renderLineHighlight: singleLine ? 'none' : 'line',
      overviewRulerLanes: 0,
      hideCursorInOverviewRuler: true,
      scrollbar: {
        vertical: singleLine ? 'hidden' : 'auto',
        horizontal: 'hidden',
        verticalScrollbarSize: 8,
      },
      automaticLayout: true,
    });
    
    // Configura autocomplete
    setupAutoComplete();
    
    // Listener de mudanças
    editor.onDidChangeModelContent(() => {
      const newValue = editor.getValue();
      if (newValue !== value) {
        value = newValue;
        dispatch('change', { value: newValue });
      }
    });
    
    // Listener de blur
    editor.onDidBlurEditorWidget(() => {
      dispatch('blur', { value: editor.getValue() });
    });
  });
  
  onDestroy(() => {
    if (editor) {
      editor.dispose();
    }
  });
  
  // Atualiza o valor quando a prop muda externamente
  $: if (editor && value !== editor.getValue()) {
    editor.setValue(value);
  }
  
  // Atualiza autocomplete quando o schema muda
  $: if (monaco && schema) {
    setupAutoComplete();
  }
  
  function setupAutoComplete() {
    if (!monaco) return;
    
    // Remove providers anteriores (simplificado - na prática precisaria de um dispose)
    monaco.languages.registerCompletionItemProvider('plaintext', {
      triggerCharacters: ['.', '|', '{'],
      provideCompletionItems: (model, position) => {
        const textUntilPosition = model.getValueInRange({
          startLineNumber: 1,
          startColumn: 1,
          endLineNumber: position.lineNumber,
          endColumn: position.column
        });
        
        const suggestions = [];
        
        // Detecta contexto
        const inTemplate = textUntilPosition.match(/\{\{[^}]*$/);
        const afterPipe = textUntilPosition.match(/\|\s*\w*$/);
        const afterDot = textUntilPosition.match(/\.\w*$/);
        
        if (afterPipe) {
          // Após pipe: sugerir funções
          for (const func of templateFunctions) {
            suggestions.push({
              label: func.name,
              kind: monaco.languages.CompletionItemKind.Function,
              insertText: func.name,
              detail: func.description,
              documentation: `Exemplo: ${func.example}`,
            });
          }
        } else if (afterDot || inTemplate) {
          // Após ponto ou dentro de template: sugerir variáveis
          
          // Variáveis do schema
          const properties = schema?.properties || {};
          for (const [name, prop] of Object.entries(properties)) {
            suggestions.push({
              label: name,
              kind: monaco.languages.CompletionItemKind.Variable,
              insertText: name,
              detail: `${prop.type || 'any'} - Parâmetro`,
              documentation: prop.description || '',
            });
          }
          
          // Variáveis globais
          for (const v of globalVariables) {
            suggestions.push({
              label: v.name,
              kind: monaco.languages.CompletionItemKind.Property,
              insertText: v.name,
              detail: 'Global',
              documentation: v.description,
            });
          }
        }
        
        // Se não estamos em contexto especial, sugerir início de template
        if (!inTemplate && textUntilPosition.slice(-1) === '{') {
          suggestions.push({
            label: '{{.}}',
            kind: monaco.languages.CompletionItemKind.Snippet,
            insertText: '{.${1:variable}}}',
            insertTextRules: monaco.languages.CompletionItemInsertTextRule.InsertAsSnippet,
            detail: 'Template variable',
            documentation: 'Insere uma variável de template',
          });
        }
        
        return { suggestions };
      }
    });
  }
  
  // Métodos públicos
  export function getValue() {
    return editor?.getValue() || value;
  }
  
  export function setValue(newValue) {
    if (editor) {
      editor.setValue(newValue);
    }
    value = newValue;
  }
  
  export function focus() {
    editor?.focus();
  }
</script>

<div 
  class="monaco-container"
  class:single-line={singleLine}
  style="height: {height};"
  bind:this={container}
>
</div>

{#if placeholder && !value}
  <div class="placeholder">{placeholder}</div>
{/if}

<style>
  .monaco-container {
    position: relative;
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    overflow: hidden;
  }
  
  .monaco-container:focus-within {
    border-color: var(--color-accent);
    outline: none;
  }
  
  .monaco-container.single-line {
    height: 36px !important;
  }
  
  .placeholder {
    position: absolute;
    top: 8px;
    left: 12px;
    color: var(--color-text-muted);
    pointer-events: none;
    font-size: 13px;
    font-family: 'Fira Code', 'Consolas', monospace;
  }
  
  :global(.monaco-editor) {
    border-radius: var(--border-radius);
  }
  
  :global(.monaco-editor .margin) {
    background-color: transparent !important;
  }
</style>



