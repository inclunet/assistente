import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  EDITOR_PRESENTATION_COMMAND_IDS,
  captureEditorPresentationTarget,
  isEditorPresentationCommand,
  registerEditorPresentationSurface,
} from './commandEditorPresentation';

const command = 'editor.menu.file.open' as const;

function registration(root: HTMLElement, overrides: Partial<Parameters<typeof registerEditorPresentationSurface>[0]> = {}) {
  return {
    root,
    ownerId: 'owner-1',
    sessionId: 'session-1',
    workspaceId: 'workspace-1',
    tabId: 'tab-1',
    documentId: 'document-1',
    instanceId: 'instance-1',
    generation: 'generation-1',
    allowedCommandIds: EDITOR_PRESENTATION_COMMAND_IDS,
    isActive: () => true,
    isCurrent: () => true,
    canOpen: () => true,
    open: () => true,
    ...overrides,
  };
}

afterEach(() => {
  document.body.innerHTML = '';
});

describe('commandEditorPresentation', () => {
  it('expõe os comandos de apresentação e navegação local', () => {
    expect(EDITOR_PRESENTATION_COMMAND_IDS).toEqual([
      'editor.menu.file.open',
      'editor.menu.format.open',
      'editor.menu.insert.open',
      'editor.menu.mode.open',
      'editor.slides.open',
      'editor.presentation.fullscreen',
      'editor.table.cell.next',
      'editor.table.cell.previous',
    ]);
    expect(isEditorPresentationCommand('editor.menu.insert.open')).toBe(true);
    expect(isEditorPresentationCommand('editor.file.save')).toBe(false);
  });

  it('captura uma única superfície visível na rota do editor e rejeita rota/modal/duplicata', () => {
    const root = document.createElement('div');
    document.body.append(root);
    const dispose = registerEditorPresentationSurface(registration(root));
    const lease = captureEditorPresentationTarget(() => '/');
    expect(lease?.isCurrent()).toBe(true);
    expect(captureEditorPresentationTarget(() => '/settings')).toBeUndefined();

    const second = document.createElement('div');
    document.body.append(second);
    const disposeSecond = registerEditorPresentationSurface(registration(second, { instanceId: 'instance-2' }));
    expect(captureEditorPresentationTarget(() => '/')).toBeUndefined();
    disposeSecond();

    root.setAttribute('aria-hidden', 'true');
    expect(captureEditorPresentationTarget(() => '/')).toBeUndefined();
    root.removeAttribute('aria-hidden');
    dispose();
  });

  it('revalida canOpen, consome o lease antes do callback e falha fechado', () => {
    const root = document.createElement('div');
    document.body.append(root);
    let allowed = true;
    const open = vi.fn(() => true);
    const dispose = registerEditorPresentationSurface(registration(root, {
      canOpen: () => allowed,
      open,
    }));
    const lease = captureEditorPresentationTarget(() => '/')!;
    allowed = false;
    expect(lease.canOpen(command)).toBe(false);
    expect(lease.open(command)).toBe(false);
    expect(open).not.toHaveBeenCalled();

    allowed = true;
    const second = captureEditorPresentationTarget(() => '/')!;
    expect(second.open(command)).toBe(true);
    expect(second.open(command)).toBe(false);
    expect(open).toHaveBeenCalledTimes(1);
    dispose();
  });

  it('invalida por subscription/ABA e callbacks que lançam', () => {
    const root = document.createElement('div');
    document.body.append(root);
    let notify: () => void = () => undefined;
    const dispose = registerEditorPresentationSurface(registration(root, {
      subscribe: (callback) => { notify = callback; return () => undefined; },
      canOpen: () => { throw new Error('guard'); },
    }));
    const lease = captureEditorPresentationTarget(() => '/')!;
    notify();
    expect(lease.isCurrent()).toBe(false);
    expect(lease.canOpen(command)).toBe(false);
    expect(lease.open(command)).toBe(false);
    dispose();
  });

  it('recusa registro incompleto e target fora do editor/overlay', () => {
    const root = document.createElement('div');
    document.body.append(root);
    const dispose = registerEditorPresentationSurface(registration(root, {
      allowedCommandIds: [] as never,
    }));
    expect(captureEditorPresentationTarget(() => '/')).toBeUndefined();
    dispose();

    const realRoot = document.createElement('div');
    const foreign = document.createElement('input');
    const overlay = document.createElement('div');
    overlay.setAttribute('aria-modal', 'true');
    overlay.append(foreign);
    document.body.append(realRoot, overlay);
    const cleanup = registerEditorPresentationSurface(registration(realRoot, {
      canOpen: (_id, target) => target !== foreign,
    }));
    const lease = captureEditorPresentationTarget(() => '/')!;
    expect(lease.canOpen(command, foreign)).toBe(false);
    cleanup();
  });

  it('não captura superfície inativa', () => {
    const root = document.createElement('div');
    document.body.append(root);
    const dispose = registerEditorPresentationSurface(registration(root, { isActive: () => false }));
    expect(captureEditorPresentationTarget(() => '/')).toBeUndefined();
    dispose();
  });

  it('invalida lease de target capturado definitivamente após mudança de seleção', () => {
    const root = document.createElement('div');
    document.body.append(root);
    let notifySelectionChanged: (() => void) | undefined;
    let current = true;
    const dispose = registerEditorPresentationSurface(registration(root, {
      captureTarget: () => ({ selection: 'first' }),
      isCapturedTargetCurrent: (target) => current && target !== undefined,
      subscribeCapturedTarget: (_target, onChange) => {
        notifySelectionChanged = onChange;
        return () => { notifySelectionChanged = undefined; };
      },
    }));
    const lease = captureEditorPresentationTarget(() => '/')!;
    expect(notifySelectionChanged).toBeDefined();
    current = false;
    notifySelectionChanged?.();
    current = true;
    expect(lease.isCurrent()).toBe(false);
    expect(lease.canOpen('editor.table.cell.next')).toBe(false);
    dispose();
  });
});
