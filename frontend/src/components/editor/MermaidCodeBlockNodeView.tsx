import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from '@tiptap/react';

import { MarkdownRenderer } from '../ui/MarkdownRenderer';

type MermaidRequestEditHandler = (ctx: {
  code: string;
  apply: (nextCode: string) => void;
  remove: () => void;
}) => void;

export function MermaidCodeBlockNodeView(props: NodeViewProps) {
  const { node, editor, getPos, extension } = props;
  const language = String((node.attrs as any)?.language || '').toLowerCase();

  const requestEdit = (extension.options as any)?.onRequestEditMermaid as
    | MermaidRequestEditHandler
    | undefined;

  const apply = (nextCode: string) => {
    const pos = typeof getPos === 'function' ? (getPos() as number) : null;
    if (pos === null) return;

    const from = pos + 1;
    const to = pos + node.nodeSize - 1;

    editor.commands.command(({ tr, state }) => {
      tr.replaceWith(from, to, state.schema.text(nextCode));
      return true;
    });
  };

  const remove = () => {
    const pos = typeof getPos === 'function' ? (getPos() as number) : null;
    if (pos === null) return;

    editor.commands.command(({ tr }) => {
      tr.delete(pos, pos + node.nodeSize);
      return true;
    });
  };

  if (language === 'mermaid') {
    const code = node.textContent || '';
    const previewMarkdown = '\n\n```mermaid\n' + code + '\n```\n';

    return (
      <NodeViewWrapper className="rich-mermaid-block" role="group" aria-label="Bloco Mermaid">
        <div className="rich-mermaid-block__header">
          <div className="rich-mermaid-block__title">Mermaid</div>
          <div className="rich-mermaid-block__actions">
            <button
              type="button"
              className="rich-mermaid-block__button"
              onClick={() => requestEdit?.({ code, apply, remove })}
              aria-label="Editar diagrama Mermaid"
            >
              Editar
            </button>
            <button
              type="button"
              className="rich-mermaid-block__button rich-mermaid-block__button--danger"
              onClick={() => requestEdit?.({ code, apply, remove })}
              aria-label="Remover bloco Mermaid"
            >
              Remover
            </button>
          </div>
        </div>

        <div
          className="rich-mermaid-block__preview"
          onDoubleClick={() => requestEdit?.({ code, apply, remove })}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault();
              requestEdit?.({ code, apply, remove });
            }
          }}
          tabIndex={0}
          aria-label="Preview do Mermaid. Pressione Enter para editar"
        >
          <MarkdownRenderer content={previewMarkdown} interactiveButtons={false} focusableMermaid={false} />
        </div>

        <pre className="rich-mermaid-block__code" aria-label="Código Mermaid">
          <NodeViewContent />
        </pre>
      </NodeViewWrapper>
    );
  }

  return (
    <NodeViewWrapper as="pre" className="rich-code-block">
      <NodeViewContent />
    </NodeViewWrapper>
  );
}
