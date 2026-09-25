import { afterEach, expect, it, vi } from 'vitest';
import { EDITOR_MERMAID_COMMAND_EVENT, captureEditorMermaidTarget, registerEditorMermaidSurface, requestEditorMermaidCommand, type EditorMermaidCommandRequest, type EditorMermaidTarget } from './commandEditorMermaid';
const cleanups: Array<() => void> = [];
afterEach(() => cleanups.splice(0).forEach(off => off()));
function target(): EditorMermaidTarget {
  return { isCurrent: () => true, canExecute: () => true, prepare: async () => true, execute: () => true, dispose: vi.fn() };
}
it('registro passa comando/documento/input exatos e unregister elimina captura', () => {
  const lease = target(); const capture = vi.fn(() => lease); const doc = {};
  const off = registerEditorMermaidSurface({ documentId: 'doc', capture }); cleanups.push(off);
  expect(captureEditorMermaidTarget('editor.mermaid.open', doc, { index: 2 })).toBe(lease);
  expect(capture).toHaveBeenCalledExactlyOnceWith('editor.mermaid.open', doc, { index: 2 });
  off(); expect(captureEditorMermaidTarget('editor.mermaid.open')).toBeUndefined();
});
it('ambiguidade recusa e libera todos os candidatos', () => {
  const a = target(); const b = target();
  cleanups.push(registerEditorMermaidSurface({ documentId: 'a', capture: () => a }), registerEditorMermaidSurface({ documentId: 'b', capture: () => b }));
  expect(captureEditorMermaidTarget('editor.mermaid.open')).toBeUndefined();
  expect(a.dispose).toHaveBeenCalledOnce(); expect(b.dispose).toHaveBeenCalledOnce();
});
it('evento cancelável só retorna aceito com preventDefault e preserva shortcut', () => {
  expect(requestEditorMermaidCommand('editor.mermaid.open')).toBe(false);
  const doc = {}; const input = { code: 'graph TD' }; const shortcut = { version: 1 as const, code: 'Enter', modifiers: ['Control' as const] };
  const listener = (event: Event) => {
    const detail = (event as CustomEvent<EditorMermaidCommandRequest>).detail;
    expect(detail).toEqual({ commandID: 'editor.mermaid.apply', input, expectedDocument: doc, shortcut });
    expect(detail.input).not.toBe(input); event.preventDefault();
  };
  window.addEventListener(EDITOR_MERMAID_COMMAND_EVENT, listener); cleanups.push(() => window.removeEventListener(EDITOR_MERMAID_COMMAND_EVENT, listener));
  expect(requestEditorMermaidCommand('editor.mermaid.apply', input, doc, shortcut)).toBe(true);
});
