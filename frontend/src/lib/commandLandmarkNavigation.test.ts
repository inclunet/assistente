import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { captureLandmarkNavigationTarget, registerLandmarkNavigationSurface, requestLandmarkNavigationCommand, LANDMARK_COMMAND_EVENT, type LandmarkNavigationSnapshot } from './commandLandmarkNavigation';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const cleanups: (() => void)[] = [];
let nonce = 0;
function surface(overrides: Partial<LandmarkNavigationSnapshot> = {}) {
  const root = document.createElement('div');
  const buttons = ['tabs', 'tools', 'content'].map(id => {
    const button = document.createElement('button');
    button.textContent = id;
    root.append(button);
    return button;
  });
  document.body.append(root);
  const snapshot: LandmarkNavigationSnapshot = { enabled: true, allowWhenModalOpen: false, pathname: '/editor', defaultLandmarkId: 'content',
    landmarks: buttons.map(button => ({ id: button.textContent!, label: button.textContent!, contains: () => document.activeElement === button,
      focus: vi.fn(() => { button.focus(); return document.activeElement === button; }) })), ...overrides };
  const listeners = new Set<() => void>();
  const instanceId = `landmarks-${++nonce}`;
  const unregister = registerLandmarkNavigationSurface({ instanceId, read: () => snapshot, subscribe(changed) {
    listeners.add(changed); return () => { listeners.delete(changed); };
  } });
  cleanups.push(unregister, () => root.remove());
  return { root, buttons, snapshot, instanceId, listeners, unregister, changed: () => listeners.forEach(changed => changed()) };
}
function modal(id: string) {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.dataset.modalId = id;
  document.body.append(overlay);
  registerOpenModal(id);
  const close = () => { unregisterOpenModal(id); overlay.remove(); };
  cleanups.push(close);
  return { overlay, close };
}
const capture = () => captureLandmarkNavigationTarget(() => '/editor');
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'editor', title: 'Editor', position: 0 }] } });
});
afterEach(() => { cleanups.splice(0).reverse().forEach(cleanup => cleanup()); vi.restoreAllMocks(); });

