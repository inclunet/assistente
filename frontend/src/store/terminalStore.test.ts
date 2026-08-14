import { beforeEach, describe, expect, it, vi } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { useTerminalStore } from './terminalStore';

const mockGetTerminalHistory = vi.fn();

vi.mock('@wailsjs/go/wailsapi/Terminal', () => ({
  ListTerminalSessions: vi.fn().mockResolvedValue([]),
  CreateTerminalSession: vi.fn().mockResolvedValue(null),
  CloseTerminalSession: vi.fn().mockResolvedValue(undefined),
  SendTerminalInput: vi.fn().mockResolvedValue(undefined),
  InterruptTerminalCommand: vi.fn().mockResolvedValue(undefined),
  GetTerminalHistory: (...args: unknown[]) => mockGetTerminalHistory(...args),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => vi.fn()),
}));

vi.mock('../services/audioFeedback', () => ({
  playSendSound: vi.fn(),
  playReceiveSound: vi.fn(),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

describe('terminalStore', () => {
  beforeEach(() => {
    mockGetTerminalHistory.mockReset();
    useTerminalStore.setState({
      sessions: [],
      historyBySession: {},
      activeEntryBySession: {},
      isLoadingSessions: false,
      loadingHistoryBySession: {},
    });
  });

  it('não reintroduz histórico ou loading quando a sessão fecha durante loadHistory', async () => {
    const historyRequest = createDeferred<unknown[]>();
    mockGetTerminalHistory.mockReturnValue(historyRequest.promise);
    useTerminalStore.setState({
      sessions: [terminal.SessionInfo.createFrom({
        id: 'session-1',
        name: 'Terminal 1',
        cwd: 'C:\\workspace',
        state: 'idle',
        shell: 'powershell',
        createdAt: '2026-05-04T00:00:00.000Z',
        lastUsed: '2026-05-04T00:00:00.000Z',
      })],
      historyBySession: { 'session-1': [] },
      loadingHistoryBySession: {},
    });

    const loadPromise = useTerminalStore.getState().loadHistory('session-1');
    expect(useTerminalStore.getState().loadingHistoryBySession['session-1']).toBe(true);

    useTerminalStore.setState({
      sessions: [],
      historyBySession: {},
      activeEntryBySession: {},
      loadingHistoryBySession: {},
    });
    historyRequest.resolve([
      { id: 'history-1', command: 'pwd', output: 'ok' },
    ]);
    await loadPromise;

    expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('session-1');
    expect(useTerminalStore.getState().loadingHistoryBySession).not.toHaveProperty('session-1');
  });
});
