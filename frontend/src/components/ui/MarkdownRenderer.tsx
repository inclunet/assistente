import { useEffect, useRef, useState } from 'react';

import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';
import mermaid from 'mermaid';
import * as monaco from 'monaco-editor';
import './MarkdownRenderer.css';

interface MarkdownRendererProps {
  content: string;
  className?: string;
  interactiveButtons?: boolean; // Se true, botões de copiar são focáveis e Monaco Editor é habilitado
}

// Configuração do markdown-it
const md = new MarkdownIt({
  html: false,        // Não permite HTML raw
  xhtmlOut: false,
  breaks: false,      // Não converte \n em <br> (interfere com tabelas)
  linkify: true,      // Converte URLs em links automaticamente
  typographer: true   // Substituições tipográficas
});

// Configuração do DOMPurify
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

// Hook DOMPurify para adicionar tabindex e abrir links em nova aba
DOMPurify.addHook('afterSanitizeAttributes', (node: Element) => {
  if (node.tagName === 'A') {
    node.setAttribute('tabindex', '-1');
    node.setAttribute('target', '_blank');
    node.setAttribute('rel', 'noopener noreferrer');
  }
});

let mermaidInitialized = false;

function initMermaid() {
  if (mermaidInitialized) return;
  mermaid.initialize({
    startOnLoad: false,
    theme: 'dark',
    securityLevel: 'strict', // 'strict' previne execução de código malicioso em diagramas
    fontFamily: 'inherit'
  });
  mermaidInitialized = true;
}

function renderMarkdown(text: string): string {
  if (!text) return '';
  
  let processed = text;
  
  // 1. Extrai tabelas de dentro de blocos de código markdown/md APENAS
  processed = processed.replace(/```(markdown|md)\s*\n([\s\S]*?)```/gi, (match, _lang, content) => {
    const trimmedContent = content.trim();
    const hasTableSyntax = trimmedContent.includes('|') && 
                           (trimmedContent.includes('|---') || trimmedContent.includes('| ---') || trimmedContent.includes('|:--'));
    if (hasTableSyntax) {
      return '\n' + trimmedContent + '\n';
    }
    return match;
  });
  
  // 2. Remove linhas vazias dentro de tabelas
  const lines = processed.split('\n');
  const result: string[] = [];
  let inTable = false;
  
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i].trim();
    const isTableLine = line.startsWith('|') && line.endsWith('|');
    const isEmpty = line === '';
    
    if (isTableLine) {
      inTable = true;
      result.push(lines[i]);
    } else if (isEmpty && inTable) {
      let nextNonEmpty = '';
      for (let j = i + 1; j < lines.length; j++) {
        if (lines[j].trim() !== '') {
          nextNonEmpty = lines[j].trim();
          break;
        }
      }
      if (nextNonEmpty.startsWith('|') && nextNonEmpty.endsWith('|')) {
        continue;
      } else {
        inTable = false;
        result.push(lines[i]);
      }
    } else {
      inTable = false;
      result.push(lines[i]);
    }
  }
  
  processed = result.join('\n');
  
  const parsed = md.render(processed);
  return DOMPurify.sanitize(parsed, purifyConfig);
}

