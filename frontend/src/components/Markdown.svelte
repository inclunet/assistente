<script>
  import MarkdownIt from 'markdown-it';
    import DOMPurify from 'dompurify';
  import mermaid from 'mermaid';
  import { onMount, afterUpdate } from 'svelte';
  
  export let content = '';
  export let interactiveButtons = false; // Se true, botões de copiar são focáveis e Monaco Editor é habilitado
  
  let containerElement;
  let mermaidInitialized = false;
  
  // Inicializa Mermaid
  function initMermaid() {
    if (mermaidInitialized) return;
    mermaid.initialize({
      startOnLoad: false,
      theme: 'dark',
      securityLevel: 'loose',
      fontFamily: 'inherit'
    });
    mermaidInitialized = true;
  }
  
  // Inicializa markdown-it com suporte a tabelas e outras extensões
  const md = new MarkdownIt({
    html: false,        // Não permite HTML raw
    xhtmlOut: false,
    breaks: false,      // Não converte \n em <br> (interfere com tabelas)
    linkify: true,      // Converte URLs em links automaticamente
    typographer: true   // Substituições tipográficas
  });
  
  // Configuração do DOMPurify para permitir elementos de tabela
  const purifyConfig = {
    ALLOWED_TAGS: [
      'p', 'br', 'strong', 'em', 'b', 'i', 'u', 's', 'del',
      'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
      'ul', 'ol', 'li',
      'a', 'img',
      'pre', 'code',
      'blockquote',
      'table', 'thead', 'tbody', 'tr', 'th', 'td',
      'hr', 'div', 'span'
    ],
    ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'class', 'target', 'rel', 'tabindex']
  };
  
  // Hook para adicionar tabindex="-1" em links (não interfere na navegação por setas)
  DOMPurify.addHook('afterSanitizeAttributes', function(node) {
    if (node.tagName === 'A') {
      node.setAttribute('tabindex', '-1');
      // Abre links em nova aba
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  });
  
  // Função para processar o markdown
  function renderMarkdown(text) {
    if (!text) return '';
    
    let processed = text;
    
    // 1. Extrai tabelas de dentro de blocos de código markdown/md APENAS
    // O LLM às vezes coloca tabelas dentro de ```markdown ... ```
    // NÃO afeta blocos de código de outras linguagens (python, java, etc.)
    processed = processed.replace(/```(markdown|md)\s*\n([\s\S]*?)```/gi, (match, lang, content) => {
      // Verifica se o conteúdo é uma tabela válida (tem | e linha de separação com ---)
      const trimmedContent = content.trim();
      const hasTableSyntax = trimmedContent.includes('|') && 
                             (trimmedContent.includes('|---') || trimmedContent.includes('| ---') || trimmedContent.includes('|:--'));
      if (hasTableSyntax) {
        return '\n' + trimmedContent + '\n';
      }
      return match; // Mantém como código se não for tabela válida
    });
    
    // 2. Remove linhas vazias dentro de tabelas (LLMs às vezes adicionam)
    const lines = processed.split('\n');
    const result = [];
    let inTable = false;
    
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i].trim();
      const isTableLine = line.startsWith('|') && line.endsWith('|');
      const isEmpty = line === '';
      
      if (isTableLine) {
        inTable = true;
        result.push(lines[i]);
      } else if (isEmpty && inTable) {
        // Verifica se a próxima linha não-vazia ainda é tabela
        let nextNonEmpty = '';
        for (let j = i + 1; j < lines.length; j++) {
          if (lines[j].trim() !== '') {
            nextNonEmpty = lines[j].trim();
            break;
          }
        }
        if (nextNonEmpty.startsWith('|') && nextNonEmpty.endsWith('|')) {
          // Ignora a linha vazia, estamos ainda na tabela
          continue;
        } else {
          // Fim da tabela
          inTable = false;
          result.push(lines[i]);
        }
      } else {
        inTable = false;
        result.push(lines[i]);
      }
    }
    
    processed = result.join('\n');
    
    // Parse com markdown-it
    const parsed = md.render(processed);
    
    return DOMPurify.sanitize(parsed, purifyConfig);
  }
  
  $: html = renderMarkdown(content);
  
  // Adiciona botões de copiar após renderização
  function addCopyButtons() {
    if (!containerElement) return;
    
    // Remove botões existentes para evitar duplicação
    containerElement.querySelectorAll('.copy-btn').forEach(btn => btn.remove());
    
    // Adiciona botão em blocos de código
    containerElement.querySelectorAll('pre').forEach((pre, index) => {
      // Pula se já processado
      if (pre.parentElement?.classList?.contains('code-block')) return;
      if (pre.parentElement?.classList?.contains('mermaid-diagram')) return;
      
      // Detecta a linguagem do código
      const codeElement = pre.querySelector('code');
      let language = 'Código';
      
      if (codeElement && codeElement.className) {
        const classMatch = codeElement.className.match(/language-(\w+)/i);
        if (classMatch) {
          const lang = classMatch[1].toLowerCase();
          // Pula blocos mermaid - serão processados por renderMermaid
          if (lang === 'mermaid') return;
          // Usa o nome da linguagem com primeira letra maiúscula
          language = classMatch[1].charAt(0).toUpperCase() + classMatch[1].slice(1);
        }
      }
      
      // Cria wrapper com acessibilidade
      const wrapper = document.createElement('div');
      wrapper.className = 'code-block';
      wrapper.setAttribute('role', 'group');
      wrapper.setAttribute('aria-label', language);
      pre.parentNode.insertBefore(wrapper, pre);
      wrapper.appendChild(pre);
      
      // Container de botões
      const btnContainer = document.createElement('div');
      btnContainer.className = 'code-buttons';
      
      const copyBtn = document.createElement('button');
      copyBtn.className = 'copy-btn';
      copyBtn.textContent = 'Copiar';
      copyBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      copyBtn.setAttribute('aria-label', `Copiar código ${language}`);
      copyBtn.onclick = async () => {
        const code = pre.textContent;
        try {
          await navigator.clipboard.writeText(code);
          copyBtn.textContent = 'Copiado!';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        } catch (err) {
          copyBtn.textContent = 'Erro';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        }
      };
      btnContainer.appendChild(copyBtn);
      
      // Botão "Editar no Monaco" só aparece quando interactiveButtons está ativo
      if (interactiveButtons) {
        const code = pre.textContent;
        const langLower = language.toLowerCase();
        
        // Container para o Monaco (inicialmente oculto)
        const monacoContainer = document.createElement('div');
        monacoContainer.className = 'monaco-inline-container';
        monacoContainer.style.display = 'none';
        wrapper.insertBefore(monacoContainer, pre);
        
        let editor = null;
        let isEditorMode = false;
        
        const toggleBtn = document.createElement('button');
        toggleBtn.className = 'copy-btn toggle-editor-btn';
        toggleBtn.textContent = 'Editar';
        toggleBtn.setAttribute('tabindex', '0');
        toggleBtn.setAttribute('aria-label', `Editar código ${language} no Monaco Editor`);
        toggleBtn.onclick = async () => {
          isEditorMode = !isEditorMode;
          
          if (isEditorMode) {
            // Ativa modo Monaco
            pre.style.display = 'none';
            monacoContainer.style.display = 'block';
            toggleBtn.textContent = 'Visualizar';
            toggleBtn.setAttribute('aria-label', 'Voltar para visualização do código');
            
            if (!editor) {
              // Carrega Monaco dinamicamente
              const monaco = await import('monaco-editor');
              editor = monaco.editor.create(monacoContainer, {
                value: code,
                language: langLower === 'código' ? 'plaintext' : langLower,
                theme: 'vs-dark',
                readOnly: false,
                minimap: { enabled: false },
                lineNumbers: 'on',
                scrollBeyondLastLine: false,
                wordWrap: 'on',
                fontSize: 14,
                fontFamily: "'Fira Code', 'Consolas', monospace",
                padding: { top: 8, bottom: 8 },
                automaticLayout: true,
                // Acessibilidade para leitores de tela
                accessibilitySupport: 'on',
                ariaLabel: `Editor de código ${language}. Use Ctrl+F1 para opções de acessibilidade.`,
              });
              setTimeout(() => editor.focus(), 50);
            } else {
              editor.focus();
            }
          } else {
            // Volta para visualização
            pre.style.display = 'block';
            monacoContainer.style.display = 'none';
            toggleBtn.textContent = 'Editar';
            toggleBtn.setAttribute('aria-label', `Editar código ${language} no Monaco Editor`);
          }
        };
        btnContainer.appendChild(toggleBtn);
      }
      
      wrapper.appendChild(btnContainer);
    });
    
    // Adiciona botão em tabelas
    containerElement.querySelectorAll('table').forEach((table, index) => {
      // Pula se já processado
      if (table.parentElement?.classList?.contains('table-block')) return;
      
      const rows = table.querySelectorAll('tr');
      
      // Cria wrapper com acessibilidade
      const wrapper = document.createElement('div');
      wrapper.className = 'table-block';
      wrapper.setAttribute('role', 'group');
      wrapper.setAttribute('aria-label', 'Tabela');
      table.parentNode.insertBefore(wrapper, table);
      wrapper.appendChild(table);
      
      // Container de botões
      const btnContainer = document.createElement('div');
      btnContainer.className = 'code-buttons';
      
      // Função auxiliar para extrair dados da tabela
      const getTableData = () => {
        const data = [];
        rows.forEach(row => {
          const cells = row.querySelectorAll('th, td');
          data.push(Array.from(cells).map(cell => cell.textContent.trim()));
        });
        return data;
      };
      
      // Botão Copiar (TSV)
      const copyBtn = document.createElement('button');
      copyBtn.className = 'copy-btn';
      copyBtn.textContent = 'Copiar';
      copyBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      copyBtn.setAttribute('aria-label', 'Copiar tabela');
      copyBtn.onclick = async () => {
        const data = getTableData();
        const text = data.map(row => row.join('\t')).join('\n');
        try {
          await navigator.clipboard.writeText(text);
          copyBtn.textContent = 'Copiado!';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        } catch (err) {
          copyBtn.textContent = 'Erro';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        }
      };
      btnContainer.appendChild(copyBtn);
      
      // Botão CSV - Download como arquivo
      const csvBtn = document.createElement('button');
      csvBtn.className = 'copy-btn';
      csvBtn.textContent = 'CSV';
      csvBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      csvBtn.setAttribute('aria-label', 'Baixar tabela como CSV');
      csvBtn.setAttribute('title', 'Baixar como CSV');
      csvBtn.onclick = () => {
        const data = getTableData();
        // Escapa campos com vírgulas, aspas ou quebras de linha
        const escapeCsv = (field) => {
          if (field.includes(',') || field.includes('"') || field.includes('\n')) {
            return '"' + field.replace(/"/g, '""') + '"';
          }
          return field;
        };
        const csv = data.map(row => row.map(escapeCsv).join(',')).join('\n');
        
        // Cria e baixa o arquivo
        const blob = new Blob(['\ufeff' + csv], { type: 'text/csv;charset=utf-8;' }); // BOM para Excel
        const url = URL.createObjectURL(blob);
        const link = document.createElement('a');
        link.href = url;
        link.download = `tabela-${Date.now()}.csv`;
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        URL.revokeObjectURL(url);
        
        csvBtn.textContent = '✓';
        setTimeout(() => { csvBtn.textContent = 'CSV'; }, 2000);
      };
      btnContainer.appendChild(csvBtn);
      
      // Botão Excel - Copia HTML para colar em Excel/Sheets
      const excelBtn = document.createElement('button');
      excelBtn.className = 'copy-btn';
      excelBtn.textContent = 'Excel';
      excelBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      excelBtn.setAttribute('aria-label', 'Copiar para Excel/Sheets');
      excelBtn.setAttribute('title', 'Copiar para colar em Excel ou Google Sheets');
      excelBtn.onclick = async () => {
        try {
          // Copia HTML da tabela para clipboard (Excel/Sheets entendem)
          const tableHtml = table.outerHTML;
          const blob = new Blob([tableHtml], { type: 'text/html' });
          
          // Também inclui texto plano como fallback
          const data = getTableData();
          const textPlain = data.map(row => row.join('\t')).join('\n');
          
          await navigator.clipboard.write([
            new ClipboardItem({
              'text/html': blob,
              'text/plain': new Blob([textPlain], { type: 'text/plain' })
            })
          ]);
          
          excelBtn.textContent = '✓';
          setTimeout(() => { excelBtn.textContent = 'Excel'; }, 2000);
        } catch (err) {
          // Fallback: copia só texto
          const data = getTableData();
          const text = data.map(row => row.join('\t')).join('\n');
          await navigator.clipboard.writeText(text);
          excelBtn.textContent = '✓';
          setTimeout(() => { excelBtn.textContent = 'Excel'; }, 2000);
        }
      };
      btnContainer.appendChild(excelBtn);
      
      // Botão "Ver código" só aparece quando interactiveButtons está ativo
      if (interactiveButtons) {
        // Gera Markdown da tabela
        const generateMarkdown = () => {
          let markdown = '';
          const currentRows = table.querySelectorAll('tr');
          currentRows.forEach((row, rowIdx) => {
            const cells = row.querySelectorAll('th, td');
            const rowText = Array.from(cells).map(cell => cell.textContent.trim()).join(' | ');
            markdown += '| ' + rowText + ' |\n';
            if (rowIdx === 0) {
              const separator = Array.from(cells).map(() => '---').join(' | ');
              markdown += '| ' + separator + ' |\n';
            }
          });
          return markdown.trim();
        };
        
        // Container para o Monaco (inicialmente oculto)
        const monacoContainer = document.createElement('div');
        monacoContainer.className = 'monaco-inline-container';
        monacoContainer.style.display = 'none';
        wrapper.insertBefore(monacoContainer, table);
        
        let editor = null;
        let isEditorMode = false;
        
        const toggleBtn = document.createElement('button');
        toggleBtn.className = 'copy-btn toggle-editor-btn';
        toggleBtn.textContent = 'Ver código';
        toggleBtn.setAttribute('tabindex', '0');
        toggleBtn.setAttribute('aria-label', 'Ver código Markdown da tabela');
        toggleBtn.onclick = async () => {
          isEditorMode = !isEditorMode;
          
          if (isEditorMode) {
            // Ativa modo Monaco
            table.style.display = 'none';
            monacoContainer.style.display = 'block';
            toggleBtn.textContent = 'Ver tabela';
            toggleBtn.setAttribute('aria-label', 'Voltar para visualização da tabela');
            
            if (!editor) {
              const monaco = await import('monaco-editor');
              editor = monaco.editor.create(monacoContainer, {
                value: generateMarkdown(),
                language: 'markdown',
                theme: 'vs-dark',
                readOnly: false,
                minimap: { enabled: false },
                lineNumbers: 'on',
                scrollBeyondLastLine: false,
                wordWrap: 'on',
                fontSize: 14,
                fontFamily: "'Fira Code', 'Consolas', monospace",
                padding: { top: 8, bottom: 8 },
                automaticLayout: true,
                // Acessibilidade para leitores de tela
                accessibilitySupport: 'on',
                ariaLabel: 'Editor de código Markdown da tabela. Use Ctrl+F1 para opções de acessibilidade.',
              });
              setTimeout(() => editor.focus(), 50);
            } else {
              editor.setValue(generateMarkdown());
              editor.focus();
            }
          } else {
            // Volta para visualização
            table.style.display = '';
            monacoContainer.style.display = 'none';
            toggleBtn.textContent = 'Ver código';
            toggleBtn.setAttribute('aria-label', 'Ver código Markdown da tabela');
          }
        };
        btnContainer.appendChild(toggleBtn);
      }
      
      wrapper.appendChild(btnContainer);
    });
  }
  
  // Renderiza diagramas Mermaid
  async function renderMermaid() {
    if (!containerElement) return;
    
    initMermaid();
    
    // Encontra blocos de código mermaid
    const mermaidBlocks = containerElement.querySelectorAll('code.language-mermaid');
    
    for (let i = 0; i < mermaidBlocks.length; i++) {
      const codeBlock = mermaidBlocks[i];
      const pre = codeBlock.parentElement;
      
      // Pula se já foi renderizado
      if (pre.dataset.mermaidRendered) continue;
      
      const mermaidCode = codeBlock.textContent;
      
      try {
        const id = `mermaid-${Date.now()}-${i}`;
        const { svg } = await mermaid.render(id, mermaidCode);
        
        // Cria container para o diagrama
        const diagramWrapper = document.createElement('div');
        diagramWrapper.className = 'mermaid-diagram';
        diagramWrapper.setAttribute('role', 'group');
        diagramWrapper.setAttribute('aria-label', 'Mermaid');
        diagramWrapper.innerHTML = svg;
        
        // Substitui o bloco de código pelo diagrama
        pre.parentNode.insertBefore(diagramWrapper, pre);
        pre.style.display = 'none';
        pre.dataset.mermaidRendered = 'true';
        
        // Container de botões
        const btnContainer = document.createElement('div');
        btnContainer.className = 'code-buttons';
        
        // Botão copiar
        const copyBtn = document.createElement('button');
        copyBtn.className = 'copy-btn';
        copyBtn.textContent = 'Copiar';
        copyBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
        copyBtn.setAttribute('aria-label', 'Copiar código Mermaid');
        copyBtn.onclick = async () => {
          try {
            await navigator.clipboard.writeText(mermaidCode);
            copyBtn.textContent = 'Copiado!';
            setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
          } catch (err) {
            copyBtn.textContent = 'Erro';
            setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
          }
        };
        btnContainer.appendChild(copyBtn);
        
        // Botão toggle para Monaco (só quando interactiveButtons)
        if (interactiveButtons) {
          // Container para o Monaco
          const monacoContainer = document.createElement('div');
          monacoContainer.className = 'monaco-inline-container';
          monacoContainer.style.display = 'none';
          
          // Insere o container do Monaco dentro do diagramWrapper, antes do SVG
          const svgElement = diagramWrapper.querySelector('svg');
          if (svgElement) {
            diagramWrapper.insertBefore(monacoContainer, svgElement);
          } else {
            diagramWrapper.appendChild(monacoContainer);
          }
          
          let editor = null;
          let isEditorMode = false;
          
          const toggleBtn = document.createElement('button');
          toggleBtn.className = 'copy-btn toggle-editor-btn';
          toggleBtn.textContent = 'Ver código';
          toggleBtn.setAttribute('tabindex', '0');
          toggleBtn.setAttribute('aria-label', 'Ver código Mermaid no editor');
          toggleBtn.onclick = async () => {
            isEditorMode = !isEditorMode;
            
            if (isEditorMode) {
              // Oculta SVG, mostra Monaco
              if (svgElement) svgElement.style.display = 'none';
              monacoContainer.style.display = 'block';
              toggleBtn.textContent = 'Ver diagrama';
              toggleBtn.setAttribute('aria-label', 'Voltar para visualização do diagrama');
              
              if (!editor) {
                const monaco = await import('monaco-editor');
                editor = monaco.editor.create(monacoContainer, {
                  value: mermaidCode,
                  language: 'markdown', // Mermaid não tem syntax oficial, usa markdown
                  theme: 'vs-dark',
                  readOnly: false,
                  minimap: { enabled: false },
                  lineNumbers: 'on',
                  scrollBeyondLastLine: false,
                  wordWrap: 'on',
                  fontSize: 14,
                  fontFamily: "'Fira Code', 'Consolas', monospace",
                  padding: { top: 8, bottom: 8 },
                  automaticLayout: true,
                  // Acessibilidade para leitores de tela
                  accessibilitySupport: 'on',
                  ariaLabel: 'Editor de código Mermaid. Use Ctrl+F1 para opções de acessibilidade.',
                });
                setTimeout(() => editor.focus(), 50);
              } else {
                editor.focus();
              }
            } else {
              // Mostra SVG, oculta Monaco
              if (svgElement) svgElement.style.display = '';
              monacoContainer.style.display = 'none';
              toggleBtn.textContent = 'Ver código';
              toggleBtn.setAttribute('aria-label', 'Ver código Mermaid no editor');
            }
          };
          btnContainer.appendChild(toggleBtn);
        }
        
        diagramWrapper.appendChild(btnContainer);
        
      } catch (err) {
        console.error('Erro ao renderizar Mermaid:', err);
        // Mantém o código fonte se houver erro
      }
    }
  }
  
  afterUpdate(() => {
    addCopyButtons();
    renderMermaid();
  });
</script>

<div class="markdown-content" bind:this={containerElement}>
  {@html html}
</div>

<style>
  .markdown-content {
    line-height: var(--line-height);
    word-wrap: break-word;
    overflow-wrap: break-word;
  }
  
  .markdown-content :global(p) {
    margin: 0 0 0.75em 0;
  }
  
  .markdown-content :global(p:last-child) {
    margin-bottom: 0;
  }
  
  .markdown-content :global(a) {
    color: var(--color-link);
    text-decoration: underline;
  }
  
  .markdown-content :global(a:hover) {
    text-decoration: none;
  }
  
  .markdown-content :global(code) {
    font-family: 'Fira Code', 'Consolas', 'Monaco', monospace;
    background-color: var(--color-bg-tertiary);
    padding: 0.15em 0.4em;
    border-radius: var(--border-radius);
    font-size: 0.9em;
  }
  
  .markdown-content :global(pre) {
    background-color: var(--color-bg-tertiary);
    padding: var(--spacing-md);
    border-radius: var(--border-radius);
    overflow-x: auto;
    margin: 0.75em 0;
  }
  
  .markdown-content :global(pre code) {
    background: none;
    padding: 0;
    font-size: 0.85em;
    line-height: 1.5;
  }
  
  .markdown-content :global(table) {
    width: 100%;
    border-collapse: collapse;
    margin: 0.75em 0;
  }
  
  .markdown-content :global(th),
  .markdown-content :global(td) {
    border: 1px solid var(--color-border);
    padding: var(--spacing-sm);
    text-align: left;
  }
  
  .markdown-content :global(th) {
    background-color: var(--color-bg-tertiary);
    font-weight: 600;
  }
  
  .markdown-content :global(tr:nth-child(even)) {
    background-color: var(--color-bg-secondary);
  }
  
  .markdown-content :global(ul),
  .markdown-content :global(ol) {
    margin: 0.5em 0;
    padding-left: 1.5em;
  }
  
  .markdown-content :global(li) {
    margin: 0.25em 0;
  }
  
  .markdown-content :global(blockquote) {
    border-left: 3px solid var(--color-accent);
    margin: 0.75em 0;
    padding-left: var(--spacing-md);
    color: var(--color-text-secondary);
  }
  
  .markdown-content :global(h1),
  .markdown-content :global(h2),
  .markdown-content :global(h3),
  .markdown-content :global(h4),
  .markdown-content :global(h5),
  .markdown-content :global(h6) {
    margin: 1em 0 0.5em 0;
    font-weight: 600;
    line-height: 1.3;
  }
  
  .markdown-content :global(h1) { font-size: 1.5em; }
  .markdown-content :global(h2) { font-size: 1.3em; }
  .markdown-content :global(h3) { font-size: 1.15em; }
  .markdown-content :global(h4) { font-size: 1em; }
  
  .markdown-content :global(hr) {
    border: none;
    border-top: 1px solid var(--color-border);
    margin: 1em 0;
  }
  
  .markdown-content :global(img) {
    max-width: 100%;
    height: auto;
    border-radius: var(--border-radius);
  }
  
  /* Estilo especial para código em mensagens do usuário (fundo mais claro) */
  :global(.message.user) .markdown-content :global(code),
  :global(.message.user) .markdown-content :global(pre) {
    background-color: rgba(255, 255, 255, 0.15);
  }
  
  :global(.message.user) .markdown-content :global(th),
  :global(.message.user) .markdown-content :global(tr:nth-child(even)) {
    background-color: rgba(255, 255, 255, 0.1);
  }
  
  /* Classe para esconder visualmente mas manter acessível */
  .markdown-content :global(.visually-hidden) {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
  
  
  /* Wrappers de código e tabela */
  .markdown-content :global(.code-block),
  .markdown-content :global(.table-block) {
    margin: 0.75em 0;
  }
  
  .markdown-content :global(.code-block pre),
  .markdown-content :global(.table-block table) {
    margin: 0;
  }
  
  /* Container de botões */
  .markdown-content :global(.code-buttons) {
    display: flex;
    gap: var(--spacing-xs);
    margin-top: var(--spacing-xs);
  }
  
  /* Botão de copiar */
  .markdown-content :global(.copy-btn) {
    flex: 1;
    padding: var(--spacing-xs) var(--spacing-sm);
    background-color: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    cursor: pointer;
    text-align: center;
    transition: background-color 0.2s, color 0.2s;
  }
  
  .markdown-content :global(.copy-btn:hover) {
    background-color: var(--color-accent);
    color: var(--color-bg-primary);
  }
  
  .markdown-content :global(.copy-btn:focus-visible) {
    outline: 2px solid var(--color-accent);
    outline-offset: 2px;
  }
  
  .markdown-content :global(.toggle-editor-btn) {
    background-color: var(--color-bg-secondary);
  }
  
  /* Container inline do Monaco Editor */
  .markdown-content :global(.monaco-inline-container) {
    height: 250px;
    border: 1px solid var(--color-border);
    border-radius: var(--border-radius);
    overflow: hidden;
  }
  
  /* Botão de copiar em mensagens do usuário */
  :global(.message.user) .markdown-content :global(.copy-btn) {
    background-color: rgba(255, 255, 255, 0.1);
    border-color: rgba(255, 255, 255, 0.2);
    color: rgba(255, 255, 255, 0.8);
  }
  
  :global(.message.user) .markdown-content :global(.copy-btn:hover) {
    background-color: rgba(255, 255, 255, 0.2);
    color: white;
  }
  
  /* Diagramas Mermaid */
  .markdown-content :global(.mermaid-diagram) {
    background-color: var(--color-bg-tertiary);
    border-radius: var(--border-radius);
    padding: var(--spacing-md);
    margin: 0.75em 0;
    overflow-x: auto;
  }
  
  .markdown-content :global(.mermaid-diagram svg) {
    max-width: 100%;
    height: auto;
  }
  
  .markdown-content :global(.mermaid-toggle) {
    margin-top: var(--spacing-sm);
  }
</style>

