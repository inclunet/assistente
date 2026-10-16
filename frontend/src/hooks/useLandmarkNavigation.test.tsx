import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, fireEvent, cleanup, act } from '@testing-library/react';
import { MemoryRouter, useNavigate, useLocation, type NavigateFunction } from 'react-router-dom';
import { useLandmarkNavigation, type Landmark } from './useLandmarkNavigation';
import { restoreDefaultFocus } from './useDefaultFocus';

import { captureLandmarkNavigationTarget, LANDMARK_COMMAND_EVENT, type LandmarkCommandRequest } from '../lib/commandLandmarkNavigation';
import { registerOpenModal, unregisterOpenModal } from '../lib/modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';

function openTestModal() {
  const overlay = document.createElement('div');
  overlay.className = 'modal-overlay';
  overlay.dataset.modalId = 'landmark-modal';
  const input = document.createElement('input');
  overlay.append(input);
  document.body.append(overlay);
  registerOpenModal('landmark-modal');
  input.focus();
}
function dispatchRequest(event: Event) {
  const { commandID, instanceId } = (event as CustomEvent<LandmarkCommandRequest>).detail;
  const lease = captureLandmarkNavigationTarget(() => window.location.pathname, instanceId);
  try {
    if (lease?.canOpen(commandID) && lease.open(commandID)) event.preventDefault();
  } finally { lease?.dispose(); }
}

function createLandmark(id: string, label: string, overrides?: Partial<Landmark>): Landmark & { focusFn: ReturnType<typeof vi.fn> } {
  const focusFn = vi.fn(() => true);
  return {
    id,
    label,
    focus: overrides?.focus ?? focusFn,
    contains: overrides?.contains ?? (() => false),
    isAvailable: overrides?.isAvailable,
    focusFn,
  };
}

function Fixture({ landmarks, enabled, defaultLandmarkId }: {
  landmarks: Landmark[];
  enabled?: boolean;
  defaultLandmarkId?: string;
}) {
  useLandmarkNavigation({ landmarks, enabled, defaultLandmarkId });
  return (
    <div>
      <button data-testid="tabs">Guias</button>
      <button data-testid="toolbar">Barra de ferramentas</button>
      <button data-testid="content">Conteúdo</button>
    </div>
  );
}

function ModalScopedFixture({
  landmarks,
  shouldHandleKey = () => true,
}: {
  landmarks: Landmark[];
  shouldHandleKey?: () => boolean;
}) {
  useLandmarkNavigation({
    landmarks,
    allowWhenModalOpen: true,
    shouldHandleKey,
    defaultLandmarkId: landmarks[0]?.id,
  });
  return <button data-testid="modal">Modal</button>;
}

function executeNavigation(shift = false) {
  const id = shift ? 'navigation.landmark.previous' : 'navigation.landmark.next';
  const lease = captureLandmarkNavigationTarget(() => window.location.pathname);
  try { if (lease?.canOpen(id)) lease.open(id); } finally { lease?.dispose(); }
}

