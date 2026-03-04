import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { EditorContent, ReactNodeViewRenderer, useEditor } from '@tiptap/react';
import { Extension } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import CodeBlock from '@tiptap/extension-code-block';
import Placeholder from '@tiptap/extension-placeholder';
import Link from '@tiptap/extension-link';
import { Table } from '@tiptap/extension-table';
import TableRow from '@tiptap/extension-table-row';
import TableHeader from '@tiptap/extension-table-header';
import TableCell from '@tiptap/extension-table-cell';
import { Markdown } from 'tiptap-markdown';
import { Plugin } from '@tiptap/pm/state';

import { Toolbar } from '../ui/Toolbar';
import { Menu, type MenuItem } from '../menu';
import { useAnchoredContextMenu } from '../../hooks/useAnchoredContextMenu';
import { MermaidCodeBlockNodeView } from './MermaidCodeBlockNodeView';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { useUIStore } from '../../store/uiStore';
import { isSafeLinkHref } from '../../lib/safeLink';
import { normalizePastedLinkHref } from '../../lib/linkPaste';

import './RichTextEditor.css';

export interface RichTextEditorProps {
  markdown: string;
  onMarkdownChange: (markdown: string) => void;
  readOnly?: boolean;
  placeholder?: string;
  ariaLabel?: string;
  onEditorReady?: (editor: any | null) => void;
  onRequestEditMermaid?: (ctx: {
    mermaidBlockId: string;
    code: string;
    insertText?: string;
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
  const { addToast } = useUIStore();
  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);

  const headingLevels = useMemo(() => [1, 2, 3, 4, 5, 6] as const, []);

  const isApplyingExternalMarkdownRef = useRef(false);
  const lastMarkdownRef = useRef<string>(markdown);
  const pendingMarkdownRef = useRef<string | null>(null);
  const markdownEmitTimerRef = useRef<number | null>(null);

  const {
    menu: contextMenu,
    openForTrigger,
    closeMenu: closeContextMenu,
    onSelectItem: onSelectContextMenuItem,
  } = useAnchoredContextMenu();

  const extensions = useMemo(() => {
    const PasteUrlAsLinkOnSelection = Extension.create({
      name: 'pasteUrlAsLinkOnSelection',
      addProseMirrorPlugins() {
        return [
          new Plugin({
            props: {
              handlePaste: (_view, event) => {
                if (!this.editor.isEditable) return false;

                const sel = this.editor.state.selection;
                if (!sel || sel.empty) return false;

                const href = normalizePastedLinkHref(event.clipboardData?.getData('text/plain') ?? '');
                if (!href) return false;

                this.editor.chain().focus().setLink({ href }).run();
                return true;
              },
            },
          }),
        ];
      },
    });

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
      Table.configure({
        resizable: true,
      }),
      TableRow,
      TableHeader,
      TableCell,
      Link.configure({
        openOnClick: false,
        linkOnPaste: true,
        autolink: true,
        validate: (href: string) => isSafeLinkHref(href),
        HTMLAttributes: {
          target: '_blank',
          rel: 'noopener noreferrer',
        },
      }),
      PasteUrlAsLinkOnSelection,
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

      // Debounce para evitar emitir Markdown a cada transação.
      pendingMarkdownRef.current = next;
      if (markdownEmitTimerRef.current) {
        window.clearTimeout(markdownEmitTimerRef.current);
      }
      markdownEmitTimerRef.current = window.setTimeout(() => {
        markdownEmitTimerRef.current = null;
        const pending = pendingMarkdownRef.current;
        pendingMarkdownRef.current = null;
        if (typeof pending !== 'string') return;
        if (pending === lastMarkdownRef.current) return;
        lastMarkdownRef.current = pending;
        onMarkdownChange(pending);
      }, 300);
    },
  });

  // Expõe helpers imperativos no editor (EditorPage usa para flush antes de salvar/trocar modo etc.).
  useEffect(() => {
    if (!editor) return;

    const getMarkdownNow = () => {
      try {
        const md = (editor.storage as any)?.markdown?.getMarkdown?.() as string | undefined;
        return typeof md === 'string' ? md : '';
      } catch {
        return '';
      }
    };

    const flushMarkdown = () => {
      if (isApplyingExternalMarkdownRef.current) return;
      if (markdownEmitTimerRef.current) {
        window.clearTimeout(markdownEmitTimerRef.current);
        markdownEmitTimerRef.current = null;
      }
      const next = pendingMarkdownRef.current ?? getMarkdownNow();
      pendingMarkdownRef.current = null;
      if (next === lastMarkdownRef.current) return;
      lastMarkdownRef.current = next;
      onMarkdownChange(next);
    };

    (editor as any).__getMarkdown = getMarkdownNow;
    (editor as any).__flushMarkdown = flushMarkdown;

    return () => {
      try {
        delete (editor as any).__getMarkdown;
        delete (editor as any).__flushMarkdown;
      } catch {
        // best-effort
      }
    };
  }, [editor, onMarkdownChange]);

