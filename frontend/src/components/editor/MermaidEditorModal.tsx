import { useEffect, useMemo, useRef, useState } from 'react';
import { SimpleModal } from '../ui/SimpleModal';
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
  title = 'Editar Mermaid',
  initialCode,
  initialInsertText,
  onConsumeInsertText,
  onCancel,
  onApply,
  onRemove,
}: MermaidEditorModalProps) {
  const [code, setCode] = useState(initialCode);
  const codeEditorRef = useRef<any>(null);
  const monacoRef = useRef<any>(null);

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
        const fullRange = model.getFullModelRange?.();
        if (fullRange) editor.setSelection?.(fullRange);
      } catch {
        // best-effort
      }
    });
  }, [isOpen, initialCode]);

  useEffect(() => {
    if (!isOpen) return;
    const insert = (initialInsertText || '').toString();
    if (!insert) return;
    setCode((prev) => prev + insert);
    onConsumeInsertText?.();
  }, [isOpen, initialInsertText, onConsumeInsertText]);

  const previewMarkdown = useMemo(() => {
    return `\n\n\`\`\`mermaid\n${code}\n\`\`\`\n`;
  }, [code]);

  return (
    <SimpleModal isOpen={isOpen} onClose={onCancel} title={title} size="xl" returnFocusToGrid={false}>
      <div
        className="mermaid-editor-modal"
        onKeyDown={(e) => {
          // Esc já é tratado pelo SimpleModal (onClose), mas mantemos aqui
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
        <div className="mermaid-editor-modal__split" role="group" aria-label="Editor e preview do Mermaid">
          <div className="mermaid-editor-modal__pane" role="region" aria-label="Código Mermaid">
            <div className="mermaid-editor-modal__pane-title">Código</div>
            <div className="mermaid-editor-modal__pane-body">
              <CodeEditor
                height="100%"
                language="markdown"
                ariaLabel="Código Mermaid"
                value={code}
                onChange={setCode}
                onMount={(editor, monaco) => {
                  codeEditorRef.current = editor;
                  monacoRef.current = monaco;
                }}
              />
            </div>
          </div>

          <div className="mermaid-editor-modal__pane" role="region" aria-label="Preview Mermaid">
            <div className="mermaid-editor-modal__pane-title">Preview</div>
            <div className="mermaid-editor-modal__preview">
              <MarkdownRenderer content={previewMarkdown} interactiveButtons={false} focusableMermaid={false} />
            </div>
          </div>
        </div>

        <div className="mermaid-editor-modal__actions">
          {onRemove && (
            <button type="button" className="mermaid-editor-modal__danger" onClick={onRemove}>
              Remover bloco
            </button>
          )}

          <div className="mermaid-editor-modal__actions-right">
            <button type="button" className="mermaid-editor-modal__secondary" onClick={onCancel}>
              Cancelar
            </button>
            <button type="button" className="mermaid-editor-modal__primary" onClick={() => onApply(code)}>
              Aplicar (Ctrl+Enter)
            </button>
          </div>
        </div>
      </div>
    </SimpleModal>
  );
}
