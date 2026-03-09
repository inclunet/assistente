import { useImperativeHandle } from 'react';

import type { RichTextEditorHandle } from './RichTextEditor';

type EditorLike = unknown;

type MarkdownSyncLike = {
  getMarkdownNow: (editor: any) => string;
  flushNow: (editor: any) => void;
};

type Args = {
  ref: React.Ref<RichTextEditorHandle>;
  editor: EditorLike | null;
  markdown: string;
  markdownSync: MarkdownSyncLike;
  openLinkDialog: () => Promise<void>;
  applyMermaidById: (mermaidBlockId: string, nextCode: string) => boolean;
  removeMermaidById: (mermaidBlockId: string) => boolean;
};

export function useRichTextEditorHandle({
  ref,
  editor,
  markdown,
  markdownSync,
  openLinkDialog,
  applyMermaidById,
  removeMermaidById,
}: Args) {
  useImperativeHandle(
    ref,
    () => ({
      getMarkdown: () => {
        if (!editor) return String(markdown || '');
        return markdownSync.getMarkdownNow(editor as any);
      },
      flushMarkdown: () => {
        if (!editor) return;
        markdownSync.flushNow(editor as any);
      },
      openLinkDialog: async () => {
        await openLinkDialog();
      },
      applyMermaidById,
      removeMermaidById,
    }),
    [applyMermaidById, editor, markdown, markdownSync, openLinkDialog, removeMermaidById]
  );
}
