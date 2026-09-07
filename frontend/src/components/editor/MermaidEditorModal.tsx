import { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '../ui/Modal';
import { DialogActions } from '../ui/DialogActions';
import { CodeEditor } from '../ui/CodeEditor';
import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import './MermaidEditorModal.css';

export interface MermaidEditorModalProps {
  isOpen: boolean;
  title?: string;
  initialCode: string;
  initialInsertText?: string;
  onConsumeInsertText?: () => void;
  onCancel: () => void;
  onApply: (code: string) => void;
  onRemove?: () => void;
}

export function MermaidEditorModal({
  isOpen,
  title,
  initialCode,
  initialInsertText,
  onConsumeInsertText,
  onCancel,
  onApply,
  onRemove,
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

  useEffect(() => {
    if (!isOpen) return;
    setCode(initialCode);
  }, [isOpen, initialCode]);

  useEffect(() => {
    if (!isOpen) return;

    // Foco previsível: coloca o cursor no editor e seleciona tudo.
    // (Regras do plano: ao abrir, foco vai para o editor de código; selecionar todo o código.)
    requestAnimationFrame(() => {
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
  }, [isOpen, initialCode, initialInsertText]);

  useEffect(() => {
    if (!isOpen) return;
    const insert = (initialInsertText || '').toString();
    if (!insert) return;
    requestAnimationFrame(() => {
      // Inserção real no editor (respeita cursor/undo). Fallback para estado.
      try {
        const editor = codeEditorRef.current;
        const model = editor?.getModel?.();
        const selection = editor?.getSelection?.();
        if (editor && model && selection) {
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
          setCode((prev) => prev + insert);
        }
      } catch {
        setCode((prev) => prev + insert);
      }
      onConsumeInsertText?.();
    });
  }, [isOpen, initialInsertText, onConsumeInsertText]);

  const previewMarkdown = useMemo(() => {
    return `\n\n\`\`\`mermaid\n${code}\n\`\`\`\n`;
  }, [code]);

  return (
    <Modal isOpen={isOpen} onClose={onCancel} title={modalTitle} size="xl" returnFocusOnClose={false}>
      <div
        className="mermaid-editor-modal"
        onKeyDown={(e) => {
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
            onApply(code);
            return;
          }

          if (e.key === 'Enter' && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey) {
            e.preventDefault();
            e.stopPropagation();
            onApply(code);
          }
        }}
      >
        <div className="mermaid-editor-modal__split" role="group" aria-label={t('editor.mermaid.editorPreview')}>
          <div className="mermaid-editor-modal__pane" role="region" aria-label={t('editor.mermaid.codeLabel')}>
            <div className="mermaid-editor-modal__pane-title">{t('editor.mermaid.code')}</div>
            <div className="mermaid-editor-modal__pane-body">
              <CodeEditor
                height="100%"
                language="markdown"
                ariaLabel={t('editor.mermaid.codeLabel')}
                value={code}
                onChange={setCode}
                onMount={(editor, monaco) => {
                  codeEditorRef.current = editor as unknown as MermaidEditorApi;
                  monacoRef.current = monaco as MonacoApi;
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
          {onRemove && (
            <button type="button" className="mermaid-editor-modal__danger" onClick={onRemove}>
              {t('editor.mermaid.removeBlock')}
            </button>
          )}

          <DialogActions
            className="mermaid-editor-modal__actions-right"
            primary={
              <button type="button" className="mermaid-editor-modal__primary" onClick={() => onApply(code)}>
                {t('editor.mermaid.applyShortcut')}
              </button>
            }
            secondary={
              <button type="button" className="mermaid-editor-modal__secondary" onClick={onCancel}>
                {t('common.cancel')}
              </button>
            }
          />
        </div>
      </div>
    </Modal>
  );
}
