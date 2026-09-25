import { forwardRef, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { EditorContent, useEditor, type Editor } from '@tiptap/react';

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
    expectedEditor?: object;
  }) => void;
}

export type RichTextEditorHandle = {
  getMarkdown: () => string;
  flushMarkdown: () => void;
  openLinkDialog: () => Promise<void>;
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
  const imageFallbackLabel = t('editor.richText.imageFallbackLabel');
  const imageLabelPrefix = t('editor.richText.imageLabelPrefix');
  const markdownSync = useRichMarkdownSync({
    markdown,
    onMarkdownChange,
    debounceMs: 300,
  });

  const extensions = useMemo(() => {
    return buildRichTextExtensions({
      placeholder: resolvedPlaceholder,
      imageFallbackLabel,
      imageLabelPrefix,
      onRequestEditMermaid,
    });
  }, [resolvedPlaceholder, imageFallbackLabel, imageLabelPrefix, onRequestEditMermaid]);

  const editor = useEditor({
    extensions,
    content: markdown,
    editable: !readOnly,
    onUpdate: markdownSync.onUpdate,
  });

  const openLinkDialog = useRichLinkDialog({ editor, readOnly });

  useRichTextEditorHandle({
    ref,
    editor,
    markdown,
    markdownSync,
    openLinkDialog,
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
    >
      <EditorContent editor={editor} className="rich-text-editor__content" />
    </div>
  );
});
