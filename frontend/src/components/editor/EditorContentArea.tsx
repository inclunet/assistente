import { type Ref, type RefObject, useEffect, useId, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CodeEditor, type CodeEditorProps } from '../ui/CodeEditor';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { RichTextEditor, type RichTextEditorHandle } from './RichTextEditor';
import { RevealRenderer } from './RevealRenderer';
import type { EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';
import {
  getRevealSlideEditableMarkdown,
  mergeRevealSlideEditableMarkdown,
  parseRevealMarkdown,
  replaceRevealSlide,
} from '../../lib/revealMarkdown';

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
  richEditorHandleRef: RefObject<RichTextEditorHandle | null>;
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
  const slideSelectId = useId();
  const [activeRevealSlideIndex, setActiveRevealSlideIndex] = useState(0);
  const revealDeck = useMemo(
    () => parseRevealMarkdown(activeTab?.markdown || ''),
    [activeTab?.markdown]
  );
  const isRevealDocument = revealDeck.detection.kind === 'reveal' && revealDeck.slides.length > 0;
  const activeRevealSlide = isRevealDocument
    ? revealDeck.slides[Math.min(activeRevealSlideIndex, revealDeck.slides.length - 1)] ?? revealDeck.slides[0]
    : null;
  const activeRevealSlideEditableMarkdown = activeRevealSlide
    ? getRevealSlideEditableMarkdown(activeRevealSlide.markdown)
    : null;

  useEffect(() => {
    setActiveRevealSlideIndex(0);
  }, [activeTab?.id]);

  useEffect(() => {
    if (!isRevealDocument) {
      setActiveRevealSlideIndex(0);
      return;
    }
    if (activeRevealSlideIndex >= revealDeck.slides.length) {
      setActiveRevealSlideIndex(Math.max(0, revealDeck.slides.length - 1));
    }
  }, [activeRevealSlideIndex, isRevealDocument, revealDeck.slides.length]);

  const switchRevealSlide = (nextIndex: number) => {
    richEditorHandleRef.current?.flushMarkdown?.();
    setActiveRevealSlideIndex(Math.max(0, Math.min(nextIndex, revealDeck.slides.length - 1)));
  };

  const createRevealSlide = () => {
    richEditorHandleRef.current?.flushMarkdown?.();
    const base = activeTab?.markdown || '';
    const nextMarkdown = `${base.trimEnd()}\n\n---\n\n<!-- .slide: class="content-slide" -->\n\n## ${t('editor.presentation.newSlideTitle')}\n`;
    onMarkdownChange(nextMarkdown);
    setActiveRevealSlideIndex(revealDeck.slides.length);
  };

  const handleRichMarkdownChange = (markdown: string) => {
    if (!activeTab || !activeRevealSlide) {
      onRichMarkdownChange(markdown);
      return;
    }
    onRichMarkdownChange(
      replaceRevealSlide(
        activeTab.markdown,
        activeRevealSlide,
        mergeRevealSlideEditableMarkdown(activeRevealSlide.markdown, markdown)
      )
    );
  };

  return (
    <div className="editor-page__content ws-content-area">
      {!activeTab ? (
        <div className="editor-page__empty">{t('editor.empty.noTabs')}</div>
      ) : activeTab.mode === 'markdown' ? (
        <div className={'editor-page__single'}>
          <div className="editor-page__pane" role="region" aria-label={t('editor.aria.markdownEditor')}>
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
              {isRevealDocument ? (
                <RevealRenderer markdown={debouncedMarkdownForPreview} />
              ) : (
                <>
                  <div className="editor-page__preview-hint">{t('editor.hints.previewMermaid')}</div>
                  <MarkdownRenderer
                    content={debouncedMarkdownForPreview}
                    interactiveButtons={false}
                    focusableMermaid={true}
                  />
                </>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div className="editor-page__single">
          <div className="editor-page__pane" role="region" aria-label={t('editor.aria.richEditor')}>
            <div className="editor-page__pane-title">{t('editor.panes.rich')}</div>
            <div className="editor-page__pane-body">
              {isRevealDocument && activeRevealSlide ? (
                <div className="editor-page__presentation-nav" role="group" aria-label={t('editor.presentation.navAria')}>
                  <label className="editor-page__presentation-label" htmlFor={slideSelectId}>
                    {t('editor.presentation.slideLabel', {
                      current: activeRevealSlide.index + 1,
                      total: revealDeck.slides.length,
                    })}
                  </label>
                  <select
                    id={slideSelectId}
                    className="editor-page__presentation-select"
                    value={String(activeRevealSlide.index)}
                    onChange={(e) => switchRevealSlide(Number(e.target.value))}
                    disabled={isAsking}
                    aria-label={t('editor.presentation.goToSlide')}
                  >
                    {revealDeck.slides.map((slide) => (
                      <option key={slide.index} value={String(slide.index)}>
                        {t('editor.presentation.slideOption', { index: slide.index + 1 })}
                      </option>
                    ))}
                  </select>
                  <button
                    type="button"
                    className="editor-page__presentation-button"
                    onClick={createRevealSlide}
                    disabled={isAsking}
                  >
                    {t('editor.presentation.newSlide')}
                  </button>
                </div>
              ) : null}
              <RichTextEditor
                ref={richEditorHandleRef as Ref<RichTextEditorHandle>}
                ariaLabel={t('editor.richText.label')}
                markdown={activeRevealSlideEditableMarkdown ?? activeTab.markdown}
                onMarkdownChange={handleRichMarkdownChange}
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
