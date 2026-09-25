import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { captureWorkspaceTabTarget } from './commandWorkspaceTabTarget';

const tab = { id: 'tab-1', type: 'chat' as const, title: 'Chat', position: 0 };

function setup() {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u1', sessionId: 's1', role: 'user' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws-1', name: 'Workspace', tabs: [tab], activeTabId: tab.id } });
  const root = document.createElement('div');
  root.className = 'workspace-layout';
  document.body.append(root);
  return root;
}

describe('captureWorkspaceTabTarget', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
    useAuthStore.setState({ isAuthenticated: false, user: null });
    useWorkspaceStore.setState({ workspace: null });
  });

  afterEach(() => {
    unregisterOpenModal('chat-create-test-modal');
    document.body.innerHTML = '';
  });

  it('captures only an authenticated active workspace target on the workspace route', () => {
    setup();
    const lease = captureWorkspaceTabTarget(() => '/');
    expect(lease?.isCurrent()).toBe(true);
    expect(captureWorkspaceTabTarget(() => '/settings')).toBeUndefined();
    lease?.dispose();
    expect(lease?.isCurrent()).toBe(false);
  });

  it('requires a mounted visible workspace root and an existing active tab', () => {
    const root = setup();
    root.hidden = true;
    expect(captureWorkspaceTabTarget(() => '/')).toBeUndefined();
    root.hidden = false;
    useWorkspaceStore.setState({ workspace: { id: 'ws-1', name: 'Workspace', tabs: [], activeTabId: 'tab-1' } });
    expect(captureWorkspaceTabTarget(() => '/')).toBeUndefined();
  });

  it.each(['workspace', 'tab', 'session', 'authentication'] as const)('rejects %s ABA independently', (kind) => {
    setup();
    const originalWorkspace = useWorkspaceStore.getState().workspace!;
    const originalAuth = useAuthStore.getState();
    const lease = captureWorkspaceTabTarget(() => '/');
    expect(lease?.isCurrent()).toBe(true);
    if (kind === 'workspace') {
      useWorkspaceStore.setState({ workspace: { ...originalWorkspace, id: 'other' } });
      useWorkspaceStore.setState({ workspace: originalWorkspace });
    } else if (kind === 'tab') {
      useWorkspaceStore.setState({ workspace: { ...originalWorkspace, activeTabId: 'other', tabs: [...originalWorkspace.tabs, { ...tab, id: 'other' }] } });
      useWorkspaceStore.setState({ workspace: originalWorkspace });
    } else {
      useAuthStore.setState(kind === 'session'
        ? { user: { ...originalAuth.user!, sessionId: 'other' } }
        : { isAuthenticated: false, user: null });
      useAuthStore.setState(originalAuth);
    }
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
  });

  it.each(['replace', 'detach', 'hidden', 'inert', 'aria-hidden'] as const)('rejects a root that is %s without relying on another invalidation', (kind) => {
    const root = setup();
    const lease = captureWorkspaceTabTarget(() => '/');
    expect(lease?.isCurrent()).toBe(true);
    if (kind === 'replace') root.replaceWith(root.cloneNode(true));
    else if (kind === 'detach') root.remove();
    else root.setAttribute(kind, kind === 'aria-hidden' ? 'true' : '');
    expect(lease?.isCurrent()).toBe(false);
    lease?.dispose();
  });

  it('invalidates on modal, blur, logout, active-tab/workspace ABA and root replacement', () => {
    const root = setup();
    const lease = captureWorkspaceTabTarget(() => '/');
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(overlay);
    registerOpenModal('chat-create-test-modal');
    expect(lease?.isCurrent()).toBe(false);
    unregisterOpenModal('chat-create-test-modal');
    expect(lease?.isCurrent()).toBe(true);

    window.dispatchEvent(new Event('blur'));
    expect(lease?.isCurrent()).toBe(false);

    const secondRoot = root.cloneNode(true) as HTMLElement;
    root.replaceWith(secondRoot);
    useAuthStore.setState({ isAuthenticated: false, user: null });
    expect(lease?.isCurrent()).toBe(false);

    setup();
    const activeLease = captureWorkspaceTabTarget(() => '/');
    useWorkspaceStore.setState({ workspace: { id: 'ws-2', name: 'Other', tabs: [tab], activeTabId: tab.id } });
    expect(activeLease?.isCurrent()).toBe(false);
  });
});
