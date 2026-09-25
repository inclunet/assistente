import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { captureWorkspaceCreateTarget } from './commandWorkspaceCreateTarget';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';

const tab = { id: 'tab-1', type: 'chat' as const, title: 'Chat', position: 0 };

function setup() {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u1', sessionId: 's1', role: 'user' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws-1', name: 'Workspace', tabs: [tab], activeTabId: tab.id } });
}

describe('captureWorkspaceCreateTarget', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  });

  afterEach(() => {
    unregisterOpenModal('workspace-create-target-test-modal');
    document.body.innerHTML = '';
    vi.restoreAllMocks();
  });

  it('captura origem autenticada em qualquer rota sem exigir workspace-layout', () => {
    const lease = captureWorkspaceCreateTarget(() => '/settings/data');
    expect(lease?.isCurrent()).toBe(true);
    lease?.dispose();
    expect(lease?.isCurrent()).toBe(false);
  });

  it('exige foco, autenticação, workspace ativo e aba ativa', () => {
    vi.mocked(document.hasFocus).mockReturnValue(false);
    expect(captureWorkspaceCreateTarget(() => '/settings')).toBeUndefined();
    vi.mocked(document.hasFocus).mockReturnValue(true);

    useAuthStore.setState({ isAuthenticated: false, user: null });
    expect(captureWorkspaceCreateTarget(() => '/settings')).toBeUndefined();
    setup();
    useWorkspaceStore.setState({ workspace: { id: 'ws-1', name: 'Workspace', tabs: [], activeTabId: 'missing' } });
    expect(captureWorkspaceCreateTarget(() => '/settings')).toBeUndefined();
  });

  it('invalida rota, foco, modal e geração de modal depois da captura', () => {
    let pathname = '/settings';
    const lease = captureWorkspaceCreateTarget(() => pathname);
    expect(lease?.isCurrent()).toBe(true);

    pathname = '/help';
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();

    pathname = '/settings';
    const routeLease = captureWorkspaceCreateTarget(() => pathname);
    vi.mocked(document.hasFocus).mockReturnValue(false);
    expect(routeLease?.isCurrent()).toBe(false);
    routeLease?.dispose();

    vi.mocked(document.hasFocus).mockReturnValue(true);
    const modalLease = captureWorkspaceCreateTarget(() => pathname);
    registerOpenModal('workspace-create-target-test-modal');
    expect(modalLease?.isCurrent()).toBe(false);
    unregisterOpenModal('workspace-create-target-test-modal');
    expect(modalLease?.isCurrent()).toBe(false);
    modalLease?.dispose();
  });

  it.each(['workspace', 'tab', 'session', 'logout'] as const)('fecha ABA de %s', (kind) => {
    const lease = captureWorkspaceCreateTarget(() => '/settings');
    const originalWorkspace = useWorkspaceStore.getState().workspace!;
    const originalAuth = useAuthStore.getState();
    if (kind === 'workspace') {
      useWorkspaceStore.setState({ workspace: { ...originalWorkspace, id: 'other' } });
      useWorkspaceStore.setState({ workspace: originalWorkspace });
    } else if (kind === 'tab') {
      useWorkspaceStore.setState({ workspace: { ...originalWorkspace, activeTabId: 'other', tabs: [...originalWorkspace.tabs, { ...tab, id: 'other' }] } });
      useWorkspaceStore.setState({ workspace: originalWorkspace });
    } else if (kind === 'session') {
      useAuthStore.setState({ user: { ...originalAuth.user!, sessionId: 'other' } });
      useAuthStore.setState(originalAuth);
    } else {
      useAuthStore.setState({ isAuthenticated: false, user: null });
      useAuthStore.setState(originalAuth);
    }
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
  });

  it('invalida blur e libera os listeners ao descartar a lease', () => {
    const lease = captureWorkspaceCreateTarget(() => '/settings');
    window.dispatchEvent(new Event('blur'));
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
    window.dispatchEvent(new Event('blur'));
  });
});
