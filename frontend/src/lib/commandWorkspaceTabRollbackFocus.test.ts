import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { captureWorkspaceTabActivationRollbackFocus } from './commandWorkspaceTabRollbackFocus';

const cleanups: Array<() => void> = [];

function flushRaf(): Promise<void> {
  return new Promise((resolve) => window.requestAnimationFrame(() => resolve()));
}

function makeWorkspace(activeTabId: string): WorkspaceData {
  return {
    id: 'workspace-a',
    name: 'Workspace',
    tabs: ['a', 'b', 'c'].map((id, position) => ({ id, type: 'chat' as const, title: id, position })),
    activeTabId,
  };
}

function fixture() {
  const root = document.createElement('div');
  root.className = 'workspace-layout';
  const tabs = document.createElement('div');
  tabs.className = 'ws-tabs';
  for (const id of ['a', 'b', 'c']) {
    const button = document.createElement('button');
    button.type = 'button';
    button.setAttribute('role', 'tab');
    button.dataset.tabValue = id;
    tabs.append(button);
  }
  const sourcePanel = document.createElement('section');
  sourcePanel.className = 'ws-content__panel';
  sourcePanel.dataset.tabId = 'c';
  const sourceFocus = document.createElement('input');
  sourcePanel.append(sourceFocus);
  root.append(tabs, sourcePanel);
  document.body.append(root);
  return { root, sourcePanel, sourceFocus };
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  unregisterOpenModal('rollback-test');
  document.body.replaceChildren();
  vi.restoreAllMocks();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
});

describe('workspace tab activation rollback focus', () => {
  beforeEach(() => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    useAuthStore.setState({
      isAuthenticated: true,
      user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
    });
  });

  it('recupera o painel do rollback sem esperar outro evento de ativação', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    f.sourceFocus.focus();
    const targetPanel = document.createElement('section');
    targetPanel.tabIndex = -1;
    f.root.append(targetPanel);
    const focusTarget = vi.fn(() => { targetPanel.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));

    const lease = captureWorkspaceTabActivationRollbackFocus(() => '/', {
      failedTabId: 'c',
      rollbackTabId: 'b',
    });
    lease?.apply();

    expect(focusTarget).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(targetPanel);
    lease?.dispose();
  });

  it('não captura rollback quando o foco já estava fora da aba que falhou', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    const focusTarget = vi.fn(() => { f.sourceFocus.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));

    const lease = captureWorkspaceTabActivationRollbackFocus(() => '/', {
      failedTabId: 'c',
      rollbackTabId: 'b',
    });
    expect(lease).toBeUndefined();
    expect(focusTarget).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(outside);
  });

  it('não rouba foco depois de foco externo, modal, rota, auth ou nova ativação', () => {
    const reasons = ['focus', 'modal', 'route', 'auth', 'navigation'] as const;
    for (const reason of reasons) {
      const f = fixture();
      useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
      f.sourceFocus.focus();
      const target = document.createElement('section');
      target.tabIndex = -1;
      f.root.append(target);
      const focusTarget = vi.fn(() => { target.focus(); return true; });
      cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));
      const pathname = { value: '/' };
      const lease = captureWorkspaceTabActivationRollbackFocus(() => pathname.value, {
        failedTabId: 'c',
        rollbackTabId: 'b',
      });
      const outside = document.createElement('button');
      document.body.append(outside);
      if (reason === 'focus') outside.focus();
      if (reason === 'modal') {
        const overlay = document.createElement('div');
        overlay.className = 'modal-overlay';
        document.body.append(overlay);
        registerOpenModal('rollback-test');
      }
      if (reason === 'route') pathname.value = '/settings';
      if (reason === 'auth') useAuthStore.setState({ isAuthenticated: false, user: null });
      if (reason === 'navigation') useWorkspaceStore.setState({ workspace: makeWorkspace('a') });
      lease?.apply();
      expect(focusTarget).not.toHaveBeenCalled();
      lease?.dispose();
      document.body.replaceChildren();
      unregisterOpenModal('rollback-test');
      useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } });
      useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    }
  });

  it('ignora focusin do mesmo elemento capturado, mas invalida focusin externo', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    f.sourceFocus.focus();
    const target = document.createElement('section');
    target.tabIndex = -1;
    f.root.append(target);
    const focusTarget = vi.fn(() => { target.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));
    const lease = captureWorkspaceTabActivationRollbackFocus(() => '/', { failedTabId: 'c', rollbackTabId: 'b' });
    f.sourceFocus.dispatchEvent(new FocusEvent('focusin', { bubbles: true }));
    lease?.apply();
    expect(focusTarget).toHaveBeenCalledOnce();
    lease?.dispose();
  });

  it('cancela pedido lazy quando o foco externo ocorre antes do painel montar', async () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    f.sourceFocus.focus();
    const lease = captureWorkspaceTabActivationRollbackFocus(() => '/', { failedTabId: 'c', rollbackTabId: 'b' });
    lease?.apply();
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    const focusTarget = vi.fn(() => { outside.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));
    await flushRaf();
    expect(focusTarget).not.toHaveBeenCalled();
    lease?.dispose();
  });

  it('bloqueia ABA da geração de modal mesmo que o modal termine fechado', () => {
    const f = fixture();
    useWorkspaceStore.setState({ workspace: makeWorkspace('b') });
    f.sourceFocus.focus();
    const target = document.createElement('section');
    target.tabIndex = -1;
    f.root.append(target);
    const focusTarget = vi.fn(() => { target.focus(); return true; });
    cleanups.push(registerWorkspacePanelFocus('b', focusTarget, focusTarget, () => true));
    const lease = captureWorkspaceTabActivationRollbackFocus(() => '/', { failedTabId: 'c', rollbackTabId: 'b' });
    registerOpenModal('rollback-test');
    unregisterOpenModal('rollback-test');
    lease?.apply();
    expect(focusTarget).not.toHaveBeenCalled();
    lease?.dispose();
  });
});