describe('commandLandmarkNavigation', () => {
  it.each([['next', 1], ['previous', 2], ['default', 2]] as const)('fixa origem para %s antes do picker e executa uma vez', (suffix, index) => {
    const owner = surface(); owner.buttons[0].focus();
    const lease = capture()!;
    owner.buttons[1].focus();
    const id = `navigation.landmark.${suffix}`;
    expect(lease.canOpen(id)).toBe(true);
    expect(lease.open(id)).toBe(true);
    expect(document.activeElement).toBe(owner.buttons[index]);
    expect(lease.open(id)).toBe(false);
    expect(owner.listeners.size).toBe(0);
  });
  it('prioriza proprietário que contém foco; expectedInstance não escolhe outro', () => {
    const first = surface(); const second = surface(); first.buttons[0].focus();
    expect(captureLandmarkNavigationTarget(() => '/editor', second.instanceId)).toBeUndefined();
    const lease = capture()!; expect(lease.instanceId).toBe(first.instanceId); lease.dispose();
  });
  it('fora das regiões exige único owner habilitado, inclusive com expectedInstance', () => {
    const first = surface(); surface();
    expect(captureLandmarkNavigationTarget(() => '/editor', first.instanceId)).toBeUndefined();
  });
  it('dois owners alegando mesmo foco são ambíguos', () => {
    surface({ landmarks: [{ id: 'one', label: 'One', contains: () => true, focus: () => true }] });
    surface({ landmarks: [{ id: 'two', label: 'Two', contains: () => true, focus: () => true }] });
    expect(capture()).toBeUndefined();
  });
  it.each(['owner', 'session', 'workspace', 'tab'] as const)('invalida ABA %s via stores reais', kind => {
    surface(); const lease = capture()!;
    if (kind === 'owner' || kind === 'session') {
      const user = useAuthStore.getState().user!;
      useAuthStore.setState({ user: { ...user, [kind === 'owner' ? 'userId' : 'sessionId']: 'other' } });
      useAuthStore.setState({ user });
    } else {
      const workspace = useWorkspaceStore.getState().workspace!;
      useWorkspaceStore.setState({ workspace: { ...workspace, [kind === 'workspace' ? 'id' : 'activeTabId']: 'other' } });
      useWorkspaceStore.setState({ workspace });
    }
    expect(lease.open('navigation.landmark.next')).toBe(false);
  });
  it.each(['pathname', 'defaultLandmarkId', 'enabled'] as const)('invalida ABA de %s por subscription', field => {
    const owner = surface(); const lease = capture()!;
    const before = owner.snapshot[field];
    Object.assign(owner.snapshot, { [field]: field === 'enabled' ? false : 'other' }); owner.changed();
    Object.assign(owner.snapshot, { [field]: before }); owner.changed();
    expect(lease.isCurrent()).toBe(false);
    expect(owner.listeners.size).toBe(0);
  });
  it('captura rota do reader e não window.location', () => {
    surface();
    expect(window.location.pathname).not.toBe('/editor');
    const lease = capture()!; expect(lease.isCurrent()).toBe(true); lease.dispose();
    expect(captureLandmarkNavigationTarget(() => '/other')).toBeUndefined();
  });
  it('mudança ABA na ordem das regiões invalida', () => {
    const owner = surface(); const lease = capture()!; const original = owner.snapshot.landmarks;
    owner.snapshot.landmarks = [...original].reverse(); owner.changed();
    owner.snapshot.landmarks = original; owner.changed();
    expect(lease.isCurrent()).toBe(false);
  });
  it('unregister dispõe subscription imediatamente e idempotente', () => {
    const owner = surface(); const lease = capture()!;
    expect(owner.listeners.size).toBe(1);
    owner.unregister(); owner.unregister(); lease.dispose();
    expect(owner.listeners.size).toBe(0); expect(lease.isCurrent()).toBe(false);
  });
  it('novo owner invalida lease existente sem retarget', () => {
    surface(); const lease = capture()!; surface();
    expect(lease.open('navigation.landmark.next')).toBe(false);
  });
  it('não captura sem autenticação ou workspace pronto', () => {
    surface(); useAuthStore.setState({ isAuthenticated: false }); expect(capture()).toBeUndefined();
    useAuthStore.setState({ isAuthenticated: true }); useWorkspaceStore.setState({ workspace: null });
    expect(capture()).toBeUndefined();
  });
  it('modal exige allow + guard explícito e DOM do topmost', () => {
    const owner = surface({ allowWhenModalOpen: true, shouldHandleKey: () => true });
    const top = modal('own'); owner.buttons[0].focus(); expect(capture()).toBeUndefined();
    top.overlay.append(owner.root); owner.buttons[0].focus();
    const lease = capture()!; expect(lease.canOpen('navigation.landmark.next')).toBe(true); lease.dispose();
    owner.snapshot.shouldHandleKey = undefined; expect(capture()).toBeUndefined();
  });
  it('modal superior aberto e fechado invalida para sempre', () => {
    const owner = surface({ allowWhenModalOpen: true, shouldHandleKey: () => true });
    const own = modal('own'); own.overlay.append(owner.root); owner.buttons[0].focus();
    const lease = capture()!; const other = modal('other'); other.close();
    expect(lease.open('navigation.landmark.next')).toBe(false);
  });
  it('não aplica lease modal com foco transferido ao background', () => {
    const owner = surface({ allowWhenModalOpen: true, shouldHandleKey: () => true });
    const own = modal('own'); own.overlay.append(owner.root); owner.buttons[0].focus();
    const lease = capture()!;
    const background = document.createElement('button'); document.body.append(background); background.focus();
    expect(lease.open('navigation.landmark.next')).toBe(false);
    background.remove(); lease.dispose();
  });
  it('modal sem opt-in bloqueia background', () => {
    const owner = surface(); modal('top'); owner.buttons[0].focus(); expect(capture()).toBeUndefined();
  });
  it('guard modal false e true não revive lease', () => {
    let topmost = true;
    const owner = surface({ allowWhenModalOpen: true, shouldHandleKey: () => topmost });
    const own = modal('own'); own.overlay.append(owner.root); owner.buttons[0].focus();
    const lease = capture()!; topmost = false; owner.changed(); topmost = true; owner.changed();
    expect(lease.isCurrent()).toBe(false);
  });
  it('menu bloqueia captura e efeito mas não invalida lease da paleta', () => {
    const owner = surface(); owner.buttons[0].focus(); const lease = capture()!;
    const menu = document.createElement('div'); menu.setAttribute('role', 'menu');
    const input = document.createElement('input'); menu.append(input); document.body.append(menu); input.focus();
    expect(capture()).toBeUndefined(); expect(lease.canOpen('navigation.landmark.next')).toBe(true);
    expect(lease.open('navigation.landmark.next')).toBe(false);
    menu.remove(); owner.buttons[2].focus();
    expect(lease.open('navigation.landmark.next')).toBe(true); expect(document.activeElement).toBe(owner.buttons[1]);
  });
  it('blur e composição invalidam lease mesmo após retorno', () => {
    surface(); const first = capture()!; window.dispatchEvent(new Event('blur'));
    expect(first.isCurrent()).toBe(false);
    const second = capture()!; document.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    document.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    expect(second.isCurrent()).toBe(false);
  });
  it('não captura documento sem foco', () => {
    surface(); vi.mocked(document.hasFocus).mockReturnValue(false); expect(capture()).toBeUndefined();
  });
  it('falha ao ler após subscription limpa lease e listeners', () => {
    const owner = surface(); let calls = 0;
    const lease = captureLandmarkNavigationTarget(() => { if (++calls > 1) throw new Error('route unavailable'); return '/editor'; });
    expect(lease).toBeUndefined(); expect(owner.listeners.size).toBe(0);
  });
  it('request é cancelável sem fallback e carrega identidade exata', () => {
    expect(requestLandmarkNavigationCommand('navigation.landmark.default', 'one')).toBe(false);
    const listener = vi.fn((event: Event) => { expect((event as CustomEvent).detail).toEqual({ commandID: 'navigation.landmark.default', instanceId: 'one' }); event.preventDefault(); });
    window.addEventListener(LANDMARK_COMMAND_EVENT, listener);
    try { expect(requestLandmarkNavigationCommand('navigation.landmark.default', 'one')).toBe(true); }
    finally { window.removeEventListener(LANDMARK_COMMAND_EVENT, listener); }
    expect(listener).toHaveBeenCalledTimes(1);
  });
});
