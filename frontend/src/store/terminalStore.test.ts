import { beforeEach, describe, expect, it, vi } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { resolveTerminalCommandId, useTerminalStore } from './terminalStore';
import { useAuthStore } from './authStore';
import { ListTerminalSessions } from '@wailsjs/go/wailsapi/Terminal';

const mockGetTerminalHistory = vi.fn();
const terminalEventHandlers = vi.hoisted(() => new Map<string, (data: unknown) => void>());

vi.mock('@wailsjs/go/wailsapi/Terminal', () => ({
  ListTerminalSessions: vi.fn().mockResolvedValue([]),
  CreateTerminalSession: vi.fn().mockResolvedValue(null),
  CloseTerminalSession: vi.fn().mockResolvedValue(undefined),
  SendTerminalInput: vi.fn().mockResolvedValue(undefined),
  InterruptTerminalCommand: vi.fn().mockResolvedValue(undefined),
  GetTerminalHistory: (...args: unknown[]) => mockGetTerminalHistory(...args),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn((event: string, handler: (data: unknown) => void) => {
    terminalEventHandlers.set(event, handler);
    return () => {
      if (terminalEventHandlers.get(event) === handler) terminalEventHandlers.delete(event);
    };
  }),
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
  it('ignora listagem após troca de sessão, inclusive ABA', async () => {
    const original = useAuthStore.getState();
    const pending = createDeferred<terminal.SessionInfo[]>();
    vi.mocked(ListTerminalSessions).mockReturnValueOnce(pending.promise);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'a', sessionId: 'a', role: 'user' } });
    const request = useTerminalStore.getState().loadSessions();
    useAuthStore.setState({ user: { userId: 'b', sessionId: 'b', role: 'user' } });
    useAuthStore.setState({ user: { userId: 'a', sessionId: 'a', role: 'user' } });
    pending.resolve([terminal.SessionInfo.createFrom({ id: 'stale' })]);
    try {
      expect(await request).toBe(false);
      expect(useTerminalStore.getState().sessions).toEqual([]);
    } finally { useAuthStore.setState(original); }
  });
  it('uma listagem antiga não sobrescreve uma resposta mais recente', async () => {
    const pending = createDeferred<terminal.SessionInfo[]>();
    vi.mocked(ListTerminalSessions).mockReturnValueOnce(pending.promise).mockResolvedValueOnce([terminal.SessionInfo.createFrom({ id: 'new' })]);
    const older = useTerminalStore.getState().loadSessions();
    expect(await useTerminalStore.getState().loadSessions()).toBe(true);
    pending.resolve([terminal.SessionInfo.createFrom({ id: 'old' })]);
    expect(await older).toBe(false);
    expect(useTerminalStore.getState().sessions.map(session => session.id)).toEqual(['new']);
  });
  beforeEach(() => {
    mockGetTerminalHistory.mockReset();
    terminalEventHandlers.clear();
    useTerminalStore.setState({
      sessions: [],
      historyBySession: {},
      activeEntryBySession: {},
      isLoadingSessions: false,
      loadingHistoryBySession: {},
    });
  });

  it('trata session_created duplicado como no-op sem resetar histórico', () => {
    const existingEntry = terminal.HistoryEntry.createFrom({
      id: 'entry-1', command: 'pwd', output: '', exitCode: 0,
      startedAt: '', endedAt: '', source: 'test',
    });
    const session = terminal.SessionInfo.createFrom({
      id: 'session-1',
      name: 'Terminal 1',
      cwd: 'C:\\workspace',
      state: 'idle',
      shell: 'powershell',
      createdAt: '2026-05-04T00:00:00.000Z',
      lastUsed: '2026-05-04T00:00:00.000Z',
    });
    useTerminalStore.setState({
      sessions: [session],
      historyBySession: { 'session-1': [existingEntry] },
    });
    const cleanup = useTerminalStore.getState().setupEventListeners();
    terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({
      ...session,
      name: 'Nome atualizado',
    }));

    const state = useTerminalStore.getState();
    expect(state.sessions).toHaveLength(1);
    expect(state.sessions[0]).toBe(session);
    expect(state.historyBySession['session-1']).toEqual([existingEntry]);
    cleanup();
  });

  it('adiciona session_created novo sem sobrescrever histórico pré-existente', () => {
    const existingEntry = terminal.HistoryEntry.createFrom({
      id: 'old-entry', command: 'old', output: '', exitCode: 0,
      startedAt: '', endedAt: '', source: 'test',
    });
    const cleanup = useTerminalStore.getState().setupEventListeners();
    useTerminalStore.setState({ historyBySession: { 'session-new': [existingEntry] } });
    terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({
      id: 'session-new',
      name: 'Terminal novo',
      cwd: 'C:\\workspace',
      state: 'idle',
      shell: 'powershell',
      createdAt: '2026-05-04T00:00:00.000Z',
      lastUsed: '2026-05-04T00:00:00.000Z',
    }));

    const state = useTerminalStore.getState();
    expect(state.sessions.map((session) => session.id)).toEqual(['session-new']);
    expect(state.historyBySession['session-new']).toEqual([existingEntry]);
    cleanup();
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

  it('gera IDs distintos para eventos legados sem commandId', () => {
    const first = resolveTerminalCommandId('session-1');
    const second = resolveTerminalCommandId('session-1');

    expect(first).not.toBe(second);
    expect(resolveTerminalCommandId('session-1', 'cmd-authoritative')).toBe('cmd-authoritative');
  });
});
