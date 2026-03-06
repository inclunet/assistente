type NodeLike = {
  attrs?: {
    language?: unknown;
    mermaidBlockId?: unknown;
  };
  nodeSize: number;
};

type PMDocLike = {
  descendants: (fn: (node: unknown, pos: number) => boolean) => void;
};

type EditorStateLike = {
  doc: PMDocLike;
};

type EditorCommandsLike = {
  command: (
    fn: (ctx: {
      tr: {
        replaceWith: (from: number, to: number, content: unknown) => void;
        delete: (from: number, to: number) => void;
      };
      state: {
        schema: {
          text: (text: string) => unknown;
        };
      };
    }) => boolean
  ) => void;
};

type EditorLike = {
  state?: EditorStateLike;
  commands?: EditorCommandsLike;
};

export type MermaidHit = { pos: number; node: NodeLike };

function isNodeLike(node: unknown): node is NodeLike {
  return !!node && typeof node === 'object' && 'nodeSize' in node;
}

export function findMermaidNodeById(editor: unknown, mermaidBlockId: string): MermaidHit | null {
  const idTarget = String(mermaidBlockId || '').trim();
  if (!idTarget) return null;

  const editorLike = editor as EditorLike;
  const doc = editorLike.state?.doc;
  if (!doc) return null;

  let found: MermaidHit | null = null;
  doc.descendants((node: unknown, pos: number) => {
    if (!isNodeLike(node)) return true;

    const lang = String(node.attrs?.language || '').toLowerCase();
    const id = String(node.attrs?.mermaidBlockId || '').trim();

    if (lang === 'mermaid' && id === idTarget) {
      found = { pos, node };
      return false;
    }

    return true;
  });

  return found;
}

export function applyMermaidById(editor: unknown, mermaidBlockId: string, nextCode: string): boolean {
  const editorLike = editor as EditorLike;
  const commands = editorLike.commands;
  if (!commands) return false;

  const hit = findMermaidNodeById(editor, mermaidBlockId);
  if (!hit) return false;

  try {
    const from = hit.pos + 1;
    const to = hit.pos + hit.node.nodeSize - 1;
    commands.command(({ tr, state }) => {
      tr.replaceWith(from, to, state.schema.text(String(nextCode || '')));
      return true;
    });
    return true;
  } catch {
    return false;
  }
}

export function removeMermaidById(editor: unknown, mermaidBlockId: string): boolean {
  const editorLike = editor as EditorLike;
  const commands = editorLike.commands;
  if (!commands) return false;

  const hit = findMermaidNodeById(editor, mermaidBlockId);
  if (!hit) return false;

  try {
    const from = hit.pos;
    const to = hit.pos + hit.node.nodeSize;
    commands.command(({ tr }) => {
      tr.delete(from, to);
      return true;
    });
    return true;
  } catch {
    return false;
  }
}
