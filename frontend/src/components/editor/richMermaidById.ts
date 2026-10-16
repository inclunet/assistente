import type { Editor } from '@tiptap/core';
import type { Node } from '@tiptap/pm/model';

export type MermaidHit = { pos: number; node: Node };

/** Exact live-node lookup. Duplicate IDs are ambiguous, never pick the last. */
export function findMermaidNodeById(editor: Pick<Editor, 'state'>, mermaidBlockId: string): MermaidHit | null {
  if (!mermaidBlockId || mermaidBlockId.trim() !== mermaidBlockId) return null;
  const hits: MermaidHit[] = [];
  editor.state.doc.descendants((node, pos) => {
    if (node.type.name === 'codeBlock' && node.attrs.language === 'mermaid' && node.attrs.mermaidBlockId === mermaidBlockId) hits.push({ pos, node });
  });
  return hits.length === 1 ? hits[0] : null;
}