describe('useLandmarkNavigation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
    useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'tab', tabs: [{ id: 'tab', type: 'editor', title: 'Editor', position: 0 }] } });
    window.addEventListener(LANDMARK_COMMAND_EVENT, dispatchRequest);
  });

  afterEach(() => {
    cleanup();
    window.removeEventListener(LANDMARK_COMMAND_EVENT, dispatchRequest);
    unregisterOpenModal('landmark-modal');
    document.querySelectorAll('.modal-overlay').forEach(el => el.remove());
    vi.restoreAllMocks();
  });

  it('comando next foca o próximo landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation();

    expect(tabs.focusFn).toHaveBeenCalled();
  });

  it('comando next avança circularmente quando foco está no primeiro landmark', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation();

    expect(toolbar.focusFn).toHaveBeenCalled();
  });

  it('comando next volta ao início quando foco está no último landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo', { contains: () => true });

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation();

    expect(tabs.focusFn).toHaveBeenCalled();
  });

  it('comando previous foca o landmark anterior', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', { contains: () => true });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation(true);

    expect(tabs.focusFn).toHaveBeenCalled();
  });

  it('comando previous circula para o último quando foco está no primeiro', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation(true);

    expect(content.focusFn).toHaveBeenCalled();
  });

  it('pula landmarks indisponíveis', () => {
    const tabs = createLandmark('tabs', 'Guias', {
      contains: () => true,
    });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', {
      isAvailable: () => false,
    });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation();

    expect(toolbar.focusFn).not.toHaveBeenCalled();
    expect(content.focusFn).toHaveBeenCalled();
  });

  it('pula landmarks cujo focus() retorna false (fallback)', () => {
    const tabs = createLandmark('tabs', 'Guias', { contains: () => true });
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', {
      focus: () => false,
    });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[tabs, toolbar, content]} />);

    executeNavigation();

    expect(content.focusFn).toHaveBeenCalled();
  });

  it('não faz nada quando modal está aberto', () => {
    openTestModal();
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    executeNavigation();

    expect(tabs.focusFn).not.toHaveBeenCalled();
  });

  it('permite navegação quando o escopo modal opta por tratar teclas', () => {
    openTestModal();
    const composer = createLandmark('composer', 'Campo de mensagem', { contains: () => true });

    render(<ModalScopedFixture landmarks={[composer]} />);

    executeNavigation();

    expect(composer.focusFn).toHaveBeenCalled();
  });

  it('bloqueia navegação do escopo modal quando o guard retorna false', () => {
    openTestModal();
    const composer = createLandmark('composer', 'Campo de mensagem', { contains: () => true });

    render(<ModalScopedFixture landmarks={[composer]} shouldHandleKey={() => false} />);

    executeNavigation();

    expect(composer.focusFn).not.toHaveBeenCalled();
  });

  it('não faz nada quando enabled=false', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} enabled={false} />);

    executeNavigation();

    expect(tabs.focusFn).not.toHaveBeenCalled();
  });

  it('não reage a outras teclas', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    fireEvent.keyDown(window, { key: 'F5' });
    fireEvent.keyDown(window, { key: 'Tab' });
    fireEvent.keyDown(window, { key: 'Escape' });

    expect(tabs.focusFn).not.toHaveBeenCalled();
  });

  it('lida com lista vazia de landmarks', () => {
    render(<Fixture landmarks={[]} />);

    // Não deve lançar erro
    executeNavigation();
  });

  it('funciona com apenas 1 landmark', () => {
    const tabs = createLandmark('tabs', 'Guias');

    render(<Fixture landmarks={[tabs]} />);

    executeNavigation();
    expect(tabs.focusFn).toHaveBeenCalled();
  });

  it('atualiza landmarks dinamicamente via ref', () => {
    const tabs = createLandmark('tabs', 'Guias');
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');

    const { rerender } = render(<Fixture landmarks={[tabs]} />);

    executeNavigation();
    expect(tabs.focusFn).toHaveBeenCalledTimes(1);

    // Re-render com novo landmark adicionado
    rerender(<Fixture landmarks={[tabs, toolbar]} />);

    // Simula foco em tabs
    tabs.focusFn.mockClear();
    const tabsWithContains = { ...tabs, contains: () => true };
    rerender(<Fixture landmarks={[tabsWithContains, toolbar]} />);

    executeNavigation();
    expect(toolbar.focusFn).toHaveBeenCalled();
  });

  // --- Escape + defaultLandmarkId ---

  it('Escape foca o landmark default quando foco está em outro landmark', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', { contains: () => true });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[toolbar, content]} defaultLandmarkId="content" />);

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(content.focusFn).toHaveBeenCalled();
  });

  it('Escape NÃO faz nada quando foco já está no landmark default', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo', { contains: () => true });

    render(<Fixture landmarks={[toolbar, content]} defaultLandmarkId="content" />);

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(content.focusFn).not.toHaveBeenCalled();
    expect(toolbar.focusFn).not.toHaveBeenCalled();
  });

  it('Escape NÃO faz nada quando modal está aberto', () => {
    openTestModal();
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', { contains: () => true });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[toolbar, content]} defaultLandmarkId="content" />);

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(content.focusFn).not.toHaveBeenCalled();
  });

  it('Escape NÃO faz nada quando foco não está em nenhum landmark', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[toolbar, content]} defaultLandmarkId="content" />);

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(content.focusFn).not.toHaveBeenCalled();
  });

  it('Escape NÃO faz nada sem defaultLandmarkId', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas', { contains: () => true });
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[toolbar, content]} />);

    fireEvent.keyDown(window, { key: 'Escape' });

    expect(content.focusFn).not.toHaveBeenCalled();
    expect(toolbar.focusFn).not.toHaveBeenCalled();
  });

  // --- restoreDefaultFocus (global) ---

  it('restoreDefaultFocus() foca o landmark default registrado', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');
    const content = createLandmark('content', 'Conteúdo');

    render(<Fixture landmarks={[toolbar, content]} defaultLandmarkId="content" />);

    const result = restoreDefaultFocus();

    expect(result).toBe(true);
    expect(content.focusFn).toHaveBeenCalled();
  });

  it('restoreDefaultFocus() retorna false quando não há default registrado', () => {
    const toolbar = createLandmark('toolbar', 'Barra de ferramentas');

    render(<Fixture landmarks={[toolbar]} />);

    const result = restoreDefaultFocus();

    expect(result).toBe(false);
  });

  it('restoreDefaultFocus() desregistra ao desmontar', () => {
    const content = createLandmark('content', 'Conteúdo');

    const { unmount } = render(<Fixture landmarks={[content]} defaultLandmarkId="content" />);

    expect(restoreDefaultFocus()).toBe(true);

    unmount();

    expect(restoreDefaultFocus()).toBe(false);
  });

  it('F6 nativo não executa sem adapter central', () => {
    const target = createLandmark('content', 'Conteúdo');
    render(<Fixture landmarks={[target]} />);
    fireEvent.keyDown(window, { key: 'F6' });
    fireEvent.keyDown(window, { key: 'F6', shiftKey: true });
    expect(target.focusFn).not.toHaveBeenCalled();
  });
  it('MemoryRouter invalida lease em rota ABA sem depender de window.location', () => {
    let navigate!: NavigateFunction;
    let path = '';
    const target = createLandmark('content', 'Conteúdo');
    function RoutedFixture() {
      navigate = useNavigate();
      path = useLocation().pathname;
      return <Fixture landmarks={[target]} />;
    }
    render(<MemoryRouter initialEntries={['/editor']}><RoutedFixture /></MemoryRouter>);
    const lease = captureLandmarkNavigationTarget(() => path)!;
    expect(lease.isCurrent()).toBe(true);
    act(() => navigate('/settings'));
    act(() => navigate('/editor'));
    expect(lease.open('navigation.landmark.next')).toBe(false);
    expect(target.focusFn).not.toHaveBeenCalled();
  });
  it('desmontagem do hook invalida captura e nova montagem não reautoriza', () => {
    const target = createLandmark('content', 'Conteúdo');
    const view = render(<Fixture landmarks={[target]} />);
    const lease = captureLandmarkNavigationTarget(() => window.location.pathname)!;
    view.unmount();
    render(<Fixture landmarks={[target]} />);
    expect(lease.open('navigation.landmark.next')).toBe(false);
    expect(target.focusFn).not.toHaveBeenCalled();
  });

  it('Escape sem dispatcher não executa nem consome', () => {
    window.removeEventListener(LANDMARK_COMMAND_EVENT, dispatchRequest);
    const target = createLandmark('content', 'Conteúdo');
    render(<Fixture landmarks={[createLandmark('tools', 'Ferramentas', { contains: () => true }), target]} defaultLandmarkId="content" />);
    const event = new KeyboardEvent('keydown', { key: 'Escape', bubbles: true, cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(target.focusFn).not.toHaveBeenCalled();
  });

  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }, { ctrlKey: true }, { altKey: true }, { shiftKey: true }, { metaKey: true }])('Escape ignora guarda nativa %j', flags => {
    const target = createLandmark('content', 'Conteúdo');
    render(<Fixture landmarks={[createLandmark('tools', 'Ferramentas', { contains: () => true }), target]} defaultLandmarkId="content" />);
    fireEvent.keyDown(window, { key: 'Escape', ...flags });
    expect(target.focusFn).not.toHaveBeenCalled();
  });

  it('Escape respeita defaultPrevented pelo componente antes de window', () => {
    const target = createLandmark('content', 'Conteúdo');
    const view = render(<Fixture landmarks={[createLandmark('tools', 'Ferramentas', { contains: () => true }), target]} defaultLandmarkId="content" />);
    const button = view.getByTestId('toolbar');
    button.addEventListener('keydown', event => event.preventDefault());
    fireEvent.keyDown(button, { key: 'Escape' });
    expect(target.focusFn).not.toHaveBeenCalled();
  });
});
