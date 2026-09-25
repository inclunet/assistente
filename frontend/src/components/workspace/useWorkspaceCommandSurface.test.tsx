import { StrictMode, useLayoutEffect, useRef, useState, useSyncExternalStore, type RefObject } from 'react';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { MemoryRouter, useNavigate } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { WorkspaceData, WorkspaceTab } from '../../store/workspaceStore';
import { CommandContextProvider, useCommandContextScope, type CommandContextScope } from '../../lib/commandContextReact';
import { createCommandUIEffectGuard } from '../../lib/commandUIEffect';
import { WorkspacePanelProvider } from './WorkspacePanelContext';
import { useWorkspaceCommandSurface } from './useWorkspaceCommandSurface';

const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
  activeSubscriptions: 0,
  listeners: new Set<() => void>(),
}));

const tab: WorkspaceTab = {
  id: 'tab-a', type: 'chat', title: 'Chat', position: 0,
};
const workspaceState = vi.hoisted(() => ({
  workspace: {
    id: 'workspace-a', name: 'Workspace A', profile: 'default', tabs: [], activeTabId: 'tab-a',
  } as WorkspaceData,
  listeners: new Set<() => void>(),
  activeSubscriptions: 0,
}));
workspaceState.workspace.tabs = [tab];

vi.mock('../../store/authStore', () => ({
  useAuthStore: (() => {
    const subscribe = vi.fn((listener: () => void) => {
      authState.listeners.add(listener);
      authState.activeSubscriptions += 1;
      return () => {
        if (authState.listeners.delete(listener)) authState.activeSubscriptions -= 1;
      };
    });
    const useStore = (selector?: (state: typeof authState) => unknown) =>
      selector ? useSyncExternalStore(subscribe, () => selector(authState)) : authState;
    return Object.assign(useStore, { getState: () => authState, subscribe });
  })(),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: (() => {
    const subscribe = vi.fn((listener: () => void) => {
      workspaceState.listeners.add(listener);
      workspaceState.activeSubscriptions += 1;
      return () => {
        if (workspaceState.listeners.delete(listener)) workspaceState.activeSubscriptions -= 1;
      };
    });
    const useStore = (selector?: (state: typeof workspaceState) => unknown) =>
      selector ? useSyncExternalStore(subscribe, () => selector(workspaceState)) : workspaceState;
    return Object.assign(useStore, { getState: () => workspaceState, subscribe });
  })(),
}));

interface TestSurfaceContext {
  surfaceType: 'chat';
  surfaceId: string;
  title: string;
  mode: string;
  snapshotVersion: string;
}

const context: TestSurfaceContext = {
  surfaceType: 'chat',
  surfaceId: tab.id,
  title: 'Chat',
  mode: 'idle',
  snapshotVersion: 'snapshot-1',
};

interface SurfaceOptions {
  active?: boolean;
  hidden?: boolean;
  getter?: () => TestSurfaceContext | null;
  subscribe?: (invalidate: () => void) => () => void;
  rootRef?: RefObject<HTMLDivElement>;
}

function Surface({
  active = true,
  hidden = false,
  getter = () => context,
  subscribe,
  rootRef: suppliedRootRef,
}: SurfaceOptions) {
  const ownRootRef = useRef<HTMLDivElement>(null);
  const rootRef = suppliedRootRef ?? ownRootRef;
  return (
    <div ref={rootRef} hidden={hidden} aria-hidden={hidden ? 'true' : undefined} data-testid="surface-root">
      <WorkspacePanelProvider value={{ tab, isActive: active, rootRef }}>
        <RegisteredSurface getter={getter} subscribe={subscribe} />
      </WorkspacePanelProvider>
    </div>
  );
}

function RegisteredSurface({
  getter,
  subscribe,
}: {
  getter: () => TestSurfaceContext | null;
  subscribe?: (invalidate: () => void) => () => void;
}) {
  useWorkspaceCommandSurface('chat', getter, subscribe);
  return <button type="button" data-testid="surface-control">Surface</button>;
}

function SurfaceWithProvider(props: SurfaceOptions) {
  const rootRef = useRef<HTMLDivElement>(null);
  return (
    <MemoryRouter initialEntries={['/workspace']}>
      <CommandContextProvider>
        <div>
          <Surface {...props} rootRef={rootRef} />
          <ScopeProbe />
          <RouteChanger />
        </div>
      </CommandContextProvider>
    </MemoryRouter>
  );
}

