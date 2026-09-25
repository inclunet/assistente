export const EDITOR_MERMAID_COMMAND_IDS = ['editor.mermaid.open', 'editor.mermaid.apply', 'editor.mermaid.remove'] as const;
export type EditorMermaidCommandID = typeof EDITOR_MERMAID_COMMAND_IDS[number];
export const isEditorMermaidCommand = (id: string): id is EditorMermaidCommandID =>
  (EDITOR_MERMAID_COMMAND_IDS as readonly string[]).includes(id);
export const isEditorMermaidMutation = (id: unknown): id is 'editor.mermaid.apply' | 'editor.mermaid.remove' =>
  id === 'editor.mermaid.apply' || id === 'editor.mermaid.remove';
export interface EditorMermaidInput { index?: number; mermaidBlockId?: string; insertText?: string; code?: string; expectedEditor?: object }
export interface EditorMermaidTarget {
  /** Proof from the owning editor after its preparation restored that target. */
  hasPreparedFocus?(): boolean;
  isCurrent(): boolean;
  canExecute(id: string): boolean;
  prepare(id: string, input?: unknown): Promise<boolean>;
  execute(id: string): boolean;
  dispose(): void;
}
export interface EditorMermaidSurface {
  readonly documentId: string;
  capture(id: EditorMermaidCommandID, expectedDocument?: object, input?: EditorMermaidInput): EditorMermaidTarget | undefined;
}
const surfaces = new Set<EditorMermaidSurface>();
export const EDITOR_MERMAID_COMMAND_EVENT = 'commands:editor-mermaid';
export interface EditorMermaidCommandRequest { commandID: EditorMermaidCommandID; input?: EditorMermaidInput; expectedDocument?: object; shortcut?: import('./commandShortcut').CommandShortcut }
export function requestEditorMermaidCommand(commandID: EditorMermaidCommandID, input?: EditorMermaidInput, expectedDocument?: object, shortcut?: import('./commandShortcut').CommandShortcut): boolean {
  const event = new CustomEvent<EditorMermaidCommandRequest>(EDITOR_MERMAID_COMMAND_EVENT, {
    cancelable: true, detail: { commandID, input: input ? { ...input } : undefined, expectedDocument, shortcut },
  });
  window.dispatchEvent(event);
  return event.defaultPrevented;
}
export function registerEditorMermaidSurface(surface: EditorMermaidSurface): () => void {
  surfaces.add(surface);
  return () => { surfaces.delete(surface); };
}
export function captureEditorMermaidTarget(id: string, expectedDocument?: object, input?: EditorMermaidInput): EditorMermaidTarget | undefined {
  if (!isEditorMermaidCommand(id)) return undefined;
  const targets = [...surfaces].map(surface => surface.capture(id, expectedDocument, input)).filter((target): target is EditorMermaidTarget => !!target);
  if (targets.length === 1) return targets[0];
  targets.forEach(target => target.dispose());
  return undefined;
}

/** Document identity comes from the owning editor registry, never the active tab. */
export function captureContextualEditorMermaidTarget(id: string): { documentId: string; target: EditorMermaidTarget } | undefined {
  if (!isEditorMermaidMutation(id)) return;
  const matches: Array<{ documentId: string; target: EditorMermaidTarget }> = [];
  for (const surface of surfaces) {
    const target = surface.capture(id);
    if (target) matches.push({ documentId: surface.documentId, target });
  }
  if (matches.length === 1 && matches[0].documentId) {
    const { documentId, target } = matches[0];
    let invalid = false;
    const invalidate = () => { invalid = true; target.dispose(); };
    const blur = (event: FocusEvent) => { if (event.target === window) invalidate(); };
    window.addEventListener('blur', blur, true);
    window.addEventListener('compositionstart', invalidate, true);
    return { documentId, target: {
      isCurrent: () => !invalid && target.isCurrent(),
      canExecute: command => !invalid && target.canExecute(command),
      prepare: async command => !invalid && await target.prepare(command) && !invalid,
      execute: command => !invalid && target.execute(command),
      hasPreparedFocus: () => !invalid && target.hasPreparedFocus?.() === true,
      dispose() { invalid = true; window.removeEventListener('blur', blur, true); window.removeEventListener('compositionstart', invalidate, true); target.dispose(); },
    } };
  }
  matches.forEach(({ target }) => target.dispose());
}
