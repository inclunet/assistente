import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from '@tiptap/react';
import { useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';

import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { useQuestionnaireUIStore } from '../../store/questionnaireUIStore';
import { useUIStore } from '../../store/uiStore';

type MermaidRequestEditHandler = (ctx: {
  mermaidBlockId: string;
  code: string;
  insertText?: string;
  apply: (nextCode: string) => void;
  remove: () => void;
}) => void;

function newMermaidBlockId(): string {
  try {
    return crypto.randomUUID();
  } catch {
    return `mermaid-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }
}

export function MermaidCodeBlockNodeView(props: NodeViewProps) {
  const { t } = useTranslation();
  const { node, editor, getPos, extension } = props;
  const attrs = node.attrs as Record<string, unknown>;
  const language = String((attrs?.language as string | undefined) || '').toLowerCase();

  const mermaidBlockId = useMemo(() => {
    return String((attrs?.mermaidBlockId as string | undefined) || '').trim();
  }, [attrs]);

  useEffect(() => {
    if (language !== 'mermaid') return;
    const cur = String((attrs?.mermaidBlockId as string | undefined) || '').trim();
    if (cur) return;

    const pos = typeof getPos === 'function' ? (getPos() as number) : null;
    if (pos === null) return;

    const nextId = newMermaidBlockId();
    try {
      editor.commands.command(({ tr }) => {
        const nextAttrs = { ...attrs, mermaidBlockId: nextId };
        tr.setNodeMarkup(pos, undefined, nextAttrs);
        return true;
      });
    } catch {
      // best-effort
    }
  }, [language, attrs, editor, getPos]);

  const requestQuestionnaire = useQuestionnaireUIStore((s) => s.request);
  const addToast = useUIStore((s) => s.addToast);

  const requestEdit = (extension.options as { onRequestEditMermaid?: MermaidRequestEditHandler })?.onRequestEditMermaid as
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

  const confirmRemove = async () => {
    const resp = await requestQuestionnaire({
      id: `ui-rich-mermaid-remove-${Date.now()}`,
      title: t('editor.mermaid.removeConfirmTitle'),
      description: t('editor.mermaid.removeConfirmMessage'),
      submitLabel: t('editor.mermaid.removeBtn'),
      cancelLabel: t('common.cancel'),
      allowCancel: true,
      questions: [
        {
          id: 'note',
          type: 'readonly_code',
          prompt: t('editor.mermaid.removeHint'),
          content: t('editor.mermaid.removeHint'),
        },
      ],
    });

    if (resp.cancelled) return;
    remove();
    addToast(t('editor.mermaid.blockRemoved'), 'success');
  };

  if (language === 'mermaid') {
    const code = node.textContent || '';
    const previewMarkdown = '\n\n```mermaid\n' + code + '\n```\n';

    const ensuredId = mermaidBlockId
      || String((attrs?.mermaidBlockId as string | undefined) || '').trim()
      || 'mermaid-unknown';

    return (
      <NodeViewWrapper className="rich-mermaid-block" role="group" aria-label={t('editor.mermaid.blockLabel', 'Bloco Mermaid')}>
        <div className="rich-mermaid-block__header">
          <div className="rich-mermaid-block__title">{t('editor.mermaid.title', 'Mermaid')}</div>
          <div className="rich-mermaid-block__actions">
            <button
              type="button"
              className="rich-mermaid-block__button"
              onClick={() => requestEdit?.({ mermaidBlockId: ensuredId, code, apply, remove })}
              aria-label={t('editor.mermaid.editDiagram')}
            >
              {t('editor.mermaid.editBtn')}
            </button>
            <button
              type="button"
              className="rich-mermaid-block__button rich-mermaid-block__button--danger"
              onClick={() => void confirmRemove()}
              aria-label={t('editor.mermaid.removeBtnLabel')}
            >
              {t('editor.mermaid.removeBtn')}
            </button>
          </div>
        </div>

        <div
          className="rich-mermaid-block__preview"
          onDoubleClick={() => requestEdit?.({ mermaidBlockId: ensuredId, code, apply, remove })}
          onKeyDown={(e) => {
            // Enter/F2: editar
            if (e.key === 'Enter' || e.key === 'F2') {
              e.preventDefault();
              e.stopPropagation();
              requestEdit?.({ mermaidBlockId: ensuredId, code, apply, remove });
              return;
            }

            // Backspace/Delete: confirmar remoção
            if (e.key === 'Backspace' || e.key === 'Delete') {
              // Shift+Delete: remove sem confirmação (power-user, opcional no plano)
              if (e.key === 'Delete' && e.shiftKey) {
                e.preventDefault();
                e.stopPropagation();
                remove();
                addToast(t('editor.mermaid.blockRemoved'), 'success');
                return;
              }
              e.preventDefault();
              e.stopPropagation();
              void confirmRemove();
              return;
            }

            // Digitação “em cima do diagrama”: abre editor e injeta o 1º caractere
            if (
              e.key &&
              e.key.length === 1 &&
              !e.ctrlKey &&
              !e.metaKey &&
              !e.altKey
            ) {
              e.preventDefault();
              e.stopPropagation();
              requestEdit?.({ mermaidBlockId: ensuredId, code, apply, remove, insertText: e.key });
            }
          }}
          tabIndex={0}
          aria-label={t('editor.mermaid.previewLabel')}
        >
          <MarkdownRenderer
            content={previewMarkdown}
            interactiveButtons={false}
            focusableMermaid={false}
            tabNavigation="disabled"
          />
        </div>

        <pre className="rich-mermaid-block__code" aria-label={t('editor.mermaid.codeLabel')}>
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
