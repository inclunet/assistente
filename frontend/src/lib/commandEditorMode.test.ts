import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  EDITOR_MODE_COMMAND_EVENT,
  EDITOR_MODE_COMMAND_IDS,
  captureEditorModeTarget,
  captureEditorViewFocusTarget,
  isEditorModeCommand,
  registerEditorModeSurface,
  requestEditorModeCommand,
} from './commandEditorMode';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

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

  it('captura refoco local de view sem criar lease ou aplicar mudança de modo', () => {
    const root = document.createElement('div'); document.body.append(root);
    const focusCurrentView = vi.fn(() => true);
    const applyCommitted = vi.fn(() => true);
    const dispose = registerEditorModeSurface(registration(root, {
      mode: 'view', readOnly: true, focusCurrentView, applyCommitted,
    }));
    const focus = captureEditorViewFocusTarget(() => '/');
    expect(focus?.isCurrent()).toBe(true);
    expect(focus?.focus()).toBe(true);
    expect(focus?.focus()).toBe(false);
    expect(focusCurrentView).toHaveBeenCalledTimes(1);
    expect(applyCommitted).not.toHaveBeenCalled();
    dispose();
  });

  it('permite landmark do workspace somente no lease de refoco, não no lease de mudança de modo', () => {
    const root = document.createElement('div'); document.body.append(root);
    let landmarkAllowed = true;
    const focusCurrentView = vi.fn(() => true);
    const dispose = registerEditorModeSurface(registration(root, {
      mode: 'view', focusCurrentView,
      isBlocked: () => true,
      canFocusViewFromWorkspaceLandmark: () => landmarkAllowed,
    }));
    const focus = captureEditorViewFocusTarget(() => '/');
    expect(focus?.isCurrent()).toBe(true);
    const mutation = captureEditorModeTarget(() => '/', 'editor.mode.markdown');
    expect(mutation?.prepare()).toBe(false);
    expect(focus?.focus()).toBe(true);
    expect(focusCurrentView).toHaveBeenCalledTimes(1);

    landmarkAllowed = false;
    expect(captureEditorViewFocusTarget(() => '/')).toBeUndefined();
    dispose();
  });

  it.each([
    ['surface is not in view', { mode: 'markdown' as const }],
    ['surface is asking', { isAsking: () => true }],
    ['surface is inactive', { isActive: () => false }],
    ['surface is stale', { isCurrent: () => false }],
    ['surface is blocked', { isBlocked: () => true }],
    ['focus operation is unavailable', { focusCurrentView: undefined }],
  ])('não captura foco quando %s', (_label, overrides) => {
    const root = document.createElement('div'); document.body.append(root);
    const dispose = registerEditorModeSurface(registration(root, {
      mode: 'view', focusCurrentView: () => true, ...overrides,
    }));
    expect(captureEditorViewFocusTarget(() => '/')).toBeUndefined();
    dispose();
  });

  it('invalida a lease se owner/contexto muda e recusa modal', () => {
    const root = document.createElement('div'); document.body.append(root);
    let current = true;
    const focusCurrentView = vi.fn(() => true);
    const dispose = registerEditorModeSurface(registration(root, {
      mode: 'view', isCurrent: () => current, focusCurrentView,
    }));
    const stale = captureEditorViewFocusTarget(() => '/');
    current = false;
    expect(stale?.isCurrent()).toBe(false);
    expect(stale?.focus()).toBe(false);
    expect(focusCurrentView).not.toHaveBeenCalled();

    current = true;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(overlay);
    registerOpenModal('view-focus-modal');
    try {
      expect(captureEditorViewFocusTarget(() => '/')).toBeUndefined();
    } finally {
      unregisterOpenModal('view-focus-modal');
      overlay.remove();
      dispose();
    }
  });
});
