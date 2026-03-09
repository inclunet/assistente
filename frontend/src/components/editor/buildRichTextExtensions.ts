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
import { ReactNodeViewRenderer } from '@tiptap/react';

import { MermaidCodeBlockNodeView } from './MermaidCodeBlockNodeView';
import { isSafeLinkHref } from '../../lib/safeLink';
import { normalizePastedLinkHref } from '../../lib/linkPaste';

type MermaidRequestCtx = {
  mermaidBlockId: string;
  code: string;
  insertText?: string;
  apply: (nextCode: string) => void;
  remove: () => void;
};

export function buildRichTextExtensions(args: {
  placeholder: string;
  onRequestEditMermaid?: (ctx: MermaidRequestCtx) => void;
}) {
  const { placeholder, onRequestEditMermaid } = args;

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
}