function DynamicSurfaceWithProvider() {
  const [visible, setVisible] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  return (
    <MemoryRouter initialEntries={['/workspace']}>
      <CommandContextProvider>
        <button type="button" data-testid="mount-surface" onClick={() => setVisible(true)}>Mount</button>
        <div ref={rootRef} data-testid="dynamic-root">
          {visible && (
            <WorkspacePanelProvider value={{ tab, isActive: true, rootRef }}>
              <RegisteredSurface getter={() => context} />
            </WorkspacePanelProvider>
          )}
        </div>
        <ScopeProbe />
      </CommandContextProvider>
    </MemoryRouter>
  );
}

function ScopeProbe() {
  const scope = useCommandContextScope();
  useLayoutEffect(() => {
    if (scope) (window as Window & { __commandScope?: CommandContextScope }).__commandScope = scope;
  }, [scope]);
  return null;
}

function RouteChanger() {
  const navigate = useNavigate();
  return <button type="button" onClick={() => navigate('/other')} data-testid="change-route">Route</button>;
}

function currentScope(): CommandContextScope {
  const scope = (window as Window & { __commandScope?: CommandContextScope }).__commandScope;
  if (!scope) throw new Error('scope was not mounted');
  return scope;
}

function resetStores() {
  authState.isAuthenticated = true;
  authState.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  authState.listeners.clear();
  authState.activeSubscriptions = 0;
  workspaceState.workspace = {
    id: 'workspace-a', name: 'Workspace A', profile: 'default', tabs: [tab], activeTabId: 'tab-a',
  };
  workspaceState.listeners.clear();
  workspaceState.activeSubscriptions = 0;
  delete (window as Window & { __commandScope?: CommandContextScope }).__commandScope;
}

afterEach(() => {
  vi.restoreAllMocks();
  resetStores();
});

