import { afterEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type TabType } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { registerWorkspacePanelFocus } from '../components/workspace/workspacePanelFocusRegistry';
import { captureWorkspacePanelTarget } from './commandWorkspacePanel';

const cleanups: Array<() => void> = [];
function fixture(type: TabType = 'chat') {
  const user = { userId: 'u', sessionId: 's', role: 'user' };
  const tab = { id: 'tab', type, title: 'Panel', position: 0 };
  const workspace = { id: 'w', name: 'Workspace', tabs: [tab], activeTabId: tab.id };
  useAuthStore.setState({ isAuthenticated: true, user });
  useWorkspaceStore.setState({ workspace });
  const root = document.createElement('section');
  root.className = 'ws-content__panel';
  root.dataset.tabId = tab.id;
  root.dataset.tabType = type;
  root.dataset.active = 'true';
  const input = document.createElement('input');
  root.append(input);
  document.body.append(root);
  const legacy = vi.fn(() => true);
  const immediate = vi.fn(() => { input.focus(); return true; });
  const ready = vi.fn(() => true);
  const unregister = registerWorkspacePanelFocus(tab.id, legacy, immediate, ready);
  cleanups.push(unregister);
  let pathname = '/';
  function capture() {
    const lease = captureWorkspacePanelTarget(() => pathname);
    if (lease) cleanups.push(lease.dispose);
    return lease;
  }
  return { root, input, user, workspace, tab, immediate, legacy, ready, unregister, capture,
    route: (path: string) => { pathname = path; } };
}

afterEach(() => {
  while (cleanups.length) cleanups.pop()?.();
  unregisterOpenModal('panel-test');
  document.body.replaceChildren();
  useAuthStore.setState({ isAuthenticated: false, user: null });
  useWorkspaceStore.setState({ workspace: null });
});

describe('contexto real do painel ativo', () => {
  it.each<TabType>(['chat', 'editor', 'terminal', 'tasklist'])('foca %s sem enfileirar efeito legado', (type) => {
    const f = fixture(type);
    const lease = f.capture();
    expect(lease?.isCurrent()).toBe(true);
    lease!.focus();
    expect(document.activeElement).toBe(f.input);
    expect(f.immediate).toHaveBeenCalledTimes(1);
    expect(f.legacy).not.toHaveBeenCalled();
  });
  it.each(['hidden', 'inert', 'aria-hidden'])('recusa ancestral %s', (attribute) => {
    const f = fixture();
    const parent = document.createElement('div');
    f.root.replaceWith(parent);
    parent.append(f.root);
    parent.setAttribute(attribute, attribute === 'aria-hidden' ? 'true' : '');
    expect(f.capture()).toBeUndefined();
  });
  it('recusa rota diferente e a relê antes do efeito', () => {
    const f = fixture();
    const lease = f.capture()!;
    f.route('/settings');
    expect(f.capture()).toBeUndefined();
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it('recusa painel removido ou substituído mesmo com os mesmos atributos', () => {
    const f = fixture();
    const lease = f.capture()!;
    const replacement = f.root.cloneNode(true) as HTMLElement;
    f.root.replaceWith(replacement);
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    replacement.remove();
    expect(f.capture()).toBeUndefined();
  });
  it('recusa modal aberto', () => {
    const f = fixture();
    const lease = f.capture()!;
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(overlay);
    registerOpenModal('panel-test');
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it.each(['tab', 'workspace', 'type', 'logout', 'session'] as const)('invalida %s mesmo após retornar ao valor original', (kind) => {
    const f = fixture();
    const lease = f.capture()!;
    if (kind === 'tab') useWorkspaceStore.setState({ workspace: { ...f.workspace, activeTabId: 'other' } });
    if (kind === 'workspace') useWorkspaceStore.setState({ workspace: { ...f.workspace, id: 'other' } });
    if (kind === 'type') useWorkspaceStore.setState({ workspace: { ...f.workspace, tabs: [{ ...f.tab, type: 'editor' }] } });
    if (kind === 'logout') useAuthStore.setState({ isAuthenticated: false, user: null });
    if (kind === 'session') useAuthStore.setState({ user: { ...f.user, sessionId: 'other' } });
    useWorkspaceStore.setState({ workspace: f.workspace });
    useAuthStore.setState({ isAuthenticated: true, user: f.user });
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it('preserva preparação após renomear workspace sem alterar alvo', () => {
    const f = fixture();
    const lease = f.capture()!;
    useWorkspaceStore.setState({ workspace: { ...f.workspace, name: 'Renamed' } });
    expect(lease.isCurrent()).toBe(true);
  });
  it.each(['blur', 'dispose'] as const)('recusa depois de %s', (kind) => {
    const f = fixture();
    const lease = f.capture()!;
    if (kind === 'blur') window.dispatchEvent(new Event('blur'));
    else lease.dispose();
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it('recusa novo registro mesmo reutilizando a mesma função', () => {
    const f = fixture();
    const lease = f.capture()!;
    f.unregister();
    cleanups.push(registerWorkspacePanelFocus(f.tab.id, f.legacy, f.immediate, f.ready));
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
  });
  it('registro apenas adiado não habilita o comando', () => {
    const f = fixture();
    f.unregister();
    cleanups.push(registerWorkspacePanelFocus(f.tab.id, f.legacy));
    expect(f.capture()).toBeUndefined();
    expect(f.legacy).not.toHaveBeenCalled();
  });
  it('prontidão é consultada novamente sem executar efeito', () => {
    const f = fixture();
    const lease = f.capture()!;
    f.ready.mockReturnValue(false);
    expect(f.capture()).toBeUndefined();
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it('recusa DOM de outro tipo antes da atualização visual do painel', () => {
    const f = fixture();
    f.root.dataset.tabType = 'terminal';
    expect(f.capture()).toBeUndefined();
  });
  it('falha de prontidão não escapa para o catálogo nem permite efeito', () => {
    const f = fixture();
    const lease = f.capture()!;
    f.ready.mockImplementation(() => { throw new Error('control unavailable'); });
    expect(f.capture()).toBeUndefined();
    expect(lease.isCurrent()).toBe(false);
    expect(() => lease.focus()).toThrow();
    expect(f.immediate).not.toHaveBeenCalled();
  });
  it.each([false, true])('não aceita retorno %s sem foco aplicado', (result) => {
    const f = fixture();
    f.immediate.mockImplementation(() => result);
    const lease = f.capture()!;
    expect(() => lease.focus()).toThrow();
    expect(document.activeElement).not.toBe(f.input);
  });
});