  // Limpa timer do debounce ao desmontar.
  useEffect(() => {
    return () => {
      if (markdownEmitTimerRef.current) {
        try {
          window.clearTimeout(markdownEmitTimerRef.current);
        } catch {
          // best-effort
        }
      }
      markdownEmitTimerRef.current = null;
      pendingMarkdownRef.current = null;
    };
  }, []);

  const openLinkDialog = useCallback(async () => {
    if (!editor || readOnly) return;

    const existingHref = String(editor.getAttributes('link')?.href || '').trim();
    const sel = editor.state?.selection;
    const selectedText = sel ? editor.state.doc.textBetween(sel.from, sel.to, '\n') : '';
    const selectionEmpty = !sel || sel.empty;

    const resp = await requestQuestionnaire({
      id: `ui-rich-link-${Date.now()}`,
      title: existingHref ? 'Editar link' : 'Inserir link',
      description: selectionEmpty
        ? 'Informe a URL. Se quiser, informe também o texto do link para inserir no cursor.'
        : 'Informe a URL para aplicar no texto selecionado.',
      submitLabel: existingHref ? 'Salvar link' : 'Inserir link',
      cancelLabel: 'Cancelar',
      allowCancel: true,
      questions: [
        {
          id: 'href',
          type: 'text',
          prompt: 'URL',
          placeholder: 'https://… (ou /caminho, #ancora, mailto:…)',
          required: true,
          default: existingHref,
        },
        ...(selectionEmpty
          ? [
              {
                id: 'text',
                type: 'text',
                prompt: 'Texto (opcional)',
                placeholder: 'Texto do link',
                required: false,
                default: selectedText || '',
              } as const,
            ]
          : []),
      ],
    });

    if (resp.cancelled) return;

    const href = String(resp.answers.href || '').trim();
    if (!isSafeLinkHref(href)) {
      addToast('Link inválido ou inseguro. Use http(s), mailto, /caminho ou #ancora.', 'error');
      return;
    }

    if (selectionEmpty) {
      const text = String(resp.answers.text || '').trim() || href;
      editor
        .chain()
        .focus()
        .insertContent({
          type: 'text',
          text,
          marks: [{ type: 'link', attrs: { href } }],
        })
        .run();
      return;
    }

    editor.chain().focus().extendMarkRange('link').setLink({ href }).run();
  }, [editor, readOnly, requestQuestionnaire, addToast]);

  const removeLink = useCallback(() => {
    if (!editor || readOnly) return;
    editor.chain().focus().extendMarkRange('link').unsetLink().run();
  }, [editor, readOnly]);

  const getActiveTableLabel = useCallback(() => {
    if (!editor) return '—';
    return editor.isActive('table') ? 'Na tabela' : '—';
  }, [editor]);

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

  const newMermaidBlockId = useCallback((): string => {
    try {
      return crypto.randomUUID();
    } catch {
      return `mermaid-${Date.now()}-${Math.random().toString(16).slice(2)}`;
    }
  }, []);

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

