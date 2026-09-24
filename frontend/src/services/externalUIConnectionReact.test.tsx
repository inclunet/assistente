import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ExternalUIConnectionProvider, useExternalUIConnection } from './externalUIConnectionReact';
import {
  EXTERNAL_COMMAND_READY_EVENT,
  createExternalUIConnectionService,
  type ExternalUICommandReadyEvent,
  type ExternalUIConnectionPort,
  type ExternalUIConnectionService,
  type ExternalUIConnectionStatus,
  type ExternalUIEventsOn,
  type ExternalUIOwnerProof,
} from './externalUIConnection';
import type { ExternalUIDestination } from './externalUIConnection';
import { ExternalCommandConnection } from '../components/commands/ExternalCommandConnection';

const state = vi.hoisted(() => ({ authenticated: true, user: 'user-a', session: 'session-a', workspace: 'workspace-a' }));
vi.mock('../store/authStore', () => ({ useAuthStore: (selector: (value: unknown) => unknown) => selector({
  isAuthenticated: state.authenticated, user: { userId: state.user, sessionId: state.session },
}) }));
vi.mock('../store/workspaceStore', () => ({ useWorkspaceStore: (selector: (value: unknown) => unknown) => selector({
  workspace: { id: state.workspace },
}) }));
vi.mock('./externalUIConnectionWails', () => ({ createWailsExternalUIConnectionService: vi.fn() }));

function fixture(connectionState: ExternalUIConnectionStatus['state'] = 'disconnected') {
  const snapshot: ExternalUIConnectionStatus = {
    state: connectionState,
    owner: { userId: state.user, sessionId: state.session, workspaceId: state.workspace },
    target: null,
  };
  const service: ExternalUIConnectionService = {
    refresh: vi.fn(async () => snapshot), heartbeat: vi.fn(async () => snapshot),
    begin: vi.fn(), publishContext: vi.fn(), disconnect: vi.fn(), take: vi.fn(), complete: vi.fn(),
    getSnapshot: () => snapshot, subscribe: vi.fn(() => () => undefined),
    subscribeReady: vi.fn(() => () => undefined), dispose: vi.fn(),
  };
  return service;
}

function Probe() {
  const { service, target, setTarget } = useExternalUIConnection();
  return <>
    <span data-testid="connection">{service ? 'mounted' : 'absent'}</span>
    <span data-testid="target">{target?.surface.surfaceId ?? 'none'}</span>
    <button onClick={() => setTarget({ workspaceId: state.workspace, surface: {
      surfaceId: 'surface-a', surfaceType: 'settings', snapshotVersion: 'snapshot-a',
    } })}>select</button>
  </>;
}

function ConnectionPanel({ destination }: { destination: ExternalUIDestination }) {
  const { service, target, setTarget } = useExternalUIConnection();
  return <>
    <button onClick={() => setTarget(destination)}>select authorized destination</button>
    <ExternalCommandConnection service={service} target={target} />
  </>;
}

function eventBus() {
  const listeners = new Map<string, Set<(payload: unknown) => void>>();
  const on: ExternalUIEventsOn = (name, listener) => {
    const group = listeners.get(name) ?? new Set();
    group.add(listener);
    listeners.set(name, group);
    return () => group.delete(listener);
  };
  return { on, emit: (name: string, payload: unknown) => {
    for (const listener of listeners.get(name) ?? []) listener(payload);
  } };
}

