import { beforeEach, describe, expect, it, vi } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { resolveTerminalCommandId, useTerminalStore } from './terminalStore';
import { useAuthStore } from './authStore';
import { ListTerminalSessions } from '@wailsjs/go/wailsapi/Terminal';
import { announce } from '../hooks/useAnnouncer';
import i18next from 'i18next';

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

  it('não restaura sessão encerrada por listagem que já estava em andamento', async () => {
    const pending = createDeferred<terminal.SessionInfo[]>();
    vi.mocked(ListTerminalSessions).mockReturnValueOnce(pending.promise).mockResolvedValueOnce([terminal.SessionInfo.createFrom({ id: 'alive', name: 'Outro' })]);
    const cleanup = useTerminalStore.getState().setupEventListeners();
    try {
      useTerminalStore.setState({ sessions: [terminal.SessionInfo.createFrom({ id: 'stale', name: 'Terminal' })] });
      const request = useTerminalStore.getState().loadSessions();
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 'stale' });
      pending.resolve([terminal.SessionInfo.createFrom({ id: 'stale' })]);
      expect(await request).toBe(true);
      expect(useTerminalStore.getState().sessions.map((session) => session.id)).toEqual(['alive']);
      expect(useTerminalStore.getState().isLoadingSessions).toBe(false);
    } finally { cleanup(); }
  });

  it('não considera válida a relistagem substituta após troca de usuário', async () => {
    const owner = useAuthStore.getState();
    const original = createDeferred<terminal.SessionInfo[]>();
    const replacement = createDeferred<terminal.SessionInfo[]>();
    vi.mocked(ListTerminalSessions).mockReturnValueOnce(original.promise).mockReturnValueOnce(replacement.promise);
    useAuthStore.setState({ isAuthenticated: true, user: { userId: 'a', sessionId: 'a', role: 'user' } });
    const cleanup = useTerminalStore.getState().setupEventListeners();
    try {
      const request = useTerminalStore.getState().loadSessions();
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 'closed' });
      original.resolve([terminal.SessionInfo.createFrom({ id: 'closed' })]);
      await Promise.resolve();
      useAuthStore.setState({ user: { userId: 'b', sessionId: 'b', role: 'user' } });
      replacement.resolve([terminal.SessionInfo.createFrom({ id: 'private-a' })]);
      expect(await request).toBe(false);
      expect(useTerminalStore.getState().sessions).toEqual([]);
    } finally { cleanup(); useAuthStore.setState(owner); }
  });

  it('escuta encerramentos em segundo plano e carrega o histórico ao abrir a aba', async () => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    vi.mocked(announce).mockClear();
    try {
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 's1', name: 'Principal' }));
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'c1', command: 'pwd', source: 'llm' });
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: 'saída' });
      terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 's1', commandId: 'c1', output: 'saída', exitCode: 0 });
      vi.runAllTimers();
      expect(announce).not.toHaveBeenCalled();
      expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('s1');
      const entry = terminal.HistoryEntry.createFrom({ id: 'antigo', command: 'antigo', output: 'histórico' });
      mockGetTerminalHistory.mockResolvedValueOnce([entry]);
      const pageCleanup = useTerminalStore.getState().setupEventListeners();
      await useTerminalStore.getState().loadHistory('s1');
      expect(useTerminalStore.getState().historyBySession.s1).toEqual([entry]);
      pageCleanup();
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's1' });
      expect(announce).toHaveBeenCalledTimes(1);
    } finally { cleanup(); vi.useRealTimers(); }
  });

  it('anuncia saída espontânea uma vez e ignora eventos tardios sem bloquear comando antecipado', () => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners();
    vi.mocked(announce).mockClear();
    try {
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'early', command: 'pwd', source: 'llm' });
      expect(useTerminalStore.getState().historyBySession.s1).toHaveLength(1);
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 's1', name: 'Principal' }));
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: 'pendente' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's1', terminalId: 's1', error: 'falha' });
      vi.runAllTimers();
      expect(useTerminalStore.getState().sessions).toEqual([]);
      expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('s1');
      expect(announce).toHaveBeenCalledTimes(1);
      expect(announce).toHaveBeenCalledWith(i18next.t('terminal.announce.exited', { name: 'Principal' }));
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's1' });
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's1' });
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'late', command: 'pwd', source: 'llm' });
      terminalEventHandlers.get('terminal:command_output')?.({ sessionId: 's1', commandId: 'late', output: 'tardio' });
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: 'tardio' });
      terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 's1', commandId: 'late', output: 'tardio', exitCode: 1 });
      expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('s1');
      expect(useTerminalStore.getState().activeEntryBySession).not.toHaveProperty('s1');
      expect(announce).toHaveBeenCalledTimes(1);

      useTerminalStore.setState({ sessions: [terminal.SessionInfo.createFrom({ id: 's2', name: 'Outra' })] });
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's2' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's2' });
      expect(useTerminalStore.getState().sessions).toEqual([]);
      expect(announce).toHaveBeenCalledTimes(2);
      expect(announce).toHaveBeenLastCalledWith(i18next.t('terminal.announce.exited', { name: 'Outra' }));
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's3' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's3' });
      expect(announce).toHaveBeenCalledTimes(3);
      expect(announce).toHaveBeenLastCalledWith(i18next.t('terminal.announce.exited', { name: 's3' }));
    } finally { cleanup(); vi.useRealTimers(); }
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
