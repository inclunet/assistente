import { forwardRef, useCallback, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { EditorContent, useEditor, type Editor } from '@tiptap/react';

import { applyMermaidById as applyMermaidByIdInEditor, removeMermaidById as removeMermaidByIdInEditor } from './richMermaidById';
import { useRichLinkDialog } from './useRichLinkDialog';
import { buildRichTextExtensions } from './buildRichTextExtensions';
import { useRichMarkdownSync } from './useRichMarkdownSync';
import { useRichTextEditorHandle } from './useRichTextEditorHandle';
import type { EditorLike } from './richMarkdownSync';

import './RichTextEditor.css';

export interface RichTextEditorProps {
  markdown: string;
  onMarkdownChange: (markdown: string) => void;
  readOnly?: boolean;
  placeholder?: string;
  ariaLabel?: string;
  onEditorReady?: (editor: Editor | null) => void;
  onRequestEditMermaid?: (ctx: {
    mermaidBlockId: string;
    code: string;
    insertText?: string;
    apply: (nextCode: string) => void;
    remove: () => void;
  }) => void;
}

export type RichTextEditorHandle = {
  getMarkdown: () => string;
  flushMarkdown: () => void;
  openLinkDialog: () => Promise<void>;
  applyMermaidById: (mermaidBlockId: string, nextCode: string) => boolean;
  removeMermaidById: (mermaidBlockId: string) => boolean;
};

export const RichTextEditor = forwardRef<RichTextEditorHandle, RichTextEditorProps>(function RichTextEditor(
  {
    markdown,
    onMarkdownChange,
    readOnly = false,
    placeholder,
    ariaLabel,
    onEditorReady,
    onRequestEditMermaid,
  }: RichTextEditorProps,
  ref
) {
  const { t } = useTranslation();
  const resolvedPlaceholder = placeholder ?? t('editor.richText.placeholder');
  const resolvedAriaLabel = ariaLabel ?? t('editor.richText.label');
  const markdownSync = useRichMarkdownSync({
    markdown,
    onMarkdownChange,
    debounceMs: 300,
  });

  const extensions = useMemo(() => {
    return buildRichTextExtensions({
      placeholder: resolvedPlaceholder,
      onRequestEditMermaid,
    });
  }, [resolvedPlaceholder, onRequestEditMermaid]);

  const editor = useEditor({
    extensions,
    content: markdown,
    editable: !readOnly,
    onUpdate: markdownSync.onUpdate,
  });

  const openLinkDialog = useRichLinkDialog({ editor, readOnly });

  const applyMermaidById = useCallback(
    (mermaidBlockId: string, nextCode: string) => applyMermaidByIdInEditor(editor, mermaidBlockId, nextCode),
    [editor]
  );

  const removeMermaidById = useCallback(
    (mermaidBlockId: string) => removeMermaidByIdInEditor(editor, mermaidBlockId),
    [editor]
  );

  useRichTextEditorHandle({
    ref,
    editor,
    markdown,
    markdownSync,
    openLinkDialog,
    applyMermaidById,
    removeMermaidById,
  });

  const onEditorReadyRef = useRef(onEditorReady);
  onEditorReadyRef.current = onEditorReady;

  useEffect(() => {
    onEditorReadyRef.current?.(editor || null);
    return () => onEditorReadyRef.current?.(null);
  }, [editor]);

  useEffect(() => {
    if (!editor) return;
    editor.setEditable(!readOnly);
  }, [editor, readOnly]);

  useEffect(() => {
    markdownSync.syncFromExternal(editor ? (editor as EditorLike) : null, markdown);
  }, [editor, markdown, markdownSync.syncFromExternal]);

  return (
    <div
      className="rich-text-editor"
      role="region"
      aria-label={resolvedAriaLabel}
      onKeyDown={(e) => {
        if ((e.ctrlKey || e.metaKey) && (e.key === 'k' || e.key === 'K')) {
          e.preventDefault();
          void openLinkDialog();
        }
      }}
    >
      <EditorContent editor={editor} className="rich-text-editor__content" />
    </div>
  );
});
