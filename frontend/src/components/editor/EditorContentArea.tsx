import { type Ref } from 'react';
import { useTranslation } from 'react-i18next';

import { CodeEditor, type CodeEditorProps } from '../ui/CodeEditor';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { RichTextEditor, type RichTextEditorHandle } from './RichTextEditor';
import type { EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';

interface RichMermaidRequestContext {
  mermaidBlockId?: string;
  insertText?: string;
  code?: string;
  apply: (nextCode: string) => void;
  remove: () => void;
}

export interface EditorContentAreaProps {
  activeTab: EditorDocument | null;
  isAsking: boolean;
  debouncedMarkdownForPreview: string;
  onMarkdownChange: (value: string) => void;
  onMonacoMount: NonNullable<CodeEditorProps['onMount']>;
  onRichMarkdownChange: (markdown: string) => void;
  onRichEditorReady: (editor: TipTapEditor | null) => void;
  richEditorHandleRef: Ref<RichTextEditorHandle>;
  onRequestEditMermaid: (ctx: RichMermaidRequestContext) => void;
  onOpenMermaid: (index: number, opts?: { insertText?: string }) => void;
  onRemoveMermaid: (index: number) => void;
}

/**
 * Área de conteúdo do editor: alterna entre vazio, editor Markdown (Monaco),
 * pré-visualização (Markdown renderizado) e editor rico (TipTap).
 * Mantém o comportamento original, incluindo a interação por teclado com
 * diagramas Mermaid no modo de visualização.
 */
export function EditorContentArea({
  activeTab,
  isAsking,
  debouncedMarkdownForPreview,
  onMarkdownChange,
  onMonacoMount,
  onRichMarkdownChange,
  onRichEditorReady,
  richEditorHandleRef,
  onRequestEditMermaid,
  onOpenMermaid,
  onRemoveMermaid,
}: EditorContentAreaProps) {
  const { t } = useTranslation();

  return (
    <div className="editor-page__content ws-content-area">
      {!activeTab ? (
        <div className="editor-page__empty">{t('editor.empty.noTabs')}</div>
      ) : activeTab.mode === 'markdown' ? (
        <div className={'editor-page__single'}>
          <div className="editor-page__pane" role="region" aria-label="Editor Markdown">
            <div className="editor-page__pane-title">{t('editor.panes.markdown')}</div>
            <div className="editor-page__pane-body">
              <CodeEditor
                height="100%"
                language="markdown"
                ariaLabel={t('editor.aria.markdownEditor')}
                value={activeTab.markdown}
                pasteUrlAsMarkdownLink={true}
                onChange={onMarkdownChange}
                placeholder={t('editor.placeholders.markdown')}
                readOnly={isAsking}
                onMount={onMonacoMount}
              />
            </div>
          </div>
        </div>
      ) : activeTab.mode === 'view' ? (
        <div className="editor-page__single">
          <div
            className="editor-page__pane"
            role="region"
            aria-label={t('editor.aria.preview')}
            onDoubleClick={(e) => {
              const target = e.target as HTMLElement | null;
              const wrapper = target?.closest?.('.mermaid-diagram') as HTMLElement | null;
              if (!wrapper) return;
              const raw = wrapper.dataset.mermaidIndex;
              const index = raw ? Number(raw) : NaN;
              if (!Number.isFinite(index)) return;
              onOpenMermaid(index);
            }}
            onKeyDown={(e) => {
              const target = e.target as HTMLElement | null;
              const wrapper = target?.closest?.('.mermaid-diagram') as HTMLElement | null;
              if (!wrapper) return;

              const raw = wrapper.dataset.mermaidIndex;
              const index = raw ? Number(raw) : NaN;
              if (!Number.isFinite(index)) return;

              if (e.key === 'Enter') {
                e.preventDefault();
                onOpenMermaid(index);
                return;
              }

              if (e.key === 'Backspace' || e.key === 'Delete') {
                e.preventDefault();
                onRemoveMermaid(index);
                return;
              }

              // Type-to-edit: abre o editor de Mermaid e injeta o primeiro caractere.
              if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && !e.altKey && !e.shiftKey) {
                e.preventDefault();
                onOpenMermaid(index, { insertText: e.key });
              }
            }}
          >
            <div className="editor-page__pane-title">{t('editor.panes.preview')}</div>
            <div className="editor-page__preview">
              <div className="editor-page__preview-hint">{t('editor.hints.previewMermaid')}</div>
              <MarkdownRenderer
                content={debouncedMarkdownForPreview}
                interactiveButtons={false}
                focusableMermaid={true}
              />
            </div>
          </div>
        </div>
      ) : (
        <div className="editor-page__single">
          <div className="editor-page__pane" role="region" aria-label={t('editor.aria.richEditor')}>
            <div className="editor-page__pane-title">{t('editor.panes.rich')}</div>
            <div className="editor-page__pane-body">
              <RichTextEditor
                ref={richEditorHandleRef}
                ariaLabel={t('editor.richText.label')}
                markdown={activeTab.markdown}
                onMarkdownChange={onRichMarkdownChange}
                readOnly={isAsking}
                placeholder={t('editor.placeholders.rich')}
                onEditorReady={onRichEditorReady}
                onRequestEditMermaid={onRequestEditMermaid}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
