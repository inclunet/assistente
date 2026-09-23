import { act, render, waitFor } from '@testing-library/react';
import * as React from 'react';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore, type WorkspaceData, type WorkspaceTab } from '../store/workspaceStore';
import { WorkspacePanelProvider } from '../components/workspace/WorkspacePanelContext';
import { useWorkspaceCommandSurface } from '../components/workspace/useWorkspaceCommandSurface';
import {
  CommandContextProvider,
  useCommandContextScope,
  type CommandContextScope,
} from './commandContextReact';
import { createCommandUIEffectGuard } from './commandUIEffect';
import { ReadFocusContext } from './commandContextProviders';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const tabA: WorkspaceTab = { id: 'tab-a', type: 'editor', title: 'A', position: 0 };
const tabB: WorkspaceTab = { id: 'tab-b', type: 'editor', title: 'B', position: 1 };
const defaultSurfaceSource: SurfaceSource = { version: 1 };
interface SurfaceSource {
  version: number;
}

function workspace(id: string, activeTabId: string | null = 'tab-a'): WorkspaceData {
  return {
    id,
    name: id,
    profile: 'profile-a',
    tabs: [tabA, tabB],
    activeTabId,
  };
}

function setOwner(userId = 'user-a', sessionId = 'session-a', workspaceId = 'workspace-a') {
  useAuthStore.setState({
    isAuthenticated: true,
    user: { userId, sessionId, role: 'user' },
  });
  useWorkspaceStore.setState({ workspace: workspace(workspaceId) });
}

function SurfaceContent({
  tab,
  source,
  notify,
  report,
}: {
  tab: WorkspaceTab;
  source: SurfaceSource;
  notify?: (invalidate: () => void) => () => void;
  report: (scope: CommandContextScope | null, button: HTMLButtonElement | null) => void;
}) {
  const scope = useCommandContextScope();
  const getter = React.useCallback(() => ({
    surfaceType: 'editor',
    surfaceId: tab.id,
    snapshotVersion: `${tab.id}-v${source.version}`,
  }), [source, tab.id]);
  useWorkspaceCommandSurface('editor', getter, notify);
  const root = React.useContext(WorkspacePanelRootContext);
  React.useLayoutEffect(() => {
    report(scope, root?.current?.querySelector('button') ?? null);
  }, [report, root, scope]);
  return null;
}

const WorkspacePanelRootContext = React.createContext<React.RefObject<HTMLDivElement> | null>(null);

function SurfaceBody({
  tab,
  source,
  active,
  hidden,
  notify,
  report,
}: {
  tab: WorkspaceTab;
  source: SurfaceSource;
  active: boolean;
  hidden?: boolean;
  notify?: (invalidate: () => void) => () => void;
  report: (scope: CommandContextScope | null, button: HTMLButtonElement | null) => void;
}) {
  const rootRef = React.useRef<HTMLDivElement>(null);
  return (
    <WorkspacePanelProvider value={{ tab, isActive: active, rootRef }}>
      <WorkspacePanelRootContext.Provider value={rootRef}>
        <div ref={rootRef} hidden={hidden}>
          <button type="button">capturar</button>
          <input aria-label="editor" />
        </div>
        <SurfaceContent tab={tab} source={source} notify={notify} report={report} />
      </WorkspacePanelRootContext.Provider>
    </WorkspacePanelProvider>
  );
}

function ProductHarness({
  tab = tabA,
  source = defaultSurfaceSource,
  active = true,
  hidden,
  notify,
  report,
}: {
  tab?: WorkspaceTab;
  source?: SurfaceSource;
  active?: boolean;
  hidden?: boolean;
  notify?: (invalidate: () => void) => () => void;
  report: (scope: CommandContextScope | null, button: HTMLButtonElement | null) => void;
}) {
  return (
    <MemoryRouter initialEntries={['/editor']}>
      <CommandContextProvider>
        <SurfaceBody tab={tab} source={source} active={active} hidden={hidden} notify={notify} report={report} />
      </CommandContextProvider>
    </MemoryRouter>
  );
}

function MissingProviderHarness({ report }: { report: (scope: CommandContextScope | null) => void }) {
  const scope = useCommandContextScope();
  React.useLayoutEffect(() => report(scope), [report, scope]);
  return <button type="button">sem provider</button>;
}

