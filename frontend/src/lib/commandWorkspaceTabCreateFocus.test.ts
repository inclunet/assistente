import { afterEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type TabType } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { captureWorkspaceTabCreateFocus } from './commandWorkspaceTabCreateFocus';

const cleanups: Array<() => void> = [];
const types: TabType[] = ['chat', 'editor', 'tasklist', 'terminal'];

function setup() {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'user' } });
  const source = { id: 'old', type: 'chat' as const, title: 'Old', position: 0 };
  const workspace = { id: 'workspace', name: 'Workspace', tabs: [source], activeTabId: source.id };
  useWorkspaceStore.setState({ workspace });
  const layout = document.createElement('div');
  layout.className = 'workspace-layout';
  const oldPanel = document.createElement('section');
  oldPanel.className = 'ws-content__panel';
  oldPanel.dataset.tabId = source.id;
  const input = document.createElement('input');
  oldPanel.append(input);
  layout.append(oldPanel);
  document.body.append(layout);
  input.focus();
  const lease = captureWorkspaceTabCreateFocus(() => '/');
  if (lease) cleanups.push(lease.dispose);
  return { workspace, layout, oldPanel, input, lease };
}

function addCreated(type: TabType, id = 'new') {
  const ws = useWorkspaceStore.getState().workspace!;
  const tab = { id, type, title: 'New', position: ws.tabs.length };
  useWorkspaceStore.setState({ workspace: { ...ws, tabs: [...ws.tabs, tab], activeTabId: id } });
  const panel = document.createElement('section');
  panel.className = 'ws-content__panel';
  panel.dataset.tabId = id;
  panel.dataset.tabType = type;
  panel.tabIndex = -1;
  document.querySelector('.workspace-layout')!.append(panel);
  const focus = vi.fn(() => { panel.focus(); return true; });
  const unregister = registerWorkspacePanelFocus(id, focus, focus, () => true);
  cleanups.push(unregister);
  return { panel, focus };
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  unregisterOpenModal('create-focus-test');
  document.body.replaceChildren();
  vi.restoreAllMocks();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
});