    // Garante um ID estável para o bloco recém-criado, para edição fora do cursor.
    const ensuredMermaidBlockId = (() => {
      const cur = String((info.node.attrs as any)?.mermaidBlockId || '').trim();
      if (cur) return cur;
      const nextId = newMermaidBlockId();
      try {
        editor.commands.command(({ tr }) => {
          const attrs = { ...(info.node.attrs as any), mermaidBlockId: nextId };
          tr.setNodeMarkup(info.pos, undefined, attrs);
          return true;
        });
      } catch {
        // best-effort
      }
      return nextId;
    })();

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
      mermaidBlockId: ensuredMermaidBlockId,
      code: template,
      apply,
      remove,
    });
  }, [editor, readOnly, getActiveCodeBlockInfo, onRequestEditMermaid, newMermaidBlockId]);

  const openContextMenuForTrigger = useCallback(
    (triggerEl: HTMLElement, items: MenuItem[], ariaLabel: string) => {
      openForTrigger(triggerEl, ariaLabel, items);
    },
    [openForTrigger]
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
        id: 'set-link',
        label: 'Inserir/Editar link',
        icon: editor?.isActive('link') ? '✓' : ' ',
        shortcut: 'Ctrl+K',
        action: () => void openLinkDialog(),
        ariaLabel: 'Inserir ou editar link, Ctrl+K',
      },
      {
        id: 'unset-link',
        label: 'Remover link',
        icon: '×',
        action: () => removeLink(),
        ariaLabel: 'Remover link',
        disabled: !editor?.isActive('link'),
      },
      { id: 'sep-link', separator: true },
      {
        id: 'clear-marks',
        label: 'Limpar formatação de texto',
        icon: '↺',
        action: () => editor?.chain().focus().unsetAllMarks().run(),
        ariaLabel: 'Limpar formatação de texto',
      },
    ];
  }, [editor, openLinkDialog, removeLink]);

  const getTableMenuItems = useCallback((): MenuItem[] => {
    const canUse = !!editor && !readOnly;
    const inTable = !!editor?.isActive('table');

    const canRun = (fn: () => boolean) => {
      if (!editor || readOnly) return false;
      try {
        return !!fn();
      } catch {
        return false;
      }
    };

    const canAddRowBefore = inTable && canRun(() => editor!.can().chain().focus().addRowBefore().run());
    const canAddRowAfter = inTable && canRun(() => editor!.can().chain().focus().addRowAfter().run());
    const canDeleteRow = inTable && canRun(() => editor!.can().chain().focus().deleteRow().run());
    const canAddColBefore = inTable && canRun(() => editor!.can().chain().focus().addColumnBefore().run());
    const canAddColAfter = inTable && canRun(() => editor!.can().chain().focus().addColumnAfter().run());
    const canDeleteCol = inTable && canRun(() => editor!.can().chain().focus().deleteColumn().run());
    const canDeleteTable = inTable && canRun(() => editor!.can().chain().focus().deleteTable().run());

    const canToggleHeaderRow = inTable && canRun(() => editor!.can().chain().focus().toggleHeaderRow().run());
    const canToggleHeaderCol = inTable && canRun(() => editor!.can().chain().focus().toggleHeaderColumn().run());
    const canToggleHeaderCell = inTable && canRun(() => editor!.can().chain().focus().toggleHeaderCell().run());

    const canMergeCells = inTable && canRun(() => (editor as any)!.can().chain().focus().mergeCells().run());
    const canSplitCell = inTable && canRun(() => (editor as any)!.can().chain().focus().splitCell().run());
    const canNextCell = inTable && canRun(() => (editor as any)!.can().chain().focus().goToNextCell().run());
    const canPrevCell = inTable && canRun(() => (editor as any)!.can().chain().focus().goToPreviousCell().run());

    return [
      {
        id: 'insert-table',
        label: 'Inserir tabela',
        icon: '▦',
        ariaLabel: 'Inserir tabela',
        disabled: !canUse,
        submenu: [
          {
            id: 'insert-table-2x2',
            label: '2 × 2 (com cabeçalho)',
            icon: ' ',
            ariaLabel: 'Inserir tabela 2 por 2 com cabeçalho',
            disabled: !canUse,
            action: () => editor?.chain().focus().insertTable({ rows: 2, cols: 2, withHeaderRow: true }).run(),
          },
          {
            id: 'insert-table-3x3',
            label: '3 × 3 (com cabeçalho)',
            icon: ' ',
            ariaLabel: 'Inserir tabela 3 por 3 com cabeçalho',
            disabled: !canUse,
            action: () => editor?.chain().focus().insertTable({ rows: 3, cols: 3, withHeaderRow: true }).run(),
          },
          {
            id: 'insert-table-4x4',
            label: '4 × 4 (com cabeçalho)',
            icon: ' ',
            ariaLabel: 'Inserir tabela 4 por 4 com cabeçalho',
            disabled: !canUse,
            action: () => editor?.chain().focus().insertTable({ rows: 4, cols: 4, withHeaderRow: true }).run(),
          },
        ],
      },
      { id: 'sep-t-1', separator: true },
      {
        id: 'row-before',
        label: 'Adicionar linha acima',
        icon: '+',
        ariaLabel: 'Adicionar linha acima',
        disabled: !canAddRowBefore,
        action: () => editor?.chain().focus().addRowBefore().run(),
      },
      {
        id: 'row-after',
        label: 'Adicionar linha abaixo',
        icon: '+',
        ariaLabel: 'Adicionar linha abaixo',
        disabled: !canAddRowAfter,
        action: () => editor?.chain().focus().addRowAfter().run(),
      },
      {
        id: 'del-row',
        label: 'Remover linha',
        icon: '−',
        ariaLabel: 'Remover linha',
        disabled: !canDeleteRow,
        action: () => editor?.chain().focus().deleteRow().run(),
      },
      { id: 'sep-t-2', separator: true },
      {
        id: 'col-before',
        label: 'Adicionar coluna antes',
        icon: '+',
        ariaLabel: 'Adicionar coluna antes',
        disabled: !canAddColBefore,
        action: () => editor?.chain().focus().addColumnBefore().run(),
      },
      {
        id: 'col-after',
        label: 'Adicionar coluna depois',
        icon: '+',
        ariaLabel: 'Adicionar coluna depois',
        disabled: !canAddColAfter,
        action: () => editor?.chain().focus().addColumnAfter().run(),
      },
      {
        id: 'del-col',
        label: 'Remover coluna',
        icon: '−',
        ariaLabel: 'Remover coluna',
        disabled: !canDeleteCol,
        action: () => editor?.chain().focus().deleteColumn().run(),
      },
      { id: 'sep-t-3', separator: true },
      {
        id: 'toggle-header-row',
        label: 'Alternar cabeçalho (linha)',
        icon: editor?.isActive('tableHeader') ? '✓' : ' ',
        ariaLabel: 'Alternar cabeçalho da linha',
        disabled: !canToggleHeaderRow,
        action: () => editor?.chain().focus().toggleHeaderRow().run(),
      },
      {
        id: 'toggle-header-col',
        label: 'Alternar cabeçalho (coluna)',
        icon: ' ',
        ariaLabel: 'Alternar cabeçalho da coluna',
        disabled: !canToggleHeaderCol,
        action: () => editor?.chain().focus().toggleHeaderColumn().run(),
      },
      {
        id: 'toggle-header-cell',
        label: 'Alternar cabeçalho (célula)',
        icon: ' ',
        ariaLabel: 'Alternar cabeçalho da célula',
        disabled: !canToggleHeaderCell,
        action: () => editor?.chain().focus().toggleHeaderCell().run(),
      },
      { id: 'sep-t-4', separator: true },
      {
        id: 'merge-cells',
        label: 'Mesclar células',
        icon: '⇔',
        ariaLabel: 'Mesclar células',
        disabled: !canMergeCells,
        action: () => (editor as any)?.chain().focus().mergeCells().run(),
      },
      {
        id: 'split-cell',
        label: 'Separar célula',
        icon: '⇕',
        ariaLabel: 'Separar célula',
        disabled: !canSplitCell,
        action: () => (editor as any)?.chain().focus().splitCell().run(),
      },
      { id: 'sep-t-5', separator: true },
      {
        id: 'prev-cell',
        label: 'Ir para célula anterior',
        icon: '←',
        ariaLabel: 'Ir para célula anterior',
        disabled: !canPrevCell,
        action: () => (editor as any)?.chain().focus().goToPreviousCell().run(),
      },
      {
        id: 'next-cell',
        label: 'Ir para próxima célula',
        icon: '→',
        ariaLabel: 'Ir para próxima célula',
        disabled: !canNextCell,
        action: () => (editor as any)?.chain().focus().goToNextCell().run(),
      },
      { id: 'sep-t-6', separator: true },
      {
        id: 'delete-table',
        label: 'Apagar tabela',
        icon: '🗑',
        ariaLabel: 'Apagar tabela',
        danger: true,
        disabled: !canDeleteTable,
        action: () => editor?.chain().focus().deleteTable().run(),
      },
    ];
  }, [editor, readOnly]);

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
    <div
      className="rich-text-editor"
      role="region"
      aria-label={ariaLabel}
      onKeyDown={(e) => {
        if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
          e.preventDefault();
          void openLinkDialog();
        }
      }}
    >
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

            <button
              type="button"
              className="toolbar__button"
              onClick={(e) => {
                const trigger = e.currentTarget as HTMLElement;
                openContextMenuForTrigger(trigger, getTableMenuItems(), 'Menu de tabela');
              }}
              onKeyDown={(e) => {
                if (e.key === 'ContextMenu' || (e.shiftKey && e.key === 'F10')) {
                  e.preventDefault();
                  const trigger = e.currentTarget as HTMLElement;
                  openContextMenuForTrigger(trigger, getTableMenuItems(), 'Menu de tabela');
                }
              }}
              aria-label={`Tabela: ${getActiveTableLabel()}. Abrir menu`}
              title="Tabela (menu)"
              tabIndex={-1}
              disabled={!editor || readOnly}
            >
              <span className="toolbar__button-icon" aria-hidden="true">▦</span>
              <span className="toolbar__button-label">Tabela</span>
              <span className="rich-text-editor__picker-value" aria-hidden="true">{getActiveTableLabel()}</span>
            </button>
          </div>
        }
      />

      <Menu
        items={contextMenu.items}
        x={contextMenu.x}
        y={contextMenu.y}
        visible={contextMenu.visible}
        ariaLabel={contextMenu.ariaLabel}
        onClose={closeContextMenu}
        onSelect={onSelectContextMenuItem}
      />
      <EditorContent editor={editor} className="rich-text-editor__content" />
    </div>
  );
}