afterEach(() => {
  unregisterOpenModal('integration-dialog-bottom');
  unregisterOpenModal('integration-dialog-top');
  document.body.replaceChildren();
  useAuthStore.setState({
    isAuthenticated: false,
    user: null,
  });
  useWorkspaceStore.setState({ workspace: null });
  vi.restoreAllMocks();
});

type ProductOptions = Omit<React.ComponentProps<typeof ProductHarness>, 'report'> & {
  strictMode?: boolean;
};

function renderProduct(options: ProductOptions = {}) {
  const { strictMode = false, ...harnessOptions } = options;
  let latestScope: CommandContextScope | null = null;
  let latestButton: HTMLButtonElement | null = null;
  const harness = (
    <ProductHarness
      {...harnessOptions}
      report={(scope, button) => {
        latestScope = scope;
        latestButton = button;
      }}
    />
  );
  const view = render(strictMode ? <React.StrictMode>{harness}</React.StrictMode> : harness);
  return {
    view,
    get scope() { return latestScope; },
    get button() { return latestButton; },
  };
}

function focusButton(button: HTMLButtonElement | null) {
  expect(button).not.toBeNull();
  button?.focus();
}

describe('integração dos contextos de comando no produto', () => {
  it('captura e confirma efeito pela surface real, mas recusa painel inativo e provider ausente', async () => {
    setOwner();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const product = renderProduct();
    await waitFor(() => expect(product.scope?.surfaceForElement(product.button)).toBe('tab-a'));
    focusButton(product.button);

    const guard = createCommandUIEffectGuard(product.scope?.session);
    expect(product.scope?.session.readOwnedCommandContextFrame('tab-a')).toMatchObject({
      frame: { surface: { surfaceId: 'tab-a' }, focus: { hasFocus: true, composition: 'inactive' } },
    });
    expect(ReadFocusContext()).toMatchObject({ hasFocus: true, composition: 'inactive' });
    const effect = vi.fn(() => undefined);
    const token = guard.capture('tab-a');
    expect(token).toBeDefined();
    expect(guard.commit(token!, effect)).toBe(true);
    expect(effect).toHaveBeenCalledOnce();

    product.view.rerender(
      <MemoryRouter initialEntries={['/editor']}>
        <CommandContextProvider>
          <SurfaceBody
            tab={tabA}
            source={{ version: 1 }}
            active={false}
            report={() => undefined}
          />
        </CommandContextProvider>
      </MemoryRouter>,
    );
    await waitFor(() => expect(product.scope?.surfaceForElement(product.button)).toBeUndefined());
    expect(guard.capture('tab-a')).toBeUndefined();
    guard.dispose();
    product.view.unmount();

    let missingScope: CommandContextScope | null = null;
    const missing = render(
      <MemoryRouter>
        <CommandContextProvider>
          <MissingProviderHarness report={(scope) => { missingScope = scope; }} />
        </CommandContextProvider>
      </MemoryRouter>,
    );
    await waitFor(() => expect(missingScope).not.toBeNull());
    const missingGuard = createCommandUIEffectGuard((missingScope as unknown as CommandContextScope).session);
    document.querySelector<HTMLButtonElement>('button')?.focus();
    expect(missingGuard.capture('tab-a')).toBeUndefined();
    missingGuard.dispose();
    missing.unmount();
  });

  it('revalida diálogo topmost, IME e blur antes de executar qualquer efeito', async () => {
    setOwner();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const product = renderProduct({ strictMode: true });
    await waitFor(() => expect(product.scope?.surfaceForElement(product.button)).toBe('tab-a'));
    focusButton(product.button);
    const guard = createCommandUIEffectGuard(product.scope?.session);
    const effect = vi.fn(() => undefined);

    const modalOverlay = document.createElement('div');
    modalOverlay.className = 'modal-overlay';
    document.body.appendChild(modalOverlay);
    registerOpenModal('integration-dialog-bottom');
    registerOpenModal('integration-dialog-top');
    const modalToken = guard.capture('tab-a');
    expect(modalToken).toBeDefined();
    unregisterOpenModal('integration-dialog-top');
    expect(guard.commit(modalToken!, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
    unregisterOpenModal('integration-dialog-bottom');
    modalOverlay.remove();

    const input = product.button?.parentElement?.querySelector('input') ?? null;
    expect(input).not.toBeNull();
    input?.focus();
    input?.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    input?.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    const imeToken = guard.capture('tab-a');
    expect(imeToken).toBeDefined();
    input?.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
    expect(guard.commit(imeToken!, effect)).toBe(false);
    input?.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
    input?.focus();
    const blurToken = guard.capture('tab-a');
    expect(blurToken).toBeDefined();
    window.dispatchEvent(new Event('blur'));
    expect(guard.commit(blurToken!, effect)).toBe(false);
    expect(effect).not.toHaveBeenCalled();
    guard.dispose();
    product.view.unmount();
  });

  it('aposenta a lease notificada e fecha a antiga no ABA sem captura intermediária', async () => {
    setOwner();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const source = { version: 1 };
    let invalidate: (() => void) | undefined;
    const product = renderProduct({
      source,
      notify: (callback) => {
        invalidate = callback;
        return () => { invalidate = undefined; };
      },
    });
    await waitFor(() => expect(product.scope?.surfaceForElement(product.button)).toBe('tab-a'));
    focusButton(product.button);
    const oldScope = product.scope;
    const oldGuard = createCommandUIEffectGuard(oldScope?.session);
    const oldToken = oldGuard.capture('tab-a');
    expect(oldToken).toBeDefined();

    // O getter mudou sem notificar o registro: a releitura do guard deve
    // aposentar o token mesmo sem depender de callback/evento.
    source.version = 2;
    expect(oldGuard.commit(oldToken!, () => undefined)).toBe(false);

    const currentGuard = createCommandUIEffectGuard(product.scope?.session);
    const currentToken = currentGuard.capture('tab-a');
    expect(currentToken).toBeDefined();
    act(() => invalidate?.());
    expect(currentGuard.commit(currentToken!, () => undefined)).toBe(false);

    const scopeBeforeABA = product.scope;
    act(() => {
      useWorkspaceStore.setState({ workspace: workspace('workspace-b', 'tab-b') });
      invalidate?.();
      useWorkspaceStore.setState({ workspace: workspace('workspace-a', 'tab-a') });
      invalidate?.();
    });
    await waitFor(() => expect(product.scope).not.toBe(scopeBeforeABA));
    expect(oldGuard.commit(oldToken!, () => undefined)).toBe(false);
    expect(product.scope).not.toBe(oldScope);
    focusButton(product.button);
    const newGuard = createCommandUIEffectGuard(product.scope?.session);
    const newToken = newGuard.capture('tab-a');
    expect(newToken).toBeDefined();
    expect(newGuard.commit(newToken!, () => undefined)).toBe(true);

    oldGuard.dispose();
    currentGuard.dispose();
    newGuard.dispose();
    product.view.unmount();
  });

  it('não mantém assinaturas nem registra efeito após StrictMode e unmount', async () => {
    setOwner();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const authUnsubscribe = vi.fn();
    const workspaceUnsubscribe = vi.fn();
    const authSubscribe = useAuthStore.subscribe;
    const workspaceSubscribe = useWorkspaceStore.subscribe;
    const authSubscriptions = vi.spyOn(useAuthStore, 'subscribe').mockImplementation(((listener: Parameters<typeof authSubscribe>[0]) => {
      const unsubscribe = authSubscribe(listener);
      return () => { authUnsubscribe(); unsubscribe(); };
    }) as typeof authSubscribe);
    const workspaceSubscriptions = vi.spyOn(useWorkspaceStore, 'subscribe').mockImplementation(((listener: Parameters<typeof workspaceSubscribe>[0]) => {
      const unsubscribe = workspaceSubscribe(listener);
      return () => { workspaceUnsubscribe(); unsubscribe(); };
    }) as typeof workspaceSubscribe);

    const product = renderProduct({ strictMode: true });
    await waitFor(() => expect(product.scope?.surfaceForElement(product.button)).toBe('tab-a'));
    const scope = product.scope;
    product.view.unmount();
    expect(authSubscriptions.mock.calls.length).toBeGreaterThan(0);
    expect(workspaceSubscriptions.mock.calls.length).toBeGreaterThan(0);
    expect(authUnsubscribe).toHaveBeenCalledTimes(authSubscriptions.mock.calls.length);
    expect(workspaceUnsubscribe).toHaveBeenCalledTimes(workspaceSubscriptions.mock.calls.length);

    act(() => {
      useAuthStore.setState({ user: { userId: 'user-b', sessionId: 'session-b', role: 'user' } });
      useWorkspaceStore.setState({ workspace: workspace('workspace-b', 'tab-b') });
    });
    expect(scope?.surfaceForElement(document.body)).toBeUndefined();
  });
});
