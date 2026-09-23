import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  EDITOR_MODE_COMMAND_EVENT,
  EDITOR_MODE_COMMAND_IDS,
  captureEditorModeTarget,
  isEditorModeCommand,
  registerEditorModeSurface,
  requestEditorModeCommand,
} from './commandEditorMode';

function registration(root: HTMLElement, overrides: Partial<Parameters<typeof registerEditorModeSurface>[0]> = {}) {
  return {
    root,
    ownerId: 'owner-1', sessionId: 'session-1', workspaceId: 'workspace-1',
    tabId: 'tab-1', documentId: 'document-1', instanceId: 'instance-1',
    generation: 'generation-1', mode: 'rich' as const, readOnly: false,
    isAsking: () => false, isActive: () => true, isCurrent: () => true,
    applyCommitted: () => true, ...overrides,
  };
}

afterEach(() => { document.body.innerHTML = ''; });

describe('commandEditorMode', () => {
  it('expõe IDs estritos e o evento cancelável', () => {
    expect(EDITOR_MODE_COMMAND_IDS).toEqual([
      'editor.mode.markdown', 'editor.mode.rich', 'editor.mode.view',
    ]);
    expect(isEditorModeCommand('editor.mode.view')).toBe(true);
    expect(isEditorModeCommand('editor.mode.insert')).toBe(false);
    expect(requestEditorModeCommand('editor.mode.view')).toBe(false);
    const listener = vi.fn((event: Event) => {
      expect((event as CustomEvent).detail).toEqual({ commandID: 'editor.mode.view' });
      event.preventDefault();
    });
    window.addEventListener(EDITOR_MODE_COMMAND_EVENT, listener);
    expect(requestEditorModeCommand('editor.mode.view')).toBe(true);
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(EDITOR_MODE_COMMAND_EVENT, listener);
  });

  it('filtra modo atual, readonly e asking antes de anunciar disponibilidade', () => {
    const root = document.createElement('div'); document.body.append(root);
    const dispose = registerEditorModeSurface(registration(root, { mode: 'markdown' }));
    expect(captureEditorModeTarget(() => '/', 'editor.mode.markdown')).toBeUndefined();
    dispose();
    const readOnlyDispose = registerEditorModeSurface(registration(root, { readOnly: true }));
    expect(captureEditorModeTarget(() => '/', 'editor.mode.rich')).toBeUndefined();
    expect(captureEditorModeTarget(() => '/', 'editor.mode.view')).toBeDefined();
    readOnlyDispose();
    const askingDispose = registerEditorModeSurface(registration(root, { isAsking: () => true }));
    expect(captureEditorModeTarget(() => '/', 'editor.mode.view')).toBeUndefined();
    askingDispose();
  });

  it('permite captura com menu aberto, mas prepara somente depois do bloqueio sumir', () => {
    const root = document.createElement('div'); document.body.append(root);
    let blocked = true;
    const dispose = registerEditorModeSurface(registration(root, { isBlocked: () => blocked }));
    const lease = captureEditorModeTarget(() => '/', 'editor.mode.markdown');
    expect(lease?.isCurrent()).toBe(true);
    expect(lease?.prepare()).toBe(false);
    blocked = false;
    expect(lease?.prepare()).toBe(true);
    expect(lease?.applyCommitted()).toBe(true);
    dispose();
  });

  it('faz flush uma vez, exige prepare e invalida por subscription/ABA', () => {
    const root = document.createElement('div'); document.body.append(root);
    const flush = vi.fn(); const apply = vi.fn(() => true); let notify: () => void = () => undefined;
    const dispose = registerEditorModeSurface(registration(root, {
      flushRichMarkdownNow: flush, applyCommitted: apply,
      subscribe: (callback) => { notify = callback; return () => undefined; },
    }));
    const lease = captureEditorModeTarget(() => '/', 'editor.mode.markdown')!;
    expect(lease.applyCommitted()).toBe(false);
    expect(lease.prepare()).toBe(true);
    expect(flush).toHaveBeenCalledTimes(1);
    expect(lease.prepare()).toBe(false);
    expect(lease.applyCommitted()).toBe(true);
    expect(apply).toHaveBeenCalledWith('markdown', expect.any(Boolean));
    dispose();

    const second = registerEditorModeSurface(registration(root, {
      instanceId: 'instance-2', subscribe: (callback) => { notify = callback; return () => undefined; },
    }));
    const stale = captureEditorModeTarget(() => '/', 'editor.mode.markdown')!;
    notify();
    expect(stale.isCurrent()).toBe(false);
    expect(stale.prepare()).toBe(false);
    second();
  });

  it('aplica modo confirmado sem roubar foco que mudou durante o backend', () => {
    const root = document.createElement('div');
    const before = document.createElement('input');
    const after = document.createElement('input');
    root.append(before, after); document.body.append(root); before.focus();
    const apply = vi.fn(() => true);
    const dispose = registerEditorModeSurface(registration(root, { applyCommitted: apply }));
    const lease = captureEditorModeTarget(() => '/', 'editor.mode.markdown')!;
    expect(lease.prepare()).toBe(true);
    after.focus();
    expect(lease.applyCommitted()).toBe(true);
    expect(apply).toHaveBeenCalledWith('markdown', false);
    dispose();
  });

  it('recusa registros sem enum de modo, readOnly ou isAsking válidos', () => {
    const root = document.createElement('div'); document.body.append(root);
    const base = registration(root);
    const invalidMode = registerEditorModeSurface({ ...base, mode: 'other' as never });
    const invalidReadOnly = registerEditorModeSurface({ ...base, instanceId: 'r', readOnly: undefined as never });
    const invalidAsking = registerEditorModeSurface({ ...base, instanceId: 'a', isAsking: undefined as never });
    expect(captureEditorModeTarget(() => '/', 'editor.mode.markdown')).toBeUndefined();
    invalidMode(); invalidReadOnly(); invalidAsking();
  });
});
