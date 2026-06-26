import { Extension, mergeAttributes } from '@tiptap/core';
import StarterKit from '@tiptap/starter-kit';
import CodeBlock, { type CodeBlockOptions } from '@tiptap/extension-code-block';
import Placeholder from '@tiptap/extension-placeholder';
import Link from '@tiptap/extension-link';
import Image from '@tiptap/extension-image';
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

function isSafeImageSrc(src: string): boolean {
  const raw = String(src || '').trim();
  return !!raw && isSafeLinkHref(raw) && !/^mailto:/i.test(raw);
}

export function buildRichTextExtensions(args: {
  placeholder: string;
  imageFallbackLabel: string;
  imageLabelPrefix: string;
  onRequestEditMermaid?: (ctx: MermaidRequestCtx) => void;
}) {
  const { placeholder, imageFallbackLabel, imageLabelPrefix, onRequestEditMermaid } = args;

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
      const parent = this.parent?.() as CodeBlockOptions | undefined;
      return {
        ...(parent ?? {}),
        onRequestEditMermaid: undefined,
      } as CodeBlockOptions & { onRequestEditMermaid?: (ctx: MermaidRequestCtx) => void };
    },
    addNodeView() {
      return ReactNodeViewRenderer(MermaidCodeBlockNodeView);
    },
  });

  const AccessibleImage = Image.extend({
    addNodeView() {
      return ({ node }) => {
        const src = String(node.attrs.src || '');
        const safeSrc = isSafeImageSrc(src) ? src : '';
        const alt = String(node.attrs.alt || '').trim();
        const title = String(node.attrs.title || '').trim();
        const accessibleLabel = alt || title || imageFallbackLabel;
        const describedLabel = `${imageLabelPrefix}: ${accessibleLabel}`;

        const figure = document.createElement('figure');
        figure.className = 'rich-text-editor__image-node';
        figure.contentEditable = 'false';
        figure.setAttribute('role', 'group');
        figure.setAttribute('aria-label', describedLabel);
        figure.setAttribute('tabindex', '0');
        figure.dataset.richImageNode = 'true';

        const image = document.createElement('img');
        if (safeSrc) {
          image.setAttribute('src', safeSrc);
        }
        image.alt = accessibleLabel;
        image.title = title || accessibleLabel;

        const caption = document.createElement('figcaption');
        caption.className = 'rich-text-editor__image-description';
        caption.textContent = describedLabel;

        figure.append(image, caption);

        return { dom: figure };
      };
    },
    renderHTML({ HTMLAttributes }) {
      const src = String(HTMLAttributes.src || '');
      const safeSrc = isSafeImageSrc(src) ? src : '';
      const alt = String(HTMLAttributes.alt || '').trim();
      const title = String(HTMLAttributes.title || '').trim();
      const accessibleLabel = alt || title || imageFallbackLabel;
      return [
        'img',
        mergeAttributes(
          this.options.HTMLAttributes,
          HTMLAttributes,
          {
            src: safeSrc,
            'aria-label': accessibleLabel,
            title: title || accessibleLabel,
          }
        ),
      ];
    },
  });

  return [
    StarterKit.configure({
      codeBlock: false,
      link: false,
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
    AccessibleImage.configure({
      inline: false,
      allowBase64: false,
    }),
    PasteUrlAsLinkOnSelection,
    MermaidAwareCodeBlock.configure({
      languageClassPrefix: 'language-',
      defaultLanguage: null,
      onRequestEditMermaid,
    } as { languageClassPrefix?: string; defaultLanguage?: string | null; onRequestEditMermaid?: (ctx: MermaidRequestCtx) => void }),
    Placeholder.configure({
      placeholder,
    }),
    Markdown.configure({
      html: false,
      transformPastedText: true,
    }),
  ];
}
