import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useNavigate } from 'react-router-dom';
import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';
import type { editor as MonacoEditorNamespace } from 'monaco-editor';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { loadMonacoLanguage } from '../../lib/monacoLanguageLoader';
import { markdownItDeepLink } from '../../lib/markdownItDeepLink';
import { isDeepLink, parseDeepLink, executeDeepLink } from '../../lib/deepLinks';
import './MarkdownRenderer.css';

type MermaidModule = typeof import('mermaid');
type MermaidApi = MermaidModule['default'];
type MonacoModule = typeof import('monaco-editor');
type MonacoEditor = MonacoEditorNamespace.IStandaloneCodeEditor;

interface MarkdownRendererProps {
  content: string;
  className?: string;
  interactiveButtons?: boolean;
  focusableMermaid?: boolean;
  enableSendToEditorButtons?: boolean;
  onSendToEditor?: (payload: {
    target: 'current' | 'new_tab';
    format: 'markdown' | 'html' | 'plain';
    title?: string;
    content: string;
  }) => void;
}

const md = new MarkdownIt({
  html: false,
  xhtmlOut: false,
  breaks: false,
  linkify: true,
  typographer: true,
});

md.use(markdownItDeepLink);

const purifyConfig = {
  ALLOWED_TAGS: [
    'p',
    'br',
    'strong',
    'em',
    'b',
    'i',
    'u',
    's',
    'del',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'ul',
    'ol',
    'li',
    'a',
    'img',
    'pre',
    'code',
    'blockquote',
    'table',
    'thead',
    'tbody',
    'tr',
    'th',
    'td',
    'hr',
    'div',
    'span',
  ],
  ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'class', 'target', 'rel', 'tabindex', 'data-deep-link', 'role', 'aria-label'],
  ALLOWED_URI_REGEXP: /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|assistente):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
};

DOMPurify.addHook('afterSanitizeAttributes', (node: Element) => {
  if (node.tagName === 'A') {
    const href = node.getAttribute('href') || '';
    if (isDeepLink(href)) {
      node.setAttribute('tabindex', '0');
      node.removeAttribute('target');
      node.removeAttribute('rel');
    } else {
      node.setAttribute('tabindex', '-1');
      node.setAttribute('target', '_blank');
      node.setAttribute('rel', 'noopener noreferrer');
    }
  }
});

