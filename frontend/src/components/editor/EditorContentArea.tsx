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
import { useRenderedContentNavigation } from '../../hooks/useRenderedContentNavigation';
import { isModalOpen } from '../ui/Modal';
import { clearRichEditorHistory } from './richEditorHistory';

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
  isPanelActive?: boolean;
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
  isPanelActive = true,
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
  const richEditorInstanceRef = useRef<TipTapEditor | null>(null);
  const pendingRevealSlideFocusRef = useRef(false);
  const renderedPaneRef = useRef<HTMLDivElement>(null);
  const renderedDocumentRef = useRef<HTMLDivElement>(null);
  const [readingDocumentKey, setReadingDocumentKey] = useState<string | null>(null);
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
  // A key é estável por aba (não inclui o índice do slide): trocar de slide NÃO
  // pode remontar o TipTap (perderia undo, seleção, IME e foco). O conteúdo do
  // novo slide chega pela prop `markdown` e é aplicado via syncFromExternal.
  const richEditorKey = activeTab ? `${activeTab.id}:document` : 'empty';
  const activeRevealSlideLabel = isRevealDocument && activeRevealSlide
    ? t('editor.presentation.slideLabel', {
        current: activeRevealSlide.index + 1,
        total: revealDeck.slides.length,
      })
    : '';

  const renderedDocumentKey = activeTab
    ? [activeTab.id, activeTab.draftId ?? '', activeTab.filePath ?? '', activeTab.mode].join('\u0000')
    : null;
  const renderedReadingActive = (
    isPanelActive
    && activeTab?.mode === 'view'
    && readingDocumentKey === renderedDocumentKey
  );

  useRenderedContentNavigation({
    elementRef: renderedPaneRef,
    isActive: renderedReadingActive,
    profile: 'scoped',
    contentSelector: '[data-editor-rendered-document="true"]',
    onEscape: () => renderedDocumentRef.current?.focus(),
    openAnnouncement: t('editor.documentView.readingOpened'),
    closeAnnouncement: t('editor.documentView.readingFocused'),
    shouldHandleEscape: () => (
      !isModalOpen()
      && document.querySelector('[role="menu"]') === null
      && document.activeElement !== renderedDocumentRef.current
    ),
    manageDocumentSemantics: false,
  });

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
    setReadingDocumentKey(null);
  }, [
    activeTab?.draftId,
    activeTab?.filePath,
    activeTab?.id,
    activeTab?.mode,
    isPanelActive,
  ]);

  useEffect(() => {
    if (activeTab?.loadError) {
      announce(t('editor.documentView.loadFailedAnnouncement'));
      return;
    }
    if (!activeTab?.readOnly || !activeTab.projection) return;
    announce(t('editor.documentView.openedAnnouncement', {
      format: activeTab.projection.format.toUpperCase(),
    }));
  }, [
    activeTab?.id,
    activeTab?.loadError,
    activeTab?.projection?.format,
    activeTab?.readOnly,
    announce,
    t,
  ]);

  useEffect(() => {
    onRevealSlideIndexChange?.(activeRevealSlide?.index ?? 0);
  }, [activeRevealSlide?.index, onRevealSlideIndexChange]);

  // Após a troca de slide, limpa o histórico de undo e (se a troca foi iniciada
  // pelo usuário) posiciona o cursor no início do novo slide, mantendo o foco no
  // editor (o foco não pode cair para o body — isso quebraria a navegação por
  // teclado). Este efeito roda depois do syncFromExternal do RichTextEditor
  // (efeitos do filho executam antes dos do pai), então o conteúdo do novo slide
  // já foi aplicado. A limpeza do histórico é obrigatória: sem remontagem, um
  // Ctrl+Z depois da troca restauraria o markdown do slide anterior e
  // handleRichMarkdownChange gravaria esse conteúdo no slide atual.
  useEffect(() => {
    clearRichEditorHistory(richEditorInstanceRef.current);
    if (!pendingRevealSlideFocusRef.current) return;
    pendingRevealSlideFocusRef.current = false;
    richEditorInstanceRef.current?.commands?.focus?.('start');
  }, [activeRevealSlide?.index]);

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
    if (revealDeck.slides.length - 1 !== activeRevealSlide?.index) {
      pendingRevealSlideFocusRef.current = true;
    }
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
    if (clampedIndex !== activeRevealSlide?.index) {
      pendingRevealSlideFocusRef.current = true;
    }
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

  const handleRichEditorReady = (editor: TipTapEditor | null) => {
    richEditorInstanceRef.current = editor;
    onRichEditorReady(editor);
  };

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
      {activeTab?.loadError ? (
        <div className="editor-page__document-status">
          <strong>{t('editor.documentView.loadFailedTitle')}</strong>
          <span>{t('editor.documentView.loadFailedDescription')}</span>
        </div>
      ) : activeTab?.readOnly && activeTab.projection ? (
        <div className="editor-page__document-status">
          <strong>
            {t('editor.documentView.readOnlyBanner', {
              format: activeTab.projection.format.toUpperCase(),
            })}
          </strong>
          {activeTab.projection.pages ? (
            <span>
              {t('editor.documentView.pages', { count: activeTab.projection.pages })}
            </span>
          ) : null}
          {activeTab.projection.warnings.length > 0 ? (
            <span>
              {activeTab.projection.warningCode === 'no_extractable_text'
                ? t('editor.documentView.noExtractableText')
                : t('editor.documentView.partialExtraction')}
            </span>
          ) : null}
        </div>
      ) : null}
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
            ref={renderedPaneRef}
            className="editor-page__pane"
            role="region"
            aria-label={t('editor.aria.preview')}
            onDoubleClick={(e) => {
              if (activeTab.readOnly) return;
              const target = e.target as HTMLElement | null;
              const wrapper = target?.closest?.('.mermaid-diagram') as HTMLElement | null;
              if (!wrapper) return;
              const raw = wrapper.dataset.mermaidIndex;
              const index = raw ? Number(raw) : NaN;
              if (!Number.isFinite(index)) return;
              onOpenMermaid(index);
            }}
            onKeyDown={(e) => {
              if (activeTab.readOnly) return;
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
              <div
                ref={renderedDocumentRef}
                data-editor-rendered-document="true"
                data-reading-active={renderedReadingActive ? 'true' : 'false'}
                role={renderedReadingActive ? 'document' : 'group'}
                aria-label={t(
                  renderedReadingActive
                    ? 'editor.documentView.readingDocumentLabel'
                    : 'editor.documentView.readingRegionLabel',
                )}
                tabIndex={0}
                onKeyDown={(event) => {
                  if (
                    !renderedReadingActive
                    && event.target === event.currentTarget
                    && event.key === 'Enter'
                  ) {
                    event.preventDefault();
                    event.stopPropagation();
                    setReadingDocumentKey(renderedDocumentKey);
                  }
                }}
              >
                {isRevealPreviewDocument ? (
                  <RevealRenderer
                    markdown={debouncedMarkdownForPreview}
                    documentTitle={activeTab.title}
                    fullscreenRequestNonce={revealFullscreenRequestNonce}
                    tabNavigation={renderedReadingActive ? 'enabled' : 'disabled'}
                  />
                ) : (
                  <>
                    {!activeTab.readOnly ? (
                      <div className="editor-page__preview-hint">{t('editor.hints.previewMermaid')}</div>
                    ) : null}
                    <MarkdownRenderer
                      content={debouncedMarkdownForPreview}
                      interactiveButtons={false}
                      focusableMermaid={!activeTab.readOnly}
                      tabNavigation={renderedReadingActive ? 'enabled' : 'disabled'}
                    />
                  </>
                )}
              </div>
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
                onEditorReady={handleRichEditorReady}
                onRequestEditMermaid={onRequestEditMermaid}
              />
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
