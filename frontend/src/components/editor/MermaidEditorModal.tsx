import { useEffect, useMemo, useState } from 'react';
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

  useEffect(() => {
    if (!isOpen) return;
    setCode(initialCode);
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
          if (e.key === 'Enter' && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey) {
            e.preventDefault();
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