export function MarkdownRenderer({ content, className = '', interactiveButtons = false }: MarkdownRendererProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [html, setHtml] = useState('');
  const editorsRef = useRef<Map<string, monaco.editor.IStandaloneCodeEditor>>(new Map());

  useEffect(() => {
    setHtml(renderMarkdown(content));
  }, [content]);

  useEffect(() => {
    if (!containerRef.current || !html) return;

    addCopyButtons();
    renderMermaidDiagrams();

    return () => {
      // Limpa editores Monaco quando o componente é desmontado
      editorsRef.current.forEach(editor => editor.dispose());
      editorsRef.current.clear();
    };
  }, [html, interactiveButtons]);

  function addCopyButtons() {
    if (!containerRef.current) return;

    // Remove botões existentes
    containerRef.current.querySelectorAll('.copy-btn').forEach(btn => btn.remove());

    // Adiciona botões em blocos de código
    containerRef.current.querySelectorAll('pre').forEach((pre: Element, index: number) => {
      const preEl = pre as HTMLPreElement;
      
      if (preEl.parentElement?.classList?.contains('code-block')) return;
      if (preEl.parentElement?.classList?.contains('mermaid-diagram')) return;

      const codeElement = preEl.querySelector('code');
      let language = 'Código';
      
      if (codeElement && codeElement.className) {
        const classMatch = codeElement.className.match(/language-(\w+)/i);
        if (classMatch) {
          const lang = classMatch[1].toLowerCase();
          if (lang === 'mermaid') return;
          language = classMatch[1].charAt(0).toUpperCase() + classMatch[1].slice(1);
        }
      }

      const wrapper = document.createElement('div');
      wrapper.className = 'code-block';
      wrapper.setAttribute('role', 'group');
      wrapper.setAttribute('aria-label', language);
      preEl.parentNode!.insertBefore(wrapper, preEl);
      wrapper.appendChild(preEl);

      const btnContainer = document.createElement('div');
      btnContainer.className = 'code-buttons';

      const copyBtn = document.createElement('button');
      copyBtn.className = 'copy-btn';
      copyBtn.textContent = 'Copiar';
      copyBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      copyBtn.setAttribute('aria-label', `Copiar código ${language}`);
      copyBtn.onclick = async () => {
        const code = preEl.textContent || '';
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

      if (interactiveButtons) {
        const code = preEl.textContent || '';
        const langLower = language.toLowerCase();
        
        const monacoContainer = document.createElement('div');
        monacoContainer.className = 'monaco-inline-container';
        monacoContainer.style.display = 'none';
        wrapper.insertBefore(monacoContainer, preEl);
        
        let editor: monaco.editor.IStandaloneCodeEditor | null = null;
        let isEditorMode = false;
        const editorKey = `code-${index}`;
        
        const toggleBtn = document.createElement('button');
        toggleBtn.className = 'copy-btn toggle-editor-btn';
        toggleBtn.textContent = 'Editar';
        toggleBtn.setAttribute('tabindex', '0');
        toggleBtn.setAttribute('aria-label', `Editar código ${language} no Monaco Editor`);
        toggleBtn.onclick = () => {
          isEditorMode = !isEditorMode;
          
          if (isEditorMode) {
            preEl.style.display = 'none';
            monacoContainer.style.display = 'block';
            toggleBtn.textContent = 'Visualizar';
            
            if (!editor) {
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
              });
              editorsRef.current.set(editorKey, editor);
              setTimeout(() => editor!.focus(), 50);
            } else {
              editor.focus();
            }
          } else {
            preEl.style.display = 'block';
            monacoContainer.style.display = 'none';
            toggleBtn.textContent = 'Editar';
          }
        };
        btnContainer.appendChild(toggleBtn);
      }

      wrapper.appendChild(btnContainer);
    });

    // Adiciona botões em tabelas
    containerRef.current.querySelectorAll('table').forEach((table: Element, index: number) => {
      const tableEl = table as HTMLTableElement;
      
      if (tableEl.parentElement?.classList?.contains('table-block')) return;

      const wrapper = document.createElement('div');
      wrapper.className = 'table-block';
      wrapper.setAttribute('role', 'group');
      wrapper.setAttribute('aria-label', 'Tabela');
      tableEl.parentNode!.insertBefore(wrapper, tableEl);
      wrapper.appendChild(tableEl);

      const btnContainer = document.createElement('div');
      btnContainer.className = 'code-buttons';

      const generateMarkdown = () => {
        const rows: string[] = [];
        tableEl.querySelectorAll('tr').forEach((tr: Element) => {
          const cells: string[] = [];
          tr.querySelectorAll('th, td').forEach((cell: Element) => {
            cells.push(cell.textContent || '');
          });
          rows.push('| ' + cells.join(' | ') + ' |');
        });
        
        if (rows.length > 1) {
          const separators = Array(tableEl.querySelectorAll('th').length).fill('---');
          rows.splice(1, 0, '| ' + separators.join(' | ') + ' |');
        }
        
        return rows.join('\n');
      };

      const copyBtn = document.createElement('button');
      copyBtn.className = 'copy-btn';
      copyBtn.textContent = 'Copiar';
      copyBtn.setAttribute('tabindex', interactiveButtons ? '0' : '-1');
      copyBtn.setAttribute('aria-label', 'Copiar Markdown da tabela');
      copyBtn.onclick = async () => {
        try {
          await navigator.clipboard.writeText(generateMarkdown());
          copyBtn.textContent = 'Copiado!';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        } catch (err) {
          copyBtn.textContent = 'Erro';
          setTimeout(() => { copyBtn.textContent = 'Copiar'; }, 2000);
        }
      };
      btnContainer.appendChild(copyBtn);

      if (interactiveButtons) {
        const monacoContainer = document.createElement('div');
        monacoContainer.className = 'monaco-inline-container';
        monacoContainer.style.display = 'none';
        wrapper.insertBefore(monacoContainer, tableEl);
        
        let editor: monaco.editor.IStandaloneCodeEditor | null = null;
        let isEditorMode = false;
        const editorKey = `table-${index}`;
        
        const toggleBtn = document.createElement('button');
        toggleBtn.className = 'copy-btn toggle-editor-btn';
        toggleBtn.textContent = 'Ver código';
        toggleBtn.setAttribute('tabindex', '0');
        toggleBtn.setAttribute('aria-label', 'Ver código Markdown da tabela');
        toggleBtn.onclick = () => {
          isEditorMode = !isEditorMode;
          
          if (isEditorMode) {
            tableEl.style.display = 'none';
            monacoContainer.style.display = 'block';
            toggleBtn.textContent = 'Ver tabela';
            
            if (!editor) {
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
              });
              editorsRef.current.set(editorKey, editor);
              setTimeout(() => editor!.focus(), 50);
            } else {
              editor.setValue(generateMarkdown());
              editor.focus();
            }
          } else {
            tableEl.style.display = '';
            monacoContainer.style.display = 'none';
            toggleBtn.textContent = 'Ver código';
          }
        };
        btnContainer.appendChild(toggleBtn);
      }

      wrapper.appendChild(btnContainer);
    });
  }

  async function renderMermaidDiagrams() {
    if (!containerRef.current) return;

    initMermaid();

    const mermaidBlocks = containerRef.current.querySelectorAll('code.language-mermaid');

    for (let i = 0; i < mermaidBlocks.length; i++) {
      const codeBlock = mermaidBlocks[i] as HTMLElement;
      const pre = codeBlock.parentElement as HTMLPreElement;

      if (pre.dataset.mermaidRendered) continue;

      const mermaidCode = codeBlock.textContent || '';

      try {
        const id = `mermaid-${Date.now()}-${i}`;
        const { svg } = await mermaid.render(id, mermaidCode);

        const diagramWrapper = document.createElement('div');
        diagramWrapper.className = 'mermaid-diagram';
        diagramWrapper.setAttribute('role', 'group');
        diagramWrapper.setAttribute('aria-label', 'Diagrama Mermaid');
        diagramWrapper.innerHTML = svg;

        pre.parentNode!.insertBefore(diagramWrapper, pre);
        pre.style.display = 'none';
        pre.dataset.mermaidRendered = 'true';

        const btnContainer = document.createElement('div');
        btnContainer.className = 'code-buttons';

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

        if (interactiveButtons) {
          const monacoContainer = document.createElement('div');
          monacoContainer.className = 'monaco-inline-container';
          monacoContainer.style.display = 'none';

          const svgElement = diagramWrapper.querySelector('svg');
          if (svgElement) {
            diagramWrapper.insertBefore(monacoContainer, svgElement);
          } else {
            diagramWrapper.appendChild(monacoContainer);
          }

          let editor: monaco.editor.IStandaloneCodeEditor | null = null;
          let isEditorMode = false;
          const editorKey = `mermaid-${i}`;

          const toggleBtn = document.createElement('button');
          toggleBtn.className = 'copy-btn toggle-editor-btn';
          toggleBtn.textContent = 'Ver código';
          toggleBtn.setAttribute('tabindex', '0');
          toggleBtn.setAttribute('aria-label', 'Ver código Mermaid no editor');
          toggleBtn.onclick = () => {
            isEditorMode = !isEditorMode;

            if (isEditorMode) {
              if (svgElement) svgElement.style.display = 'none';
              monacoContainer.style.display = 'block';
              toggleBtn.textContent = 'Ver diagrama';

              if (!editor) {
                editor = monaco.editor.create(monacoContainer, {
                  value: mermaidCode,
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
                });
                editorsRef.current.set(editorKey, editor);
                setTimeout(() => editor!.focus(), 50);
              } else {
                editor.focus();
              }
            } else {
              if (svgElement) svgElement.style.display = '';
              monacoContainer.style.display = 'none';
              toggleBtn.textContent = 'Ver código';
            }
          };
          btnContainer.appendChild(toggleBtn);
        }

        diagramWrapper.appendChild(btnContainer);
      } catch (err) {
        console.error('Erro ao renderizar Mermaid:', err);
      }
    }
  }

  return (
    <div 
      ref={containerRef}
      className={`markdown-content ${className}`}
      dangerouslySetInnerHTML={{ __html: html }}
    />
  );
}