export function MarkdownRenderer({
  content,
  className = '',
  interactiveButtons = false,
  focusableMermaid: _focusableMermaid = false,
  enableSendToEditorButtons = false,
  onSendToEditor,
}: MarkdownRendererProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const mermaidInitializedRef = useRef(false);
  const editorsRef = useRef<Map<string, MonacoEditor>>(new Map());
  const mermaidApiRef = useRef<MermaidApi | null>(null);
  const monacoApiRef = useRef<MonacoModule | null>(null);
  const navigate = useNavigate();

  const canSendToEditor = Boolean(enableSendToEditorButtons && onSendToEditor);

  const {
    menu: menuState,
    openAtPoint: openMenuAtPoint,
    closeMenu,
    onSelectItem: onSelectMenuItem,
  } = useAnchoredContextMenu();

  const openMenu = useCallback(
    (x: number, y: number, ariaLabel: string, items: MenuItem[]) => {
      openMenuAtPoint(x, y, ariaLabel, items);
    },
    [openMenuAtPoint]
  );

  const copyToClipboard = useCallback(async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
    } catch {
      // best-effort
    }
  }, []);

  const fencedCode = useCallback((language: string, code: string) => {
    const lang = String(language || '').trim();
    const body = String(code || '').replace(/\n$/, '');
    return `\n\`\`\`${lang}\n${body}\n\`\`\`\n`;
  }, []);

  const generateTableMarkdown = useCallback((tableEl: HTMLTableElement) => {
    const rows: string[] = [];
    tableEl.querySelectorAll('tr').forEach((tr: Element) => {
      const cells: string[] = [];
      tr.querySelectorAll('th, td').forEach((cell: Element) => {
        cells.push((cell.textContent || '').trim());
      });
      rows.push('| ' + cells.join(' | ') + ' |');
    });

    if (rows.length > 1) {
      const thCount = tableEl.querySelectorAll('th').length;
      const separators = Array(thCount > 0 ? thCount : Math.max(1, rows[0].split('|').length - 2)).fill('---');
      rows.splice(1, 0, '| ' + separators.join(' | ') + ' |');
    }

    return rows.join('\n');
  }, []);

  const loadMonaco = useCallback(async (): Promise<MonacoModule> => {
    if (monacoApiRef.current) return monacoApiRef.current;
    const mod = await import('monaco-editor');
    monacoApiRef.current = mod;
    return mod;
  }, []);

  const ensureMonacoEditor = useCallback(
    async (key: string, container: HTMLDivElement, initialValue: string, language: string) => {
      const existing = editorsRef.current.get(key);
      if (existing) {
        existing.setValue(initialValue);
        return existing;
      }

      await loadMonacoLanguage(language);
      const monaco = await loadMonaco();
      const editor = monaco.editor.create(container, {
        value: initialValue,
        language: language || 'plaintext',
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

      editorsRef.current.set(key, editor);
      return editor;
    },
    [loadMonaco]
  );

  const initMermaid = useCallback(async (): Promise<MermaidApi> => {
    if (mermaidInitializedRef.current && mermaidApiRef.current) return mermaidApiRef.current;
    const mod = await import('mermaid');
    const api = (mod.default ?? mod) as MermaidApi;
    api.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'strict' });
    mermaidApiRef.current = api;
    mermaidInitializedRef.current = true;
    return api;
  }, []);

  const addContextMenus = useCallback(
    (cleanups: Array<() => void>) => {
      if (!containerRef.current) return;
      const root = containerRef.current;

      root.querySelectorAll('pre').forEach((pre: Element, index: number) => {
        const preEl = pre as HTMLPreElement;
        if (preEl.parentElement?.classList?.contains('code-block')) return;
        if (preEl.parentElement?.classList?.contains('mermaid-diagram')) return;

        const codeEl = preEl.querySelector('code');
        const classMatch = codeEl?.className?.match(/language-([^\s]+)/i);
        const lang = classMatch ? classMatch[1].toLowerCase() : '';
        if (lang === 'mermaid') return;

        const languageLabel = classMatch
          ? classMatch[1].charAt(0).toUpperCase() + classMatch[1].slice(1)
          : 'Código';

        const wrapper = document.createElement('div');
        wrapper.className = 'code-block';
        wrapper.setAttribute('role', 'group');
        wrapper.setAttribute('aria-label', languageLabel);
        wrapper.setAttribute('tabindex', '-1');
        preEl.parentNode!.insertBefore(wrapper, preEl);
        wrapper.appendChild(preEl);

        const editorKey = interactiveButtons ? `code-${index}` : null;
        const monacoContainer = interactiveButtons
          ? (() => {
              const el = document.createElement('div');
              el.className = 'monaco-inline-container';
              el.style.display = 'none';
              el.setAttribute('tabindex', '-1');
              wrapper.insertBefore(el, preEl);
              return el;
            })()
          : null;
        let isEditorMode = false;

        const onContextMenu = (e: MouseEvent) => {
          e.preventDefault();
          e.stopPropagation();

          const codeText = preEl.textContent || '';
          const items: MenuItem[] = [
            {
              id: `code-${index}-copy`,
              label: 'Copiar',
              ariaLabel: `Copiar código ${languageLabel}`,
              action: () => void copyToClipboard(codeText),
            },
          ];

          if (canSendToEditor) {
            items.push({
              id: `code-${index}-send`,
              label: 'Enviar ao editor',
              submenu: [
                {
                  id: `code-${index}-send-current`,
                  label: 'Inserir no cursor',
                  action: () =>
                    onSendToEditor?.({
                      target: 'current',
                      format: 'markdown',
                      title: `Código ${languageLabel}`,
                      content: fencedCode(lang || 'plaintext', codeText),
                    }),
                },
                {
                  id: `code-${index}-send-new`,
                  label: 'Novo documento',
                  action: () =>
                    onSendToEditor?.({
                      target: 'new_tab',
                      format: 'markdown',
                      title: `Código ${languageLabel}`,
                      content: fencedCode(lang || 'plaintext', codeText),
                    }),
                },
              ],
            });
          }

          if (interactiveButtons && monacoContainer && editorKey) {
            items.push({ separator: true, id: `code-${index}-sep-1` });
            items.push({
              id: `code-${index}-toggle`,
              label: isEditorMode ? 'Visualizar' : 'Editar',
              action: () => {
                isEditorMode = !isEditorMode;
                if (isEditorMode) {
                  preEl.style.display = 'none';
                  monacoContainer.style.display = 'block';
                  void ensureMonacoEditor(
                    editorKey,
                    monacoContainer,
                    codeText,
                    lang || 'plaintext'
                  ).then((editor) => {
                    setTimeout(() => editor.focus(), 30);
                  });
                } else {
                  preEl.style.display = '';
                  monacoContainer.style.display = 'none';
                }
              },
            });
          }

          openMenu(e.clientX, e.clientY, `Ações: ${languageLabel}`, items);
        };

        wrapper.addEventListener('contextmenu', onContextMenu);
        cleanups.push(() => wrapper.removeEventListener('contextmenu', onContextMenu));
      });

      root.querySelectorAll('table').forEach((table: Element, index: number) => {
        const tableEl = table as HTMLTableElement;
        if (tableEl.parentElement?.classList?.contains('table-block')) return;

        const wrapper = document.createElement('div');
        wrapper.className = 'table-block';
        wrapper.setAttribute('role', 'group');
        wrapper.setAttribute('aria-label', 'Tabela');
        wrapper.setAttribute('tabindex', '-1');
        tableEl.parentNode!.insertBefore(wrapper, tableEl);
        wrapper.appendChild(tableEl);

        const editorKey = interactiveButtons ? `table-${index}` : null;
        const monacoContainer = interactiveButtons
          ? (() => {
              const el = document.createElement('div');
              el.className = 'monaco-inline-container';
              el.style.display = 'none';
              el.setAttribute('tabindex', '-1');
              wrapper.insertBefore(el, tableEl);
              return el;
            })()
          : null;
        let isEditorMode = false;

        const onContextMenu = (e: MouseEvent) => {
          e.preventDefault();
          e.stopPropagation();

          const mdTable = generateTableMarkdown(tableEl);
          const items: MenuItem[] = [
            {
              id: `table-${index}-copy`,
              label: 'Copiar (Markdown)',
              action: () => void copyToClipboard(mdTable),
            },
          ];

          if (canSendToEditor) {
            items.push({
              id: `table-${index}-send`,
              label: 'Enviar ao editor',
              submenu: [
                {
                  id: `table-${index}-send-current`,
                  label: 'Inserir no cursor',
                  action: () =>
                    onSendToEditor?.({
                      target: 'current',
                      format: 'markdown',
                      title: 'Tabela',
                      content: `\n${mdTable}\n`,
                    }),
                },
                {
                  id: `table-${index}-send-new`,
                  label: 'Novo documento',
                  action: () =>
                    onSendToEditor?.({
                      target: 'new_tab',
                      format: 'markdown',
                      title: 'Tabela',
                      content: `\n${mdTable}\n`,
                    }),
                },
              ],
            });
          }

          if (interactiveButtons && monacoContainer && editorKey) {
            items.push({ separator: true, id: `table-${index}-sep-1` });
            items.push({
              id: `table-${index}-toggle`,
              label: isEditorMode ? 'Ver tabela' : 'Ver código',
              action: () => {
                isEditorMode = !isEditorMode;
                if (isEditorMode) {
                  tableEl.style.display = 'none';
                  monacoContainer.style.display = 'block';
                  void ensureMonacoEditor(editorKey, monacoContainer, mdTable, 'markdown').then(
                    (editor) => {
                      setTimeout(() => editor.focus(), 30);
                    }
                  );
                } else {
                  tableEl.style.display = '';
                  monacoContainer.style.display = 'none';
                }
              },
            });
          }

          openMenu(e.clientX, e.clientY, 'Ações: Tabela', items);
        };

        wrapper.addEventListener('contextmenu', onContextMenu);
        cleanups.push(() => wrapper.removeEventListener('contextmenu', onContextMenu));
      });

      root.querySelectorAll('a').forEach((anchor: Element, index: number) => {
        const a = anchor as HTMLAnchorElement;
        const href = a.getAttribute('href') || '';
        if (!href) return;
        const text = (a.textContent || '').trim() || href;
        const mdLink = `[${text}](${href})`;

        const onContextMenu = (e: MouseEvent) => {
          e.preventDefault();
          e.stopPropagation();

          const items: MenuItem[] = [
            {
              id: `link-${index}-copy-url`,
              label: 'Copiar URL',
              action: () => void copyToClipboard(href),
            },
            {
              id: `link-${index}-copy-md`,
              label: 'Copiar link (Markdown)',
              action: () => void copyToClipboard(mdLink),
            },
          ];

          if (canSendToEditor) {
            items.push({
              id: `link-${index}-send`,
              label: 'Enviar ao editor',
              submenu: [
                {
                  id: `link-${index}-send-current`,
                  label: 'Inserir no cursor',
                  action: () =>
                    onSendToEditor?.({
                      target: 'current',
                      format: 'markdown',
                      title: 'Link',
                      content: mdLink,
                    }),
                },
                {
                  id: `link-${index}-send-new`,
                  label: 'Novo documento',
                  action: () =>
                    onSendToEditor?.({
                      target: 'new_tab',
                      format: 'markdown',
                      title: 'Link',
                      content: mdLink,
                    }),
                },
              ],
            });
          }

          openMenu(e.clientX, e.clientY, 'Ações: Link', items);
        };

        a.addEventListener('contextmenu', onContextMenu);
        cleanups.push(() => a.removeEventListener('contextmenu', onContextMenu));
      });
    },
    [
      canSendToEditor,
      copyToClipboard,
      ensureMonacoEditor,
      fencedCode,
      generateTableMarkdown,
      interactiveButtons,
      onSendToEditor,
      openMenu,
    ]
  );

  const renderMermaidDiagrams = useCallback(
    async (cleanups: Array<() => void>) => {
      if (!containerRef.current) return;
      const mermaidBlocks = containerRef.current.querySelectorAll('code.language-mermaid');
      if (mermaidBlocks.length === 0) return;

      const mermaid = await initMermaid();
      if (!mermaid) return;

      const getErrorText = (err: unknown) => {
        if (!err) return 'Erro desconhecido';
        if (err instanceof Error) return String(err.stack || err.message || 'Erro');
        try {
          return typeof err === 'string' ? err : JSON.stringify(err, null, 2);
        } catch {
          return String(err);
        }
      };

      const truncate = (text: string, maxChars = 8000) => {
        const s = String(text || '');
        if (s.length <= maxChars) return s;
        return s.slice(0, maxChars) + `\n… (truncado; ${s.length} chars)`;
      };

      for (let i = 0; i < mermaidBlocks.length; i++) {
        const codeBlock = mermaidBlocks[i] as HTMLElement;
        const pre = codeBlock.parentElement as HTMLPreElement;
        if (!pre || pre.dataset.mermaidRendered) continue;

        const mermaidCode = codeBlock.textContent || '';

        try {
          const id = `mermaid-${Date.now()}-${i}`;
          const { svg } = await mermaid.render(id, mermaidCode);

          const diagramWrapper = document.createElement('div');
          diagramWrapper.className = 'mermaid-diagram';
          diagramWrapper.setAttribute('role', 'group');
          diagramWrapper.setAttribute('aria-label', 'Diagrama Mermaid');
          diagramWrapper.dataset.mermaidIndex = String(i);
          diagramWrapper.dataset.mermaidCode = mermaidCode;
          diagramWrapper.tabIndex = -1;
          diagramWrapper.innerHTML = svg;

          pre.parentNode!.insertBefore(diagramWrapper, pre);
          pre.style.display = 'none';
          pre.dataset.mermaidRendered = 'true';

          const svgElement = diagramWrapper.querySelector('svg') as SVGElement | null;
          const editorKey = interactiveButtons ? `mermaid-${i}` : null;
          const monacoContainer = interactiveButtons
            ? (() => {
                const el = document.createElement('div');
                el.className = 'monaco-inline-container';
                el.style.display = 'none';
                el.setAttribute('tabindex', '-1');
                if (svgElement) diagramWrapper.insertBefore(el, svgElement);
                else diagramWrapper.appendChild(el);
                return el;
              })()
            : null;
          let isEditorMode = false;

          const onContextMenu = (e: MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();

            const items: MenuItem[] = [
              {
                id: `mermaid-${i}-copy`,
                label: 'Copiar código',
                action: () => void copyToClipboard(mermaidCode),
              },
            ];

            if (canSendToEditor) {
              items.push({
                id: `mermaid-${i}-send`,
                label: 'Enviar ao editor',
                submenu: [
                  {
                    id: `mermaid-${i}-send-current`,
                    label: 'Inserir no cursor',
                    action: () =>
                      onSendToEditor?.({
                        target: 'current',
                        format: 'markdown',
                        title: 'Mermaid',
                        content: fencedCode('mermaid', mermaidCode),
                      }),
                  },
                  {
                    id: `mermaid-${i}-send-new`,
                    label: 'Novo documento',
                    action: () =>
                      onSendToEditor?.({
                        target: 'new_tab',
                        format: 'markdown',
                        title: 'Mermaid',
                        content: fencedCode('mermaid', mermaidCode),
                      }),
                  },
                ],
              });
            }

            if (interactiveButtons && monacoContainer && editorKey) {
              items.push({ separator: true, id: `mermaid-${i}-sep-1` });
              items.push({
                id: `mermaid-${i}-toggle`,
                label: isEditorMode ? 'Ver diagrama' : 'Ver código',
                action: () => {
                  isEditorMode = !isEditorMode;
                  if (isEditorMode) {
                    if (svgElement) svgElement.style.display = 'none';
                    monacoContainer.style.display = 'block';
                    void ensureMonacoEditor(
                      editorKey,
                      monacoContainer,
                      mermaidCode,
                      'plaintext'
                    ).then((editor) => {
                      setTimeout(() => editor.focus(), 30);
                    });
                  } else {
                    if (svgElement) svgElement.style.display = '';
                    monacoContainer.style.display = 'none';
                  }
                },
              });
            }

            openMenu(e.clientX, e.clientY, 'Ações: Mermaid', items);
          };

          diagramWrapper.addEventListener('contextmenu', onContextMenu);
          cleanups.push(() => diagramWrapper.removeEventListener('contextmenu', onContextMenu));
        } catch (err) {
          console.error('Erro ao renderizar Mermaid:', err);
          const errorText = truncate(getErrorText(err));

          const diagramWrapper = document.createElement('div');
          diagramWrapper.className = 'mermaid-diagram mermaid-diagram--error';
          diagramWrapper.setAttribute('role', 'group');
          diagramWrapper.setAttribute('aria-label', 'Diagrama Mermaid (erro)');
          diagramWrapper.dataset.mermaidIndex = String(i);
          diagramWrapper.dataset.mermaidCode = mermaidCode;
          diagramWrapper.tabIndex = -1;

          const titleEl = document.createElement('div');
          titleEl.className = 'mermaid-diagram__error-title';
          titleEl.textContent = 'Erro ao renderizar Mermaid';

          const msgEl = document.createElement('div');
          msgEl.className = 'mermaid-diagram__error-message';
          msgEl.textContent = 'O preview não pôde ser gerado. Você ainda pode copiar/enviar o código.';

          const preEl = document.createElement('pre');
          preEl.className = 'mermaid-diagram__error-pre';
          preEl.textContent = errorText;

          diagramWrapper.appendChild(titleEl);
          diagramWrapper.appendChild(msgEl);
          diagramWrapper.appendChild(preEl);

          const editorKey = interactiveButtons ? `mermaid-${i}` : null;
          const monacoContainer = interactiveButtons
            ? (() => {
                const el = document.createElement('div');
                el.className = 'monaco-inline-container';
                el.style.display = 'none';
                el.setAttribute('tabindex', '-1');
                diagramWrapper.appendChild(el);
                return el;
              })()
            : null;
          let isEditorMode = false;

          const onContextMenu = (e: MouseEvent) => {
            e.preventDefault();
            e.stopPropagation();

            const items: MenuItem[] = [
              {
                id: `mermaid-${i}-err-copy-code`,
                label: 'Copiar código',
                action: () => void copyToClipboard(mermaidCode),
              },
              {
                id: `mermaid-${i}-err-copy-error`,
                label: 'Copiar erro',
                action: () => void copyToClipboard(errorText),
              },
              {
                id: `mermaid-${i}-err-rerender`,
                label: 'Re-renderizar',
                action: () => {
                  pre.style.display = '';
                  pre.removeAttribute('data-mermaid-rendered');
                  diagramWrapper.remove();
                  void renderMermaidDiagrams(cleanups);
                },
              },
            ];

            if (canSendToEditor) {
              items.push({ separator: true, id: `mermaid-${i}-err-sep-1` });
              items.push({
                id: `mermaid-${i}-err-send`,
                label: 'Enviar ao editor',
                submenu: [
                  {
                    id: `mermaid-${i}-err-send-current`,
                    label: 'Inserir no cursor',
                    action: () =>
                      onSendToEditor?.({
                        target: 'current',
                        format: 'markdown',
                        title: 'Mermaid',
                        content: fencedCode('mermaid', mermaidCode),
                      }),
                  },
                  {
                    id: `mermaid-${i}-err-send-new`,
                    label: 'Novo documento',
                    action: () =>
                      onSendToEditor?.({
                        target: 'new_tab',
                        format: 'markdown',
                        title: 'Mermaid',
                        content: fencedCode('mermaid', mermaidCode),
                      }),
                  },
                ],
              });
            }

            if (interactiveButtons && monacoContainer && editorKey) {
              items.push({ separator: true, id: `mermaid-${i}-err-sep-2` });
              items.push({
                id: `mermaid-${i}-err-toggle`,
                label: isEditorMode ? 'Ocultar editor' : 'Abrir no editor',
                action: () => {
                  isEditorMode = !isEditorMode;
                  if (isEditorMode) {
                    preEl.style.display = 'none';
                    monacoContainer.style.display = 'block';
                    void ensureMonacoEditor(
                      editorKey,
                      monacoContainer,
                      mermaidCode,
                      'plaintext'
                    ).then((editor) => {
                      setTimeout(() => editor.focus(), 30);
                    });
                  } else {
                    preEl.style.display = '';
                    monacoContainer.style.display = 'none';
                  }
                },
              });
            }

            openMenu(e.clientX, e.clientY, 'Ações: Mermaid (erro)', items);
          };

          diagramWrapper.addEventListener('contextmenu', onContextMenu);
          cleanups.push(() => diagramWrapper.removeEventListener('contextmenu', onContextMenu));

          pre.parentNode!.insertBefore(diagramWrapper, pre);
          pre.style.display = 'none';
          pre.dataset.mermaidRendered = 'true';
        }
      }
    },
    [
      canSendToEditor,
      copyToClipboard,
      ensureMonacoEditor,
      fencedCode,
      initMermaid,
      interactiveButtons,
      onSendToEditor,
      openMenu,
    ]
  );

  const handleDeepLinkClick = useCallback(
    (e: MouseEvent) => {
      const target = (e.target as HTMLElement).closest('a.deep-link') as HTMLAnchorElement | null;
      if (!target) return;

      const uri = target.getAttribute('data-deep-link') || target.getAttribute('href') || '';
      const action = parseDeepLink(uri);
      if (!action) return;

      e.preventDefault();
      e.stopPropagation();
      void executeDeepLink(action, { navigate });
    },
    [navigate],
  );

  const handleDeepLinkKeydown = useCallback(
    (e: KeyboardEvent) => {
      if (e.key !== 'Enter' && e.key !== ' ') return;

      const target = e.target as HTMLElement;
      if (!target.classList.contains('deep-link')) return;

      const uri = target.getAttribute('data-deep-link') || (target as HTMLAnchorElement).href || '';
      const action = parseDeepLink(uri);
      if (!action) return;

      e.preventDefault();
      e.stopPropagation();
      void executeDeepLink(action, { navigate });
    },
    [navigate],
  );

  const html = useMemo(() => {
    const rendered = md.render(content || '');
    return DOMPurify.sanitize(rendered, purifyConfig);
  }, [content]);

  useEffect(() => {
    const cleanups: Array<() => void> = [];
    closeMenu();

    if (containerRef.current) {
      containerRef.current
        .querySelectorAll('.code-buttons, .send-to-editor-link, .send-to-editor-inline')
        .forEach((el) => el.remove());
    }

    addContextMenus(cleanups);
    void renderMermaidDiagrams(cleanups);

    // Deep link click/keyboard handlers (delegated)
    const container = containerRef.current;
    if (container) {
      container.addEventListener('click', handleDeepLinkClick);
      container.addEventListener('keydown', handleDeepLinkKeydown);
      cleanups.push(() => {
        container.removeEventListener('click', handleDeepLinkClick);
        container.removeEventListener('keydown', handleDeepLinkKeydown);
      });
    }

    return () => {
      cleanups.forEach((fn) => fn());
      editorsRef.current.forEach((ed) => {
        try {
          ed.dispose();
        } catch {
          // ignore
        }
      });
      editorsRef.current.clear();
    };
  }, [addContextMenus, closeMenu, handleDeepLinkClick, handleDeepLinkKeydown, html, renderMermaidDiagrams]);

  return (
    <>
      <div
        ref={containerRef}
        className={`markdown-content ${className}`}
        dangerouslySetInnerHTML={{ __html: html }}
      />
      <Menu
        items={menuState.items}
        x={menuState.x}
        y={menuState.y}
        visible={menuState.visible}
        ariaLabel={menuState.ariaLabel}
        onClose={closeMenu}
        onSelect={onSelectMenuItem}
      />
    </>
  );
}

export default MarkdownRenderer;

