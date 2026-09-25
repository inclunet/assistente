import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal, useModalId, useModalIsTopmost } from '../ui/Modal';
import { DialogActions } from '../ui/DialogActions';
import { CodeEditor } from '../ui/CodeEditor';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import type { CommandShortcut } from '../../lib/commandShortcut';
import './MermaidEditorModal.css';

export interface MermaidEditorModalProps {
  isOpen: boolean;
  title?: string;
  initialCode: string;
  initialInsertText?: string;
  onConsumeInsertText?: () => void;
  onCancel: () => void;
  commandOwnerKey: string;
  onDraftChange: (code: string) => void;
  onCommand: (action: 'apply' | 'remove', code: string, shortcut?: CommandShortcut) => void;
  onModalIdChange: (id: string | null) => void;
  canRemove?: boolean;
}

function MermaidScopeContent({ children, ownerKey, onScope }: {
  children: ReactNode;
  ownerKey: string;
  onScope: (id: string | null, topmost: (() => boolean) | null) => void;
}) {
  const id = useModalId();
  const topmost = useModalIsTopmost();
  useLayoutEffect(() => {
    onScope(id, topmost);
    return () => onScope(null, null);
  }, [id, topmost, ownerKey, onScope]);
  return <>{children}</>;
}

export function MermaidEditorModal({
  isOpen,
  title,
  initialCode,
  initialInsertText,
  onConsumeInsertText,
  onCancel,
  commandOwnerKey,
  onDraftChange,
  onCommand,
  onModalIdChange,
  canRemove,
}: MermaidEditorModalProps) {
  const { t } = useTranslation();
  const [code, setCode] = useState(initialCode);
  const modalTitle = title ?? t('editor.mermaid.editorTitle');
  type MonacoRange = { startLineNumber: number; startColumn: number; endLineNumber: number; endColumn: number };
  type MonacoSelectionCtor = new (startLineNumber: number, startColumn: number, endLineNumber: number, endColumn: number) => unknown;
  type MermaidEditorApi = {
    getModel?: () => {
      getLineCount?: () => number;
      getLineMaxColumn?: (lineNumber: number) => number;
      getFullModelRange?: () => MonacoRange | null;
    } | null;
    getSelection?: () => MonacoRange | null;
    executeEdits?: (source: string, edits: Array<{ range: MonacoRange; text: string; forceMoveMarkers?: boolean }>) => void;
    focus?: () => void;
    setPosition?: (pos: { lineNumber: number; column: number }) => void;
    revealPositionInCenterIfOutsideViewport?: (pos: { lineNumber: number; column: number }) => void;
    setSelection?: (range: MonacoRange) => void;
    getPosition?: () => { lineNumber: number; column: number } | null;
  };
  type MonacoApi = { Selection?: MonacoSelectionCtor };

  const codeEditorRef = useRef<MermaidEditorApi | null>(null);
  const monacoRef = useRef<MonacoApi | null>(null);
  const [editorReady, setEditorReady] = useState(0);
  const opening = useMemo(() => ({ cancelled: false }), [isOpen, initialCode, commandOwnerKey]);
  const openingRef = useRef(opening);
  openingRef.current = opening;
  const editorOpeningRef = useRef<object | null>(null);
  const rootRef = useRef<HTMLDivElement>(null);
  const generationRef = useRef(0);
  const consumedInsertRef = useRef(false);
  const consumeRef = useRef(onConsumeInsertText);
  consumeRef.current = onConsumeInsertText;
  const draftRef = useRef({ code, onDraftChange });
  draftRef.current = { code, onDraftChange };
  const scopeRef = useRef<(() => boolean) | null>(null);
  const onScope = useCallback((id: string | null, topmost: (() => boolean) | null) => {
    scopeRef.current = id ? topmost : null;
    onModalIdChange(id);
  }, [onModalIdChange]);
  const ownsModal = () => scopeRef.current?.() === true;
  const cancel = () => {
    opening.cancelled = true;
    generationRef.current++;
    onCancel();
  };

  useLayoutEffect(() => {
    opening.cancelled = false;
    generationRef.current++;
    consumedInsertRef.current = false;
    return () => { opening.cancelled = true; generationRef.current++; };
  }, [opening]);

  useLayoutEffect(() => {
    if (!isOpen) return;
    setCode(initialCode);
  }, [isOpen, initialCode, commandOwnerKey]);

  useEffect(() => {
    if (!isOpen) return;

    // Foco previsível: coloca o cursor no editor e seleciona tudo.
    // (Regras do plano: ao abrir, foco vai para o editor de código; selecionar todo o código.)
    const generation = generationRef.current;
    const capturedEditor = codeEditorRef.current;
    if (!capturedEditor || editorOpeningRef.current !== openingRef.current) return;
    let active = true;
    const frame = requestAnimationFrame(() => {
      if (!active || generation !== generationRef.current || !ownsModal() || codeEditorRef.current !== capturedEditor) return;
      try {
        const editor = codeEditorRef.current;
        const model = editor?.getModel?.();
        if (!editor || !model) return;
        editor.focus?.();
        const insert = (initialInsertText || '').toString();

        // Se o modal abriu por "type-to-edit" (insertText), não seleciona tudo
        // para evitar substituir o código inteiro ao inserir o primeiro caractere.
        if (insert) {
          const lastLine = model.getLineCount?.() || 1;
          const lastCol = (model.getLineMaxColumn?.(lastLine) || 1);
          editor.setPosition?.({ lineNumber: lastLine, column: lastCol });
          editor.revealPositionInCenterIfOutsideViewport?.({ lineNumber: lastLine, column: lastCol });
          return;
        }

        const fullRange = model.getFullModelRange?.();
        if (fullRange) editor.setSelection?.(fullRange);
      } catch {
        // best-effort
      }
    });
    return () => { active = false; cancelAnimationFrame(frame); };
  }, [isOpen, initialCode, initialInsertText, commandOwnerKey, editorReady]);

  useEffect(() => {
    if (!isOpen) return;
    const insert = (initialInsertText || '').toString();
    if (!insert) return;
    const generation = generationRef.current;
    const capturedEditor = codeEditorRef.current;
    if (!capturedEditor || editorOpeningRef.current !== openingRef.current) return;
    let active = true;
    const frame = requestAnimationFrame(() => {
      if (!active || generation !== generationRef.current || consumedInsertRef.current || !ownsModal() || codeEditorRef.current !== capturedEditor) return;
      consumedInsertRef.current = true;
      // Inserção real no editor (respeita cursor/undo). Fallback para estado.
      try {
        const editor = codeEditorRef.current;
        const model = editor?.getModel?.();
        const selection = editor?.getSelection?.();
        if (editor && model && selection && editor.executeEdits) {
          const monaco = monacoRef.current;
          editor.executeEdits?.('mermaid-insert', [
            {
              range: selection,
              text: insert,
              forceMoveMarkers: true,
            },
          ]);
          // Colapsa cursor no fim da inserção.
          const pos = editor.getPosition?.();
          if (pos && monaco?.Selection) {
            editor.setSelection?.(new monaco.Selection(pos.lineNumber, pos.column, pos.lineNumber, pos.column) as MonacoRange);
          }
        } else {
          const next = draftRef.current.code + insert;
          setCode(next);
          draftRef.current.onDraftChange(next);
        }
      } catch {
        // Não repetir uma edição que pode ter sido aplicada antes da exceção.
      }
      consumeRef.current?.();
    });
    return () => { active = false; cancelAnimationFrame(frame); };
  }, [isOpen, initialCode, initialInsertText, commandOwnerKey, editorReady]);

  const previewMarkdown = useMemo(() => {
    return `\n\n\`\`\`mermaid\n${code}\n\`\`\`\n`;
  }, [code]);

  return (
    <Modal isOpen={isOpen} onClose={cancel} title={modalTitle} size="xl" returnFocusOnClose={false}>
      <MermaidScopeContent ownerKey={commandOwnerKey} onScope={onScope}>
      <div
        ref={rootRef}
        data-mermaid-command-scope={commandOwnerKey}
        className="mermaid-editor-modal"
        onKeyDown={(e) => {
          const isSave = (e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key.toLowerCase() === 's' || e.key === 'Enter' && e.ctrlKey && !e.metaKey);
          if (isSave) e.stopPropagation();
          if (!isOpen || !ownsModal() || e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || e.keyCode === 229 || e.getModifierState('AltGraph') || e.ctrlKey && e.metaKey) return;
          // Esc já é tratado pelo Modal (onClose), mas mantemos aqui
          // para garantir que o editor não capture/propague teclas.
          if (e.key === 'Escape') {
            e.stopPropagation();
            return;
          }

          // Ctrl+S / Cmd+S: aplicar (opcional no plano, mas útil no modal)
          if ((e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey && (e.key === 's' || e.key === 'S')) {
            e.preventDefault();
            e.stopPropagation();
            onCommand('apply', code, { version: 1, code: 'KeyS', modifiers: e.metaKey ? ['Meta'] : ['Control'] });
            return;
          }

          if (e.key === 'Enter' && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey) {
            e.preventDefault();
            e.stopPropagation();
            onCommand('apply', code, { version: 1, code: 'Enter', modifiers: ['Control'] });
          }
        }}
      >
        <div className="mermaid-editor-modal__split" role="group" aria-label={t('editor.mermaid.editorPreview')}>
          <div className="mermaid-editor-modal__pane" role="region" aria-label={t('editor.mermaid.codeLabel')}>
            <div className="mermaid-editor-modal__pane-title">{t('editor.mermaid.code')}</div>
            <div className="mermaid-editor-modal__pane-body">
              <CodeEditor
                key={JSON.stringify([commandOwnerKey, initialCode])}
                height="100%"
                language="markdown"
                ariaLabel={t('editor.mermaid.codeLabel')}
                value={code}
                onChange={(value) => { setCode(value); onDraftChange(value); }}
                onMount={(editor, monaco) => {
                  if (!isOpen || openingRef.current !== opening || opening.cancelled || codeEditorRef.current === editor && editorOpeningRef.current === opening) return;
                  editorOpeningRef.current = opening;
                  codeEditorRef.current = editor as unknown as MermaidEditorApi;
                  monacoRef.current = monaco as MonacoApi;
                  setEditorReady(value => value + 1);
                }}
              />
            </div>
          </div>

          <div className="mermaid-editor-modal__pane" role="region" aria-label={t('editor.mermaid.preview')}>
            <div className="mermaid-editor-modal__pane-title">{t('editor.mermaid.preview')}</div>
            <div className="mermaid-editor-modal__preview">
              <MarkdownRenderer
                content={previewMarkdown}
                interactiveButtons={false}
                tabNavigation="disabled"
              />
            </div>
          </div>
        </div>

        <div className="mermaid-editor-modal__actions">
          {canRemove && (
            <button type="button" className="mermaid-editor-modal__danger" onClick={() => { if (isOpen && ownsModal()) onCommand('remove', code); }}>
              {t('editor.mermaid.removeBlock')}
            </button>
          )}

          <DialogActions
            className="mermaid-editor-modal__actions-right"
            primary={
              <button type="button" className="mermaid-editor-modal__primary" onClick={() => { if (isOpen && ownsModal()) onCommand('apply', code); }}>
                {t('editor.mermaid.applyShortcut')}
              </button>
            }
            secondary={
              <button type="button" className="mermaid-editor-modal__secondary" onClick={cancel}>
                {t('common.cancel')}
              </button>
            }
          />
        </div>
      </div>
      </MermaidScopeContent>
    </Modal>
  );
}
