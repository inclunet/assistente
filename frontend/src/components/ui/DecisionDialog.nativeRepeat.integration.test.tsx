import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, waitFor } from '@testing-library/react';
import { DecisionDialog } from './DecisionDialog';

const mocks = vi.hoisted(() => {
  const eventListeners = new Map<string, (payload: unknown) => void>();
  const eventCleanups = new Map<string, ReturnType<typeof vi.fn>>();
  const eventsOn = vi.fn((name: string, listener: (payload: unknown) => void) => {
    eventListeners.set(name, listener);
    const cleanup = vi.fn(() => {
      if (eventListeners.get(name) === listener) eventListeners.delete(name);
    });
    eventCleanups.set(name, cleanup);
    return cleanup;
  });
  const acquireGlobalCommandOwnership = vi.fn(() => ({
    isReady: () => true,
    owns: () => true,
    dispose: vi.fn(),
  }));
  return { announceRequest: vi.fn(), eventListeners, eventCleanups, eventsOn, acquireGlobalCommandOwnership };
});

vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: mocks.eventsOn }));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: mocks.acquireGlobalCommandOwnership }));
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));
vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announceRequest: mocks.announceRequest }),
}));
vi.mock('../../services/audioFeedback', () => ({
  playSound: vi.fn(),
  SOUND_TYPES: { ALERT: 'alert' },
}));
vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: { config: { decisionAlertSound: boolean } }) => unknown) =>
    selector({ config: { decisionAlertSound: false } }),
}));

function installWailsAPI() {
  const api = {
    OpenDecisionRepeatHotkeySession: vi.fn(async () => 'session-integration'),
    SetDecisionRepeatHotkey: vi.fn(async (_sessionID: string, _revision: number, _dialogID: string) => undefined),
    CloseDecisionRepeatHotkeySession: vi.fn(async () => undefined),
  };
  Object.assign(window, { go: { app: { App: api } } });
  return api;
}

describe('DecisionDialog + Modal native repeat integration', () => {
  afterEach(() => {
    document.body.innerHTML = '';
    mocks.eventListeners.clear();
    mocks.eventCleanups.clear();
    mocks.announceRequest.mockClear();
    mocks.eventsOn.mockClear();
    mocks.acquireGlobalCommandOwnership.mockClear();
    delete (window as Window & { go?: unknown }).go;
  });

  it('registra o topo real, reanuncia pelo evento e limpa a sessão sem onAction', async () => {
    const api = installWailsAPI();
    const onAction = vi.fn();
    const view = render(
      <DecisionDialog
        isOpen
        title="Pergunta nativa"
        description="Confirme a operação"
        actions={[{ id: 'confirm', label: 'Confirmar', primary: true }]}
        onAction={onAction}
        onCancel={vi.fn()}
      />,
    );

    await waitFor(() => expect(api.SetDecisionRepeatHotkey).toHaveBeenCalledOnce());
    const [, revision, dialogId] = api.SetDecisionRepeatHotkey.mock.calls[0];
    expect(mocks.eventListeners.has('command:decision-repeat')).toBe(true);
    const initialAnnouncements = mocks.announceRequest.mock.calls.length;

    mocks.eventListeners.get('command:decision-repeat')?.({
      sessionId: 'session-integration',
      revision,
      dialogId,
    });

    expect(onAction).not.toHaveBeenCalled();
    expect(mocks.announceRequest).toHaveBeenCalledTimes(initialAnnouncements + 1);

    view.unmount();
    await waitFor(() => expect(api.CloseDecisionRepeatHotkeySession).toHaveBeenCalledWith('session-integration'));
    expect(mocks.eventCleanups.get('command:decision-repeat')).toHaveBeenCalledOnce();
  });
});
