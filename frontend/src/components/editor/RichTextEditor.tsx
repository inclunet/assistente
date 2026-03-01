import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react';
import StarterKit from '@tiptap/starter-kit';
import CodeBlock from '@tiptap/extension-code-block';
import Placeholder from '@tiptap/extension-placeholder';
import { Markdown } from 'tiptap-markdown';

import { Toolbar } from '../ui/Toolbar';
import { ContextMenu, MenuItem } from '../ui/ContextMenu';
import { MermaidCodeBlockNodeView } from './MermaidCodeBlockNodeView';

import './RichTextEditor.css';

export interface RichTextEditorProps {
  markdown: string;
  onMarkdownChange: (markdown: string) => void;
  readOnly?: boolean;
  placeholder?: string;
  ariaLabel?: string;
  onEditorReady?: (editor: any | null) => void;
  onRequestEditMermaid?: (ctx: {
    code: string;
    apply: (nextCode: string) => void;
    remove: () => void;
  }) => void;
}

export function RichTextEditor({
  markdown,
  onMarkdownChange,
  readOnly = false,
  placeholder = 'Escreva…',
  ariaLabel = 'Editor de texto rico',
  onEditorReady,
  onRequestEditMermaid,
}: RichTextEditorProps) {
  const headingLevels = useMemo(() => [1, 2, 3, 4, 5, 6] as const, []);

  const isApplyingExternalMarkdownRef = useRef(false);
  const lastMarkdownRef = useRef<string>(markdown);

  const menuTriggerRef = useRef<HTMLElement | null>(null);
  const [contextMenu, setContextMenu] = useState<{
    visible: boolean;
    x: number;
    y: number;
    items: MenuItem[];
    ariaLabel: string;
  }>({
    visible: false,
    x: 0,
    y: 0,
    items: [],
    ariaLabel: '',
  });

  const extensions = useMemo(() => {
    const MermaidAwareCodeBlock = CodeBlock.extend({
      addOptions() {
        return {
          ...(this.parent?.() as any),
          onRequestEditMermaid: undefined,
        };
      },
      addNodeView() {
        return ReactNodeViewRenderer(MermaidCodeBlockNodeView);
      },
    });

    return [
      StarterKit.configure({
        codeBlock: false,
      }),
      MermaidAwareCodeBlock.configure({
        languageClassPrefix: 'language-',
        defaultLanguage: null,
        onRequestEditMermaid,
      } as any),
      Placeholder.configure({
        placeholder,
      }),
      Markdown.configure({
        html: false,
        transformPastedText: true,
      }),
    ];
  }, [placeholder, onRequestEditMermaid]);

  const editor = useEditor({
    extensions,
    content: markdown,
    editable: !readOnly,
    onUpdate: ({ editor }) => {
      if (isApplyingExternalMarkdownRef.current) return;
      const md = (editor.storage as any)?.markdown?.getMarkdown?.() as string | undefined;
      const next = typeof md === 'string' ? md : '';
      lastMarkdownRef.current = next;
      onMarkdownChange(next);
    },
  });

  const getActiveCodeBlockInfo = useCallback(() => {
    if (!editor) return null;
    const sel = editor.state?.selection;
    if (!sel) return null;
    const $from = sel.$from;

    for (let depth = $from.depth; depth >= 0; depth -= 1) {
      const node = $from.node(depth);
      if (node.type.name !== 'codeBlock') continue;
      const pos = $from.before(depth);
      const language = String((node.attrs as any)?.language || '').toLowerCase();
      return { pos, node, language };
    }

    return null;
  }, [editor]);

  const insertMermaidBlock = useCallback(() => {
    if (!editor || readOnly) return;

    const template = 'flowchart TD\n  A[Início] --> B[Fim]';
    editor
      .chain()
      .focus()
      .setCodeBlock({ language: 'mermaid' })
      .insertContent(template)
      .run();

    const info = getActiveCodeBlockInfo();
    if (!info || info.language !== 'mermaid') return;

    const apply = (nextCode: string) => {
      const current = getActiveCodeBlockInfo();
      const target = current && current.language === 'mermaid' ? current : info;
      const from = target.pos + 1;
      const to = target.pos + target.node.nodeSize - 1;

      editor.commands.command(({ tr, state }) => {
        tr.replaceWith(from, to, state.schema.text(nextCode));
        return true;
      });
    };

    const remove = () => {
      const current = getActiveCodeBlockInfo();
      const target = current && current.language === 'mermaid' ? current : info;
      editor.commands.command(({ tr }) => {
        tr.delete(target.pos, target.pos + target.node.nodeSize);
        return true;
      });
    };

    onRequestEditMermaid?.({
      code: template,
      apply,
      remove,
    });
  }, [editor, readOnly, getActiveCodeBlockInfo, onRequestEditMermaid]);

  const closeContextMenu = useCallback(() => {
    setContextMenu((prev) => ({ ...prev, visible: false }));
    window.setTimeout(() => {
      menuTriggerRef.current?.focus?.();
      menuTriggerRef.current = null;
    }, 10);
  }, []);

  const openContextMenuForTrigger = useCallback(
    (triggerEl: HTMLElement, items: MenuItem[], ariaLabel: string) => {
      const rect = triggerEl.getBoundingClientRect();
      menuTriggerRef.current = triggerEl;
      setContextMenu({
        visible: true,
        x: rect.left,
        y: rect.bottom,
        items,
        ariaLabel,
      });
    },
    []
  );

  const getActiveHeadingLabel = useCallback(() => {
    if (!editor) return 'P';
    for (const level of headingLevels) {
      if (editor.isActive('heading', { level })) return `H${level}`;
    }
    return 'P';
  }, [editor, headingLevels]);

  const getActiveTextMarksLabel = useCallback(() => {
    if (!editor) return 'Normal';
    const marks: string[] = [];
    if (editor.isActive('bold')) marks.push('B');
    if (editor.isActive('italic')) marks.push('I');
    if (editor.isActive('strike')) marks.push('S');
    return marks.length > 0 ? marks.join(' ') : 'Normal';
  }, [editor]);

  const getActiveBlockLabel = useCallback(() => {
    if (!editor) return 'Parágrafo';
    if (editor.isActive('bulletList')) return 'Lista';
    if (editor.isActive('orderedList')) return 'Numerada';
    if (editor.isActive('codeBlock')) return 'Código';
    if (editor.isActive('blockquote')) return 'Citação';
    return 'Parágrafo';
  }, [editor]);

  const getTextMenuItems = useCallback((): MenuItem[] => {
    return [
      {
        id: 'toggle-bold',
        label: 'Negrito',
        icon: editor?.isActive('bold') ? '✓' : ' ',
        shortcut: 'Ctrl+B',
        action: () => editor?.chain().focus().toggleBold().run(),
        ariaLabel: 'Negrito, Ctrl+B',
      },
      {
        id: 'toggle-italic',
        label: 'Itálico',
        icon: editor?.isActive('italic') ? '✓' : ' ',
        shortcut: 'Ctrl+I',
        action: () => editor?.chain().focus().toggleItalic().run(),
        ariaLabel: 'Itálico, Ctrl+I',
      },
      {
        id: 'toggle-strike',
        label: 'Tachado',
        icon: editor?.isActive('strike') ? '✓' : ' ',
        shortcut: 'Ctrl+Shift+X',
        action: () => editor?.chain().focus().toggleStrike().run(),
        ariaLabel: 'Tachado, Ctrl+Shift+X',
      },
      { id: 'sep-1', separator: true },
      {
        id: 'clear-marks',
        label: 'Limpar formatação de texto',
        icon: '↺',
        action: () => editor?.chain().focus().unsetAllMarks().run(),
        ariaLabel: 'Limpar formatação de texto',
      },
    ];
  }, [editor]);

  const getHeadingMenuItems = useCallback((): MenuItem[] => {
    const items: MenuItem[] = [
      {
        id: 'set-paragraph',
        label: 'Parágrafo',
        icon: editor && !editor.isActive('heading') ? '✓' : ' ',
        action: () => editor?.chain().focus().setParagraph().run(),
        ariaLabel: 'Parágrafo',
      },
      { id: 'sep-h', separator: true },
    ];

    for (const level of headingLevels) {
      items.push({
        id: `set-h${level}`,
        label: `Cabeçalho ${level} (H${level})`,
        icon: editor?.isActive('heading', { level }) ? '✓' : ' ',
        action: () => editor?.chain().focus().toggleHeading({ level }).run(),
        ariaLabel: `Cabeçalho ${level}`,
      });
    }

    return items;
  }, [editor, headingLevels]);

  const getBlockMenuItems = useCallback((): MenuItem[] => {
    return [
      {
        id: 'toggle-bullet',
        label: 'Lista com marcadores',
        icon: editor?.isActive('bulletList') ? '✓' : ' ',
        action: () => editor?.chain().focus().toggleBulletList().run(),
        ariaLabel: 'Lista com marcadores',
      },
      {
        id: 'toggle-ordered',
        label: 'Lista numerada',
        icon: editor?.isActive('orderedList') ? '✓' : ' ',
        action: () => editor?.chain().focus().toggleOrderedList().run(),
        ariaLabel: 'Lista numerada',
      },
      { id: 'sep-b1', separator: true },
      {
        id: 'toggle-codeblock',
        label: 'Bloco de código',
        icon: editor?.isActive('codeBlock') ? '✓' : ' ',
        action: () => editor?.chain().focus().toggleCodeBlock().run(),
        ariaLabel: 'Bloco de código',
      },
      {
        id: 'toggle-blockquote',
        label: 'Citação',
        icon: editor?.isActive('blockquote') ? '✓' : ' ',
        action: () => editor?.chain().focus().toggleBlockquote().run(),
        ariaLabel: 'Citação',
      },
      { id: 'sep-b2', separator: true },
      {
        id: 'clear-nodes',
        label: 'Limpar bloco (voltar para parágrafo)',
        icon: '↺',
        action: () => editor?.chain().focus().clearNodes().run(),
        ariaLabel: 'Limpar bloco',
      },
    ];
  }, [editor]);

  useEffect(() => {
    onEditorReady?.(editor || null);
    return () => onEditorReady?.(null);
  }, [editor, onEditorReady]);

  useEffect(() => {
    if (!editor) return;
    editor.setEditable(!readOnly);
  }, [editor, readOnly]);

  useEffect(() => {
    if (!editor) return;
    if (markdown === lastMarkdownRef.current) return;

    isApplyingExternalMarkdownRef.current = true;
    try {
      editor.commands.setContent(markdown);
      lastMarkdownRef.current = markdown;
    } finally {
      // libera no próximo tick para evitar loops durante onUpdate
      window.setTimeout(() => {
        isApplyingExternalMarkdownRef.current = false;
      }, 0);
    }
  }, [editor, markdown]);

  return (
    <div className="rich-text-editor" role="region" aria-label={ariaLabel}>
      <Toolbar
        className="rich-text-editor__format-toolbar"
        ariaLabel="Formatação do editor rico. Use setas para navegar"
        left={<div className="rich-text-editor__format-title">Formatação</div>}
        right={
          <div className="rich-text-editor__format-right">
            <button
              type="button"
              className="toolbar__button"
              onClick={() => insertMermaidBlock()}
              aria-label="Inserir diagrama Mermaid"
              title="Inserir Mermaid"
              tabIndex={-1}
              disabled={!editor || readOnly}
            >
              <span className="toolbar__button-icon" aria-hidden="true">▣</span>
              <span className="toolbar__button-label">Mermaid</span>
            </button>

            <button
              type="button"
              className="toolbar__button"
              onClick={(e) => {
                const trigger = e.currentTarget as HTMLElement;
                openContextMenuForTrigger(trigger, getTextMenuItems(), 'Menu de texto');
              }}
              onKeyDown={(e) => {
                if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                  e.preventDefault();
                  const trigger = e.currentTarget as HTMLElement;
                  openContextMenuForTrigger(trigger, getTextMenuItems(), 'Menu de texto');
                }
              }}
              aria-label={`Texto: ${getActiveTextMarksLabel()}. Abrir menu`}
              title="Texto (menu)"
              tabIndex={-1}
            >
              <span className="toolbar__button-icon" aria-hidden="true">A</span>
              <span className="toolbar__button-label">Texto</span>
              <span className="rich-text-editor__picker-value" aria-hidden="true">{getActiveTextMarksLabel()}</span>
            </button>

            <button
              type="button"
              className="toolbar__button"
              onClick={(e) => {
                const trigger = e.currentTarget as HTMLElement;
                openContextMenuForTrigger(trigger, getHeadingMenuItems(), 'Menu de parágrafo');
              }}
              onKeyDown={(e) => {
                if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                  e.preventDefault();
                  const trigger = e.currentTarget as HTMLElement;
                  openContextMenuForTrigger(trigger, getHeadingMenuItems(), 'Menu de parágrafo');
                }
              }}
              aria-label={`Parágrafo: ${getActiveHeadingLabel()}. Abrir menu`}
              title="Parágrafo (menu)"
              tabIndex={-1}
            >
              <span className="toolbar__button-icon" aria-hidden="true">H</span>
              <span className="toolbar__button-label">Parágrafo</span>
              <span className="rich-text-editor__picker-value" aria-hidden="true">{getActiveHeadingLabel()}</span>
            </button>

            <button
              type="button"
              className="toolbar__button"
              onClick={(e) => {
                const trigger = e.currentTarget as HTMLElement;
                openContextMenuForTrigger(trigger, getBlockMenuItems(), 'Menu de blocos');
              }}
              onKeyDown={(e) => {
                if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                  e.preventDefault();
                  const trigger = e.currentTarget as HTMLElement;
                  openContextMenuForTrigger(trigger, getBlockMenuItems(), 'Menu de blocos');
                }
              }}
              aria-label={`Blocos: ${getActiveBlockLabel()}. Abrir menu`}
              title="Blocos (menu)"
              tabIndex={-1}
            >
              <span className="toolbar__button-icon" aria-hidden="true">▦</span>
              <span className="toolbar__button-label">Blocos</span>
              <span className="rich-text-editor__picker-value" aria-hidden="true">{getActiveBlockLabel()}</span>
            </button>
          </div>
        }
      />

      <ContextMenu
        items={contextMenu.items}
        x={contextMenu.x}
        y={contextMenu.y}
        visible={contextMenu.visible}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
        onSelect={() => closeContextMenu()}
      />
      <EditorContent editor={editor} className="rich-text-editor__content" />
    </div>
  );
}