describe('ExternalUIConnectionProvider', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    Object.assign(state, { authenticated: true, user: 'user-a', session: 'session-a', workspace: 'workspace-a' });
  });
  afterEach(() => { cleanup(); vi.useRealTimers(); });

  it('does not create a connection without an authenticated destination', () => {
    state.authenticated = false;
    const createService = vi.fn(() => fixture());
    render(<ExternalUIConnectionProvider createService={createService}><Probe /></ExternalUIConnectionProvider>);
    expect(createService).not.toHaveBeenCalled();
    expect(screen.getByTestId('connection')).toHaveTextContent('absent');
  });

  it('reads once but never pairs or polls a disconnected interface', async () => {
    const service = fixture();
    render(<ExternalUIConnectionProvider createService={() => service}><Probe /></ExternalUIConnectionProvider>);
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(service.refresh).toHaveBeenCalledTimes(1);
    expect(service.begin).not.toHaveBeenCalled();
    expect(service.heartbeat).not.toHaveBeenCalled();
  });

  it('renews only an active lease and stops renewing on unmount', async () => {
    const service = fixture('connected');
    const view = render(<ExternalUIConnectionProvider createService={() => service}><Probe /></ExternalUIConnectionProvider>);
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(service.heartbeat).toHaveBeenCalledTimes(1);
    view.unmount();
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(service.heartbeat).toHaveBeenCalledTimes(1);
    expect(service.dispose).toHaveBeenCalledTimes(1);
  });

  it('disposes the old owner and never carries its target into another workspace', () => {
    const first = fixture();
    const second = fixture();
    const createService = vi.fn().mockReturnValueOnce(first).mockReturnValueOnce(second);
    const view = render(<ExternalUIConnectionProvider createService={createService}><Probe /></ExternalUIConnectionProvider>);
    act(() => screen.getByRole('button', { name: 'select' }).click());
    expect(screen.getByTestId('target')).toHaveTextContent('surface-a');
    state.workspace = 'workspace-b';
    view.rerender(<ExternalUIConnectionProvider createService={createService}><Probe /></ExternalUIConnectionProvider>);
    expect(first.dispose).toHaveBeenCalledTimes(1);
    expect(createService).toHaveBeenCalledTimes(2);
    expect(screen.getByTestId('target')).toHaveTextContent('none');
    expect(second.refresh).toHaveBeenCalledTimes(1);
  });

  it('does not overlap heartbeats when the transport is still pending', async () => {
    const service = fixture('connected');
    let finish!: (value: ExternalUIConnectionStatus) => void;
    vi.mocked(service.heartbeat).mockImplementation(() => new Promise(resolve => { finish = resolve; }));
    const view = render(<ExternalUIConnectionProvider createService={() => service}><Probe /></ExternalUIConnectionProvider>);
    await act(async () => { await vi.advanceTimersByTimeAsync(30_000); });
    expect(service.heartbeat).toHaveBeenCalledTimes(1);
    view.unmount();
    await act(async () => { finish(service.getSnapshot()!); });
    expect(service.dispose).toHaveBeenCalledTimes(1);
    expect(service.refresh).toHaveBeenCalledTimes(1);
  });

  it('rejects a retained target setter after leaving and returning to the same workspace', () => {
    let retained: ReturnType<typeof useExternalUIConnection>['setTarget'] | undefined;
    function Capture() {
      const connection = useExternalUIConnection();
      retained ??= connection.setTarget;
      return <Probe />;
    }
    const createService = () => fixture();
    const tree = () => <ExternalUIConnectionProvider createService={createService}><Capture /></ExternalUIConnectionProvider>;
    const view = render(tree());
    act(() => screen.getByRole('button', { name: 'select' }).click());
    state.workspace = 'workspace-b';
    view.rerender(tree());
    act(() => retained?.({ workspaceId: 'workspace-a', surface: {
      surfaceId: 'obsolete', surfaceType: 'settings', snapshotVersion: 'old',
    } }));
    state.workspace = 'workspace-a';
    view.rerender(tree());
    expect(screen.getByTestId('target')).toHaveTextContent('none');
    act(() => retained?.({ workspaceId: 'workspace-a', surface: {
      surfaceId: 'obsolete', surfaceType: 'settings', snapshotVersion: 'old',
    } }));
    expect(screen.getByTestId('target')).toHaveTextContent('none');
  });

  it('integra consentimento, Begin, poll do claim e ready com o serviço real sobre uma porta fake', async () => {
    const owner: ExternalUIOwnerProof = {
      userId: state.user, sessionId: state.session, workspaceId: state.workspace,
    };
    const destination: ExternalUIDestination = {
      workspaceId: owner.workspaceId,
      tabId: 'tab-settings',
      surface: { surfaceType: 'settings', surfaceId: 'settings-commands', snapshotVersion: 'surface-1' },
    };
    const disconnected = {
      state: 'disconnected', owner,
      target: { workspaceId: '', surface: { surfaceType: '', surfaceId: '', snapshotVersion: '' } },
    };
    const waiting = { state: 'waiting_claim', owner, target: destination, expiresAt: '2099-01-01T00:00:00Z' };
    const connected: ExternalUIConnectionStatus = {
      state: 'connected', owner, target: destination,
      connectionId: '0190f7b2-7c00-7000-8000-000000000011', generation: '3',
      targetSnapshotId: '0190f7b2-7c00-7000-8000-000000000012',
      contextVersion: '0190f7b2-7c00-7000-8000-000000000013', expiresAt: '2099-01-01T00:00:00Z',
    };
    let authoritative: unknown = disconnected;
    const events = eventBus();
    const port: ExternalUIConnectionPort = {
      ReadExternalUIConnection: vi.fn(async () => authoritative),
      BeginExternalUIConnection: vi.fn(async () => {
        authoritative = waiting;
        return { invitation: 'claim-this-once', expiresAt: '2099-01-01T00:00:00Z' };
      }),
      PublishExternalUIContext: vi.fn(async () => connected),
      HeartbeatExternalUIConnection: vi.fn(async () => authoritative),
      DisconnectExternalUIConnection: vi.fn(async () => undefined),
      TakeExternalUICommand: vi.fn(async () => ({})),
      CompleteExternalUICommand: vi.fn(async () => ({ accepted: true })),
    };
    const service = createExternalUIConnectionService(port, () => owner, events);
    const createService = vi.fn(() => service);
    const readyListener = vi.fn();
    service.subscribeReady(readyListener);
    const view = render(
      <ExternalUIConnectionProvider createService={createService}>
        <ConnectionPanel destination={destination} />
      </ExternalUIConnectionProvider>,
    );

    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: 'select authorized destination' }));
      fireEvent.click(screen.getByRole('checkbox', { name: 'commandSettings.externalConnection.consent' }));
      fireEvent.click(screen.getByRole('button', { name: 'commandSettings.externalConnection.begin' }));
      await vi.waitFor(() => expect(service.getSnapshot()?.state).toBe('waiting_claim'));
    });
    expect(port.BeginExternalUIConnection).toHaveBeenCalledExactlyOnceWith(destination);
    expect(screen.getByText('commandSettings.externalConnection.state.waiting_claim')).toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'commandSettings.externalConnection.invitationLabel' }))
      .toHaveValue('claim-this-once');
    expect(screen.getByText('commandSettings.externalConnection.invitationCannotCancel')).toBeInTheDocument();

    authoritative = connected;
    await act(async () => { await vi.advanceTimersByTimeAsync(10_000); });
    expect(port.ReadExternalUIConnection).toHaveBeenCalledTimes(3);
    expect(screen.getByText('commandSettings.externalConnection.state.connected')).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'commandSettings.externalConnection.invitationLabel' })).not.toBeInTheDocument();

    const ready: ExternalUICommandReadyEvent = {
      connectionId: connected.connectionId!, generation: connected.generation!,
      invocationId: '0190f7b2-7c00-7000-8000-000000000014',
      targetSnapshotId: connected.targetSnapshotId!, contextVersion: connected.contextVersion!,
      commandId: 'navigation.settings.open',
    };
    events.emit(EXTERNAL_COMMAND_READY_EVENT, ready);
    expect(readyListener).toHaveBeenCalledExactlyOnceWith(ready);
    view.unmount();
  });
});