describe('useWorkspaceCommandSurface', () => {
  it('captura e commita o guard na surface ativa e visível do painel', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });

    const guard = createCommandUIEffectGuard(currentScope().session);
    const token = guard.capture(tab.id);
    const effect = vi.fn(() => undefined);

    expect(token).toBeDefined();
    expect(guard.commit(token as object, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    guard.dispose();
  });

  it('registra painel montado depois do provider sem trocar o scope', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<DynamicSurfaceWithProvider />);
    const oldScope = currentScope();
    fireEvent.click(screen.getByTestId('mount-surface'));
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });

    const guard = createCommandUIEffectGuard(currentScope().session);
    const token = guard.capture(tab.id);
    const effect = vi.fn(() => undefined);
    expect(currentScope()).toBe(oldScope);
    expect(token).toBeDefined();
    expect(guard.commit(token as object, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    guard.dispose();
  });

  it.each([
    ['painel inativo', { active: false }],
    ['painel hidden', { hidden: true }],
    ['fonte ausente', { getter: () => null }],
  ])('recusa capture para %s', (_label, props) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider {...props} />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });

    const scope = currentScope();
    const guard = createCommandUIEffectGuard(scope.session);
    expect(scope.surfaceForElement(control)).toBe('active' in props && props.active === false ? undefined : tab.id);
    expect(guard.capture(tab.id)).toBeUndefined();
    guard.dispose();
  });

  it('recusa painel desconectado mesmo com lease antiga', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const scope = currentScope();
    const guard = createCommandUIEffectGuard(scope.session);
    const token = guard.capture(tab.id);
    expect(token).toBeDefined();

    view.rerender(<MemoryRouter><CommandContextProvider><ScopeProbe /></CommandContextProvider></MemoryRouter>);
    expect(guard.commit(token as object, () => undefined)).toBe(false);
    guard.dispose();
  });

  it('recusa commit após retarget do store sem notificação', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const guard = createCommandUIEffectGuard(currentScope().session);
    const token = guard.capture(tab.id);
    const effect = vi.fn(() => undefined);

    workspaceState.workspace = {
      ...workspaceState.workspace,
      id: 'workspace-b',
      tabs: [tab],
      activeTabId: tab.id,
    };

    expect(guard.commit(token as object, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
    guard.dispose();
  });

  it('relê o getter na confirmação mesmo sem notificação da fonte', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let currentContext: typeof context | null = context;
    render(<SurfaceWithProvider getter={() => currentContext} />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const guard = createCommandUIEffectGuard(currentScope().session);
    const token = guard.capture(tab.id);
    const effect = vi.fn(() => undefined);

    currentContext = { ...context, snapshotVersion: 'snapshot-2' };

    expect(guard.commit(token as object, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
    guard.dispose();
  });

  it('aposenta a lease no ciclo ABA mesmo com o mesmo snapshot final', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const sourceListeners = new Set<() => void>();
    const subscribe = (invalidate: () => void) => {
      sourceListeners.add(invalidate);
      return () => { sourceListeners.delete(invalidate); };
    };
    render(<SurfaceWithProvider subscribe={subscribe} />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const guard = createCommandUIEffectGuard(currentScope().session);
    const oldToken = guard.capture(tab.id);
    expect(oldToken).toBeDefined();

    act(() => {
      workspaceState.workspace = { ...workspaceState.workspace, profile: 'profile-b' };
      sourceListeners.forEach((invalidate) => invalidate());
      workspaceState.workspace = { ...workspaceState.workspace, profile: 'default' };
    });

    expect(guard.commit(oldToken as object, () => undefined)).toBe(false);
    const newToken = guard.capture(tab.id);
    const effect = vi.fn(() => undefined);
    expect(newToken).toBeDefined();
    expect(guard.commit(newToken as object, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    guard.dispose();
  });

  it('não reanexa getter de source antiga após o scope observar troca de owner', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const sourceListeners = new Set<() => void>();
    const subscribe = (invalidate: () => void) => {
      sourceListeners.add(invalidate);
      return () => { sourceListeners.delete(invalidate); };
    };
    render(<SurfaceWithProvider subscribe={subscribe} />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const oldScope = currentScope();
    expect(oldScope.surfaceForElement(control)).toBe(tab.id);

    act(() => {
      authState.user = { userId: 'user-a', sessionId: 'session-b', role: 'user' };
      authState.listeners.forEach((listener) => listener());
      authState.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
      sourceListeners.forEach((invalidate) => invalidate());
    });

    expect(oldScope.surfaceForElement(control)).toBeUndefined();
    expect(oldScope.session.readOwnedCommandContextFrame(tab.id)).toBeUndefined();
  });

  it('descarta lease antiga quando a sessão muda', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const oldScope = currentScope();
    const guard = createCommandUIEffectGuard(oldScope.session);
    const sessionToken = guard.capture(tab.id);
    act(() => {
      authState.user = { userId: 'user-a', sessionId: 'session-b', role: 'user' };
      authState.listeners.forEach((listener) => listener());
    });
    expect(guard.commit(sessionToken as object, () => undefined)).toBe(false);

    guard.dispose();
  });

  it('remonta um scope novo no owner ABA notificado, sem ressuscitar o token antigo', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const oldScope = currentScope();
    const oldGuard = createCommandUIEffectGuard(oldScope.session);
    const oldToken = oldGuard.capture(tab.id);
    expect(oldScope.surfaceForElement(control)).toBe(tab.id);

    act(() => {
      authState.user = { userId: 'user-a', sessionId: 'session-b', role: 'user' };
      authState.listeners.forEach((listener) => listener());
      authState.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
      authState.listeners.forEach((listener) => listener());
    });

    expect(oldScope.surfaceForElement(control)).toBeUndefined();
    expect(oldScope.session.readOwnedCommandContextFrame(tab.id)).toBeUndefined();
    expect(oldGuard.commit(oldToken as object, () => undefined)).toBe(false);

    const newScope = currentScope();
    expect(newScope).not.toBe(oldScope);
    const newGuard = createCommandUIEffectGuard(newScope.session);
    const newToken = newGuard.capture(tab.id);
    const effect = vi.fn(() => undefined);
    expect(newToken).toBeDefined();
    expect(newGuard.commit(newToken as object, effect)).toBe(true);
    expect(effect).toHaveBeenCalledTimes(1);
    newGuard.dispose();
    oldGuard.dispose();
  });

  it('descarta lease antiga quando a rota recria o scope', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<SurfaceWithProvider />);
    const control = screen.getByTestId('surface-control');
    act(() => { control.focus(); });
    const oldScope = currentScope();
    const guard = createCommandUIEffectGuard(oldScope.session);
    const token = guard.capture(tab.id);
    expect(token).toBeDefined();

    fireEvent.click(screen.getByTestId('change-route'));
    expect(currentScope()).not.toBe(oldScope);
    expect(guard.commit(token as object, () => undefined)).toBe(false);
    guard.dispose();
  });

  it('limpa subscriptions de scope e surface sob StrictMode e unmount', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<StrictMode><SurfaceWithProvider /></StrictMode>);
    expect(authState.activeSubscriptions).toBeGreaterThan(0);
    expect(workspaceState.activeSubscriptions).toBeGreaterThan(0);
    view.unmount();
    expect(authState.activeSubscriptions).toBe(0);
    expect(workspaceState.activeSubscriptions).toBe(0);
  });
});
