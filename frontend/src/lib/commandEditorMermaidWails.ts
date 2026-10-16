import type { CommandShortcut } from './commandShortcut';
import type { UICommandBeginResponse } from './commandUIExecution';

interface MermaidKeyboardAPI {
  BeginEditorMermaidUIKey(generation: string, shortcut: CommandShortcut, repeat: boolean): Promise<UICommandBeginResponse | null>;
  DispatchLocalCommandKey(generation: string, shortcut: CommandShortcut, kind: string, repeat: boolean): Promise<unknown>;
}
/** The scoped modal shortcut keeps keyboard provenance; never falls back to Palette. */
export async function beginEditorMermaidKey(generation: string, shortcut: CommandShortcut): Promise<UICommandBeginResponse> {
  const api = (window as Window & { go?: { app?: { App?: Partial<MermaidKeyboardAPI> } } }).go?.app?.App;
  if (typeof api?.BeginEditorMermaidUIKey !== 'function' || typeof api.DispatchLocalCommandKey !== 'function') throw new Error('Mermaid keyboard API unavailable');
  if (!generation || shortcut.version !== 1 || shortcut.modifiers.length !== 1 ||
      !(shortcut.code === 'KeyS' && (shortcut.modifiers[0] === 'Control' || shortcut.modifiers[0] === 'Meta') ||
        shortcut.code === 'Enter' && shortcut.modifiers[0] === 'Control')) throw new Error('Invalid Mermaid shortcut');
  const result = await api.BeginEditorMermaidUIKey(generation, { ...shortcut, modifiers: [...shortcut.modifiers] }, false);
  if (!result) throw new Error('Mermaid key already consumed');
  return result;
}

/** Install release observation at keydown, before async modal preparation. */
export function captureEditorMermaidKeyboard(generation: string, shortcut: CommandShortcut) {
  const stable = { ...shortcut, modifiers: [...shortcut.modifiers] };
  let started: Promise<UICommandBeginResponse> | undefined;
  let released = false;
  let sent = false;
  const flush = () => {
    if (!released || !started || sent) return;
    sent = true;
    const sendRelease = async () => {
      const api = (window as Window & { go?: { app?: { App?: Partial<MermaidKeyboardAPI> } } }).go?.app?.App;
      await api?.DispatchLocalCommandKey?.(generation, stable, 'up', false);
    };
    void started.then(sendRelease, sendRelease).catch(() => undefined);
  };
  const release = () => {
    released = true;
    window.removeEventListener('keyup', keyup, true);
    window.removeEventListener('blur', release, true);
    flush();
  };
  const keyup = (event: KeyboardEvent) => { if (event.code === stable.code) release(); };
  window.addEventListener('keyup', keyup, true);
  window.addEventListener('blur', release, true);
  return {
    begin() { if (!started) started = beginEditorMermaidKey(generation, stable); flush(); return started; },
    dispose: release,
  };
}
