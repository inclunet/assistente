import { useRef } from 'react';
import { act, cleanup, createEvent, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useProviderCreationCommands, PROVIDER_CREATION_COMMANDS } from './providerCreationCommands';
import { capturePagePresentationTarget, PAGE_PRESENTATION_COMMAND_EVENT } from './commandPagePresentation';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
const mocks = vi.hoisted(() => ({ catalog: vi.fn(), announce: vi.fn(), open: vi.fn() }));
vi.mock('../services/commandCatalog', () => ({ listCommandCatalog: mocks.catalog }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mocks.announce }) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
let route = '/settings/providers';
function Harness() {
  const root = useRef<HTMLDivElement>(null);
  const creation = useProviderCreationCommands({ root, pathname: route, ready: true, open: mocks.open });
  return <div ref={root}><button>Previous</button><button ref={creation.buttonRef} onClick={creation.requestOpen}>New provider</button>
    <input aria-label="Other" />{creation.menu}</div>;
}
const dispatch = (event: Event) => {
  const detail = (event as CustomEvent).detail;
  const target = capturePagePresentationTarget(() => route, detail.commandID, detail.instanceId);
  if (target?.open(detail.commandID)) event.preventDefault();
};
beforeEach(() => {
  vi.clearAllMocks();
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  route = '/settings/providers';
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'user' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', name: 'Workspace', activeTabId: null, tabs: [] } });
  mocks.catalog.mockResolvedValue(PROVIDER_CREATION_COMMANDS.map(id => ({ id, name: id, available: true })));
  window.addEventListener(PAGE_PRESENTATION_COMMAND_EVENT, dispatch);
});
afterEach(() => {
  window.removeEventListener(PAGE_PRESENTATION_COMMAND_EVENT, dispatch);
  cleanup(); vi.useRealTimers(); vi.restoreAllMocks();
});
describe('Provider creation menu with real presentation registry', () => {
  it.each(['chatgpt', 'acp', 'api'] as const)('opens the selected %s flow by keyboard', async kind => {
    const user = userEvent.setup(); render(<Harness />);
    await user.click(screen.getByRole('button', { name: 'New provider' }));
    await screen.findByRole('menuitem', { name: 'providers.' + kind + '.create.open' });
    await waitFor(() => expect(screen.getByRole('menuitem', { name: 'providers.chatgpt.create.open' })).toHaveFocus());
    await user.keyboard('{ArrowDown}'.repeat(['chatgpt', 'acp', 'api'].indexOf(kind)) + '{Enter}');
    expect(mocks.open).toHaveBeenCalledExactlyOnceWith(kind);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  });
  it('restores trigger focus on Escape and allows reopening', async () => {
    const user = userEvent.setup(); render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'New provider' });
    await user.click(trigger); await screen.findByRole('menuitem', { name: 'providers.api.create.open' });
    await user.keyboard('{Escape}');
    expect(trigger).toHaveFocus();
    await user.keyboard('{Enter}');
    expect(await screen.findByRole('menu')).toBeInTheDocument();
    expect(mocks.open).not.toHaveBeenCalled();
  });
  it.each([false, true])('Tab dismisses from the anchor without preventing browser navigation (reverse=%s)', async reverse => {
    const user = userEvent.setup(); render(<Harness />);
    await user.click(screen.getByRole('button', { name: 'New provider' }));
    await screen.findByRole('menuitem', { name: 'providers.api.create.open' });
    // user-event computes Tab from the original (now removed) event target;
    // Chromium's default navigation is covered by providers-creation.spec.ts.
    const event = createEvent.keyDown(document.activeElement!, { key: 'Tab', shiftKey: reverse });
    fireEvent(document.activeElement!, event);
    expect(event.defaultPrevented).toBe(false);
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'New provider' })).toHaveFocus();
    expect(mocks.open).not.toHaveBeenCalled();
  });
  it('keeps unavailable catalog actions disabled', async () => {
    mocks.catalog.mockResolvedValue([{ id: 'providers.api.create.open', name: 'API', available: false }]);
    render(<Harness />); await userEvent.click(screen.getByRole('button', { name: 'New provider' }));
    const item = await screen.findByRole('menuitem', { name: 'API' });
    expect(item).toBeDisabled();
    fireEvent.click(item); expect(mocks.open).not.toHaveBeenCalled();
  });
  it.each(['session', 'route', 'modal', 'focus', 'escape', 'outside', 'ime'] as const)('discards a late catalog after changing %s', async change => {
    let complete!: (value: unknown[]) => void;
    mocks.catalog.mockReturnValue(new Promise(resolve => { complete = resolve; }));
    const view = render(<Harness />);
    await userEvent.click(screen.getByRole('button', { name: 'New provider' }));
    act(() => {
      if (change === 'session') useAuthStore.setState({ user: { userId: 'other', sessionId: 'new', role: 'user' } });
      if (change === 'route') { route = '/settings/data'; view.rerender(<Harness />); }
      if (change === 'modal') registerOpenModal('blocking');
      if (change === 'focus') { screen.getByRole('textbox').focus(); fireEvent.keyDown(screen.getByRole('textbox'), { key: 'Tab' }); }
      if (change === 'escape') fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
      if (change === 'outside') fireEvent.mouseDown(document.body);
      if (change === 'ime') fireEvent.compositionStart(screen.getByRole('textbox'));
    });
    await act(async () => complete(PROVIDER_CREATION_COMMANDS.map(id => ({ id, name: id, available: true }))));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(mocks.open).not.toHaveBeenCalled();
    if (change === 'modal') unregisterOpenModal('blocking');
  });
  it.each(['menu', 'other', 'timeout'] as const)('catalog failure preserves correct focus (%s)', async focus => {
    let reject!: (error: Error) => void;
    mocks.catalog.mockReturnValue(new Promise((_, rejectPromise) => { reject = rejectPromise; }));
    render(<Harness />);
    const trigger = screen.getByRole('button', { name: 'New provider' });
    await userEvent.click(trigger);
    await waitFor(() => expect(screen.getByRole('menuitem')).toHaveFocus());
    const other = screen.getByRole('textbox');
    if (focus === 'other') other.focus();
    if (focus === 'timeout') {
      vi.useFakeTimers();
      // Existing timer was scheduled with real timers; replace it via reopening.
      fireEvent.keyDown(document.activeElement!, { key: 'Escape' });
      fireEvent.click(trigger);
      act(() => { vi.advanceTimersByTime(100); });
      expect(screen.getByRole('menuitem')).toHaveFocus();
      act(() => { vi.advanceTimersByTime(5000); });
      vi.useRealTimers();
    } else {
      await act(async () => reject(new Error('unavailable')));
    }
    expect(focus === 'other' ? other : trigger).toHaveFocus();
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(mocks.announce).toHaveBeenCalledWith('commandPalette.error', 'assertive');
  });

  it.each(['rejection', 'synchronous'])('announces catalog failure without executing a choice (%s)', async failure => {
    if (failure === 'synchronous') mocks.catalog.mockImplementation(() => { throw new Error('unavailable'); });
    else mocks.catalog.mockRejectedValue(new Error('unavailable'));
    render(<Harness />); await userEvent.click(screen.getByRole('button', { name: 'New provider' }));
    await waitFor(() => expect(mocks.announce).toHaveBeenCalledWith('commandPalette.error', 'assertive'));
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(mocks.open).not.toHaveBeenCalled();
  });
});
