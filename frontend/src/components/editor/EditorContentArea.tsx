import { type Ref, type RefObject, useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { CodeEditor, type CodeEditorProps } from '../ui/CodeEditor';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { RichTextEditor, type RichTextEditorHandle } from './RichTextEditor';
import { RevealRenderer } from './RevealRenderer';
import { useEditorStore, type EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';
import {
  getRevealSlideEditableMarkdown,
  mergeRevealSlideEditableMarkdown,
  parseRevealMarkdown,
  replaceRevealSlide,
} from '../../lib/revealMarkdown';
import {
  getMarkdownFenceMarker,
  isClosingMarkdownFence,
  type MarkdownFenceMarker,
} from '../../lib/markdownFence';
import { useAnnouncer } from '../../hooks/useAnnouncer';

const RAW_HTML_RE = /<\/?[a-z][a-z0-9-]*(?:\s|>|\/>)/i;

function hasRawHtmlOutsideFences(markdown: string): boolean {
  const lines = String(markdown || '').split(/\r?\n/);
  let fence: MarkdownFenceMarker | null = null;

  for (const line of lines) {
    if (fence) {
      if (isClosingMarkdownFence(line, fence)) fence = null;
      continue;
    }

    const nextFence = getMarkdownFenceMarker(line);
    if (nextFence) {
      fence = nextFence;
      continue;
    }

    if (RAW_HTML_RE.test(line)) return true;
  }

  return false;
}

function normalizeRevealEditableMarkdown(markdown: string): string {
  const text = String(markdown || '');
  const lines = text.match(/[^\n]*(?:\n|$)/g) ?? [''];
  const normalizedLines = lines.filter((line, index) => !(line === '' && index === lines.length - 1));
  let fence: MarkdownFenceMarker | null = null;

  return normalizedLines
    .map((line) => {
      const lineWithoutNewline = line.replace(/\r?\n$/, '');
      const newline = line.match(/\r?\n$/)?.[0] ?? '';

      if (fence) {
        if (isClosingMarkdownFence(lineWithoutNewline, fence)) fence = null;
        return line;
      }

      const nextFence = getMarkdownFenceMarker(lineWithoutNewline);
      if (nextFence) {
        fence = nextFence;
        return line;
      }

      const separatorMatch = lineWithoutNewline.match(/^(\s*)-{3,}\s*$/);
      if (!separatorMatch) return line;
      return `${separatorMatch[1] || ''}___${newline}`;
    })
    .join('');
}

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
  onRevealSlideIndexChange?: (index: number) => void;
  revealAppendNonce?: number;
  revealSlideNavigationRequest?: { index: number; nonce: number } | null;
  revealFullscreenRequestNonce?: number;
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
  onRevealSlideIndexChange,
  revealAppendNonce = 0,
  revealSlideNavigationRequest = null,
  revealFullscreenRequestNonce = 0,
  richEditorHandleRef,
  onRequestEditMermaid,
  onOpenMermaid,
  onRemoveMermaid,
}: EditorContentAreaProps) {
  const { t } = useTranslation();
  const { announce } = useAnnouncer();
  const [activeRevealSlideIndex, setActiveRevealSlideIndex] = useState(0);
  const lastRevealAppendNonceRef = useRef(revealAppendNonce);
  const lastRevealSlideNavigationNonceRef = useRef(revealSlideNavigationRequest?.nonce ?? 0);
  const previousRevealSlideAnnouncementRef = useRef<string | null>(null);
  const revealDeck = useMemo(
    () => parseRevealMarkdown(activeTab?.markdown || ''),
    [activeTab?.markdown]
  );
  const previewRevealDeck = useMemo(
    () => parseRevealMarkdown(debouncedMarkdownForPreview || ''),
    [debouncedMarkdownForPreview]
  );
  const isRevealDocument = revealDeck.detection.kind === 'reveal' && revealDeck.slides.length > 0;
  const isRevealPreviewDocument = previewRevealDeck.detection.kind === 'reveal' && previewRevealDeck.slides.length > 0;
  const activeRevealSlide = isRevealDocument
    ? revealDeck.slides[Math.min(activeRevealSlideIndex, revealDeck.slides.length - 1)] ?? revealDeck.slides[0]
    : null;
  const activeRevealSlideEditableMarkdown = activeRevealSlide
    ? getRevealSlideEditableMarkdown(activeRevealSlide.markdown)
    : null;
  const activeRevealSlideHasRawHtml = activeRevealSlideEditableMarkdown
    ? hasRawHtmlOutsideFences(activeRevealSlideEditableMarkdown)
    : false;
  const richEditorKey = activeTab
    ? isRevealDocument && activeRevealSlide
      ? `${activeTab.id}:reveal-slide:${activeRevealSlide.index}`
      : `${activeTab.id}:document`
    : 'empty';
  const activeRevealSlideLabel = isRevealDocument && activeRevealSlide
    ? t('editor.presentation.slideLabel', {
        current: activeRevealSlide.index + 1,
        total: revealDeck.slides.length,
      })
    : '';

  const getLatestMarkdown = () => {
    if (!activeTab) return '';
    return useEditorStore.getState().getDocument(activeTab.id)?.markdown ?? activeTab.markdown;
  };

  const getLatestSlideForCurrentIndex = (markdown: string) => {
    if (!activeRevealSlide) return null;
    const latestDeck = parseRevealMarkdown(markdown);
    if (latestDeck.slides.length === 0) return null;
    return latestDeck.slides[Math.min(activeRevealSlide.index, latestDeck.slides.length - 1)] ?? null;
  };

  useEffect(() => {
    setActiveRevealSlideIndex(0);
    lastRevealAppendNonceRef.current = revealAppendNonce;
    lastRevealSlideNavigationNonceRef.current = revealSlideNavigationRequest?.nonce ?? 0;
  }, [activeTab?.id]);

  useEffect(() => {
    onRevealSlideIndexChange?.(activeRevealSlide?.index ?? 0);
  }, [activeRevealSlide?.index, onRevealSlideIndexChange]);

  useEffect(() => {
    if (!activeRevealSlideLabel) {
      previousRevealSlideAnnouncementRef.current = null;
      return;
    }
    if (previousRevealSlideAnnouncementRef.current === null) {
      previousRevealSlideAnnouncementRef.current = activeRevealSlideLabel;
      return;
    }
    if (previousRevealSlideAnnouncementRef.current === activeRevealSlideLabel) return;

    previousRevealSlideAnnouncementRef.current = activeRevealSlideLabel;
    announce(activeRevealSlideLabel);
  }, [activeRevealSlideLabel, announce]);

  useEffect(() => {
    if (!isRevealDocument) {
      setActiveRevealSlideIndex(0);
      return;
    }
    if (activeRevealSlideIndex >= revealDeck.slides.length) {
      setActiveRevealSlideIndex(Math.max(0, revealDeck.slides.length - 1));
    }
  }, [activeRevealSlideIndex, isRevealDocument, revealDeck.slides.length]);

  useEffect(() => {
    if (revealAppendNonce === lastRevealAppendNonceRef.current) return;
    if (!isRevealDocument || activeTab?.mode !== 'rich' || revealDeck.slides.length === 0) return;
    lastRevealAppendNonceRef.current = revealAppendNonce;
    setActiveRevealSlideIndex(revealDeck.slides.length - 1);
  }, [activeTab?.mode, isRevealDocument, revealAppendNonce, revealDeck.slides.length]);

  const getMarkdownWithCurrentRevealSlide = () => {
    if (!activeTab || !activeRevealSlide) return activeTab?.markdown || null;
    if (activeRevealSlideHasRawHtml) return getLatestMarkdown();
    const richEditorHandle = richEditorHandleRef.current;
    richEditorHandle?.flushMarkdown?.();
    const currentEditableMarkdown = richEditorHandle?.getMarkdown?.();
    const baseMarkdown = getLatestMarkdown();
    const slideForBase = getLatestSlideForCurrentIndex(baseMarkdown);
    if (typeof currentEditableMarkdown !== 'string') return baseMarkdown;
    const normalizedEditableMarkdown = normalizeRevealEditableMarkdown(currentEditableMarkdown);
    if (!slideForBase) return baseMarkdown;
    return replaceRevealSlide(
      baseMarkdown,
      slideForBase,
      mergeRevealSlideEditableMarkdown(slideForBase.markdown, normalizedEditableMarkdown)
    );
  };

  const switchRevealSlide = (nextIndex: number) => {
    const clampedIndex = Math.max(0, Math.min(nextIndex, revealDeck.slides.length - 1));
    const nextMarkdown = getMarkdownWithCurrentRevealSlide();
    if (nextMarkdown === null) {
      setActiveRevealSlideIndex(clampedIndex);
      return;
    }
    if (nextMarkdown !== activeTab?.markdown) {
      onRichMarkdownChange(nextMarkdown);
    }
    setActiveRevealSlideIndex(clampedIndex);
  };

  useEffect(() => {
    if (!revealSlideNavigationRequest) return;
    if (revealSlideNavigationRequest.nonce === lastRevealSlideNavigationNonceRef.current) return;
    lastRevealSlideNavigationNonceRef.current = revealSlideNavigationRequest.nonce;
    if (!isRevealDocument || revealDeck.slides.length === 0) return;
    switchRevealSlide(revealSlideNavigationRequest.index);
  }, [isRevealDocument, revealDeck.slides.length, revealSlideNavigationRequest]);

  const handleRichMarkdownChange = (markdown: string) => {
    if (!activeTab) return;
    if (!activeRevealSlide) {
      if (!isRevealDocument) {
        onRichMarkdownChange(markdown);
      }
      return;
    }
    if (!isRevealDocument) {
      onRichMarkdownChange(markdown);
      return;
    }
    if (activeRevealSlideHasRawHtml) return;
    const baseMarkdown = getLatestMarkdown();
    const slideForBase = getLatestSlideForCurrentIndex(baseMarkdown);
    const normalizedMarkdown = normalizeRevealEditableMarkdown(markdown);
    if (!slideForBase) {
      onRichMarkdownChange(baseMarkdown);
      return;
    }
    onRichMarkdownChange(
      replaceRevealSlide(
        baseMarkdown,
        slideForBase,
        mergeRevealSlideEditableMarkdown(slideForBase.markdown, normalizedMarkdown)
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
              {isRevealPreviewDocument ? (
                <RevealRenderer
                  markdown={debouncedMarkdownForPreview}
                  documentTitle={activeTab.title}
                  fullscreenRequestNonce={revealFullscreenRequestNonce}
                />
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
                <div className="editor-page__presentation-current">
                  {activeRevealSlideLabel}
                </div>
              ) : null}
              <RichTextEditor
                key={richEditorKey}
                ref={richEditorHandleRef as Ref<RichTextEditorHandle>}
                ariaLabel={t('editor.richText.label')}
                markdown={activeRevealSlideEditableMarkdown ?? activeTab.markdown}
                onMarkdownChange={handleRichMarkdownChange}
                readOnly={isAsking || activeRevealSlideHasRawHtml}
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
