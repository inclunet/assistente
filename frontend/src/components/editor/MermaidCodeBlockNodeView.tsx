import { NodeViewContent, NodeViewWrapper, type NodeViewProps } from '@tiptap/react';
import { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

import { MarkdownRenderer } from '../ui/MarkdownRenderer';
import { requestEditorMermaidCommand } from '../../lib/commandEditorMermaid';
import { isModalOpen } from '../../lib/modalRegistry';
import { ReadFocusContext } from '../../lib/commandContextProviders';

type MermaidRequestEditHandler = (ctx: {
  mermaidBlockId: string;
  code: string;
  insertText?: string;
  expectedEditor?: object;
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
  const [editable, setEditable] = useState(() => editor.isEditable);

  useEffect(() => {
    const refreshEditable = () => setEditable(editor.isEditable);
    // setEditable emits update, not transaction; a NodeView is not otherwise
    // rerendered when only this editor option changes.
    editor.on('update', refreshEditable);
    refreshEditable();
    return () => { editor.off('update', refreshEditable); };
  }, [editor]);

  const mermaidBlockId = useMemo(() => {
    return String((attrs?.mermaidBlockId as string | undefined) || '').trim();
  }, [attrs]);

  useEffect(() => {
    if (language !== 'mermaid' || !editor.isEditable || editor.isDestroyed) return;
    const cur = String((attrs?.mermaidBlockId as string | undefined) || '').trim();
    if (cur) return;

    const pos = typeof getPos === 'function' ? (getPos() as number) : null;
    if (typeof pos !== 'number') return;
    const liveNode = editor.state.doc.nodeAt(pos);
    if (!liveNode || liveNode.type !== node.type || liveNode.attrs.mermaidBlockId) return;

    const nextId = newMermaidBlockId();
    try {
      editor.commands.command(({ tr }) => {
        const currentNode = tr.doc.nodeAt(pos);
        if (!currentNode || currentNode.type !== node.type || currentNode.attrs.mermaidBlockId) return false;
        const nextAttrs = { ...currentNode.attrs, mermaidBlockId: nextId };
        tr.setNodeMarkup(pos, undefined, nextAttrs);
        return true;
      });
    } catch {
      // best-effort
    }
  }, [language, attrs, node.type, editor, editable, getPos]);

  const requestEdit = (extension.options as { onRequestEditMermaid?: MermaidRequestEditHandler })?.onRequestEditMermaid as
    | MermaidRequestEditHandler
    | undefined;

  const canRequest = () => !!mermaidBlockId && !editor.isDestroyed && editor.isEditable && !editor.view.composing &&
    !isModalOpen() && ReadFocusContext().composition !== 'active';
  const open = (insertText?: string) => {
    if (!canRequest()) return;
    requestEdit?.({ mermaidBlockId, expectedEditor: editor, code: node.textContent, ...(insertText === undefined ? {} : { insertText }) });
  };
  const remove = () => {
    if (!canRequest()) return;
    requestEditorMermaidCommand('editor.mermaid.remove', { mermaidBlockId, expectedEditor: editor });
  };

  if (language === 'mermaid') {
    const code = node.textContent || '';
    const previewMarkdown = '\n\n```mermaid\n' + code + '\n```\n';

    return (
      <NodeViewWrapper className="rich-mermaid-block" role="group" aria-label={t('editor.mermaid.blockLabel', 'Bloco Mermaid')}>
        <div className="rich-mermaid-block__header">
          <div className="rich-mermaid-block__title">{t('editor.mermaid.title', 'Mermaid')}</div>
          <div className="rich-mermaid-block__actions">
            <button
              type="button"
              className="rich-mermaid-block__button"
              onClick={() => open()}
              disabled={!editor.isEditable || !mermaidBlockId}
              aria-label={t('editor.mermaid.editDiagram')}
            >
              {t('editor.mermaid.editBtn')}
            </button>
            <button
              type="button"
              className="rich-mermaid-block__button rich-mermaid-block__button--danger"
              onClick={remove}
              disabled={!editor.isEditable || !mermaidBlockId}
              aria-label={t('editor.mermaid.removeBtnLabel')}
            >
              {t('editor.mermaid.removeBtn')}
            </button>
          </div>
        </div>

        <div
          className="rich-mermaid-block__preview"
          onDoubleClick={() => open()}
          onKeyDown={(e) => {
            if (e.defaultPrevented || e.repeat || e.nativeEvent.isComposing || e.keyCode === 229 || e.getModifierState('AltGraph') || !canRequest()) return;
            // Enter/F2: editar
            if (e.key === 'Enter' || e.key === 'F2') {
              e.preventDefault();
              e.stopPropagation();
              open();
              return;
            }

            // Backspace/Delete: confirmar remoção
            if (e.key === 'Backspace' || e.key === 'Delete') {
              e.preventDefault();
              e.stopPropagation();
              remove();
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
              open(e.key);
            }
          }}
          tabIndex={0}
          aria-label={t('editor.mermaid.previewLabel')}
        >
          <MarkdownRenderer
            content={previewMarkdown}
            interactiveButtons={false}
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