describe('captureWorkspaceTabCreateFocus', () => {
  it.each(types)('restaura foco no painel novo de tipo %s', async (type) => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', `workspace.tab.${type}.create`);
    if (lease) cleanups.push(lease.dispose);
    const { panel, focus } = addCreated(type);
    await lease?.apply();
    expect(focus).toHaveBeenCalledOnce();
    expect(document.activeElement).toBe(panel);
    expect(document.activeElement).not.toBe(f.input);
    expect(f.layout.isConnected).toBe(true);
  });

  it('reconhece snapshot da criação que chegou antes de apply', async () => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.editor.create');
    if (lease) cleanups.push(lease.dispose);
    const { panel } = addCreated('editor');
    await lease?.apply();
    expect(document.activeElement).toBe(panel);
    expect(f.oldPanel.isConnected).toBe(true);
  });

  it('espera snapshot chegar depois de apply dentro da janela cancelável', async () => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.editor.create');
    if (lease) cleanups.push(lease.dispose);
    const pending = lease?.apply();
    let panel: HTMLElement | undefined;
    await new Promise<void>((resolve) => setTimeout(() => {
      panel = addCreated('editor').panel;
      resolve();
    }, 180));
    await pending;
    expect(panel).toBeDefined();
    expect(document.activeElement).toBe(panel);
    expect(f.input).not.toHaveFocus();
  });

  it('cancela foco se a origem do comando fica stale enquanto aguarda o snapshot', async () => {
    const f = setup();
    let originCurrent = true;
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.editor.create', () => originCurrent);
    if (lease) cleanups.push(lease.dispose);
    const pending = lease?.apply();
    originCurrent = false;
    await new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
    await pending;
    const { panel, focus } = addCreated('editor');
    expect(focus).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(f.input);
    expect(document.activeElement).not.toBe(panel);
  });

  it('aceita captura com foco no botão da toolbar do workspace', async () => {
    const f = setup();
    const toolbarButton = document.createElement('button');
    toolbarButton.textContent = 'Nova aba';
    f.layout.append(toolbarButton);
    toolbarButton.focus();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.chat.create');
    if (lease) cleanups.push(lease.dispose);
    const { panel } = addCreated('chat');
    await lease?.apply();
    expect(document.activeElement).toBe(panel);
  });

  it.each([
    ['tipo incorreto', (_f: ReturnType<typeof setup>) => addCreated('terminal').panel],
    ['aba antiga ainda ativa', (f: ReturnType<typeof setup>) => {
      addCreated('editor');
      useWorkspaceStore.setState({ workspace: { ...f.workspace, tabs: [...f.workspace.tabs, { id: 'new', type: 'editor', title: 'New', position: 1 }], activeTabId: 'old' } });
      return document.querySelector('.ws-content__panel[data-tab-id="new"]') as HTMLElement;
    }],
  ] as const)('recusa %s', async (_reason, mutate) => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.editor.create');
    if (lease) cleanups.push(lease.dispose);
    const newPanel = mutate(f);
    await lease?.apply();
    expect(document.activeElement).toBe(f.input);
    expect(document.activeElement).not.toBe(newPanel);
  });

  it.each(['focus', 'blur', 'route', 'workspace', 'session', 'modal'] as const)('abandona o foco quando muda %s', async (reason) => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => reason === 'route' ? '/settings' : '/');
    if (lease) cleanups.push(lease.dispose);
    const other = document.createElement('button');
    document.body.append(other);
    if (reason === 'focus') other.focus();
    if (reason === 'blur') window.dispatchEvent(new Event('blur'));
    if (reason === 'workspace') useWorkspaceStore.setState({ workspace: { ...f.workspace, id: 'other' } });
    if (reason === 'session') useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'other', role: 'user' } });
    if (reason === 'modal') registerOpenModal('create-focus-test');
    if (reason !== 'session' && reason !== 'workspace') addCreated('chat');
    await lease?.apply();
    expect(document.activeElement).not.toBe(document.querySelector('.ws-content__panel[data-tab-id="new"]'));
    if (reason === 'focus') expect(document.activeElement).toBe(other);
  });

  it('invalida troca da aba ativa mesmo quando volta antes da criação', () => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.editor.create');
    if (lease) cleanups.push(lease.dispose);
    const other = { id: 'other', type: 'chat' as const, title: 'Other', position: 1 };
    useWorkspaceStore.setState({ workspace: { ...f.workspace, tabs: [...f.workspace.tabs, other], activeTabId: other.id } });
    useWorkspaceStore.setState({ workspace: f.workspace });
    const { panel } = addCreated('editor');
    lease?.apply();
    expect(document.activeElement).toBe(f.input);
    expect(document.activeElement).not.toBe(panel);
  });

  it('dispose resolve apply pendente e impede foco posterior', async () => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.chat.create');
    if (lease) cleanups.push(lease.dispose);
    const ws = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({ workspace: { ...ws, tabs: [...ws.tabs, { id: 'new', type: 'chat', title: 'New', position: 1 }], activeTabId: 'new' } });
    const panel = document.createElement('section');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'new';
    panel.dataset.tabType = 'chat';
    panel.tabIndex = -1;
    f.layout.append(panel);
    const pending = lease?.apply();
    lease?.dispose();
    await expect(pending).resolves.toBeUndefined();
    expect(document.activeElement).toBe(f.input);
  });

  it('aguarda montagem lazy por mais de oito frames, limitada por tempo', async () => {
    const f = setup();
    const lease = captureWorkspaceTabCreateFocus(() => '/', 'workspace.tab.tasklist.create');
    if (lease) cleanups.push(lease.dispose);
    const ws = useWorkspaceStore.getState().workspace!;
    useWorkspaceStore.setState({ workspace: { ...ws, tabs: [...ws.tabs, { id: 'new', type: 'tasklist', title: 'New', position: 1 }], activeTabId: 'new' } });
    const pending = lease?.apply();
    let panel: HTMLElement | undefined;
    await new Promise<void>((resolve) => setTimeout(() => {
      panel = document.createElement('section');
      panel.className = 'ws-content__panel';
      panel.dataset.tabId = 'new';
      panel.dataset.tabType = 'tasklist';
      panel.tabIndex = -1;
      f.layout.append(panel);
      const focus = vi.fn(() => { panel?.focus(); return true; });
      cleanups.push(registerWorkspacePanelFocus('new', focus, focus, () => true));
      resolve();
    }, 220));
    await pending;
    expect(panel).toBeDefined();
    expect(document.activeElement).toBe(panel);
    expect(f.input).not.toHaveFocus();
  });

  it('não captura sem identidade, rota de workspace, painel focado ou comando reconhecido', () => {
    const f = setup();
    expect(captureWorkspaceTabCreateFocus(() => '/settings')).toBeUndefined();
    expect(captureWorkspaceTabCreateFocus(() => '/', 'workspace.unknown')).toBeUndefined();
    const outside = document.createElement('button');
    document.body.append(outside);
    outside.focus();
    expect(captureWorkspaceTabCreateFocus(() => '/')).toBeUndefined();
    expect(f.lease).toBeDefined();
  });
});
