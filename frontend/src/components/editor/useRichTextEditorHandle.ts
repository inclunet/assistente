import { useImperativeHandle, useRef } from 'react';

import type { RichTextEditorHandle } from './RichTextEditor';
import type { EditorLike } from './richMarkdownSync';

type MarkdownSyncLike = {
  getMarkdownNow: (editor: EditorLike) => string;
  flushNow: (editor: EditorLike) => void;
};

type Args = {
  ref: React.Ref<RichTextEditorHandle>;
  editor: EditorLike | null;
  markdown: string;
  markdownSync: MarkdownSyncLike;
  openLinkDialog: () => Promise<void>;
};

export function useRichTextEditorHandle({
  ref,
  editor,
  markdown,
  markdownSync,
  openLinkDialog,
}: Args) {
  const live = useRef({ editor, markdown, markdownSync, openLinkDialog });
  live.current = { editor, markdown, markdownSync, openLinkDialog };
  useImperativeHandle(
    ref,
    () => ({
      getMarkdown: () => {
        if (live.current.editor !== editor) return '';
        if (!editor) return String(live.current.markdown || '');
        return live.current.markdownSync.getMarkdownNow(editor);
      },
      flushMarkdown: () => {
        if (!editor || live.current.editor !== editor) return;
        live.current.markdownSync.flushNow(editor);
      },
      openLinkDialog: async () => {
        if (live.current.editor !== editor) return;
        await live.current.openLinkDialog();
      },
    }),
    [editor]
  );
}
