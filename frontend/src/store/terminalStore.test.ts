import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import { terminal } from '../../wailsjs/go/models';
import { useTerminalStore } from './terminalStore';
import { useWorkspaceStore, type WorkspaceData } from './workspaceStore';
import { isVoiceAccessibilityOriginActive } from '../services/voiceAccessibility/types';
import { useAuthStore } from './authStore';
import { ListTerminalSessions, SendTerminalInput } from '@wailsjs/go/wailsapi/Terminal';
import { announceWithOrigin, registerVoiceAccessibilityActiveResolver, registerAnnouncerSink, unregisterAnnouncerSink } from '../services/voiceAccessibility/announcerBroker';
import en from '../locales/en';
import ptBR from '../locales/pt-BR';
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

const announce = vi.fn();

function createDeferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((promiseResolve) => {
    resolve = promiseResolve;
  });
  return { promise, resolve };
}

describe('terminalStore', () => {
  afterEach(unregisterAnnouncerSink);
  it('informa se o backend aceitou a entrada do terminal', async () => {
    const { sendInput } = useTerminalStore.getState();
    vi.mocked(SendTerminalInput).mockRejectedValueOnce(new Error('encerrado'));
    expect(await sendInput('s1', 'pwd')).toBe(false);
    expect(await sendInput('s1', 'pwd')).toBe(true);
  });
  it('diferencia falha na listagem de uma consulta vazia', async () => {
    vi.mocked(ListTerminalSessions).mockRejectedValueOnce(new Error('indisponível'));
    const { loadSessions } = useTerminalStore.getState();
    expect(await loadSessions()).toBe(false);
    expect(useTerminalStore.getState().sessionListFailed).toBe(true);
    expect(await loadSessions()).toBe(true);
    expect(useTerminalStore.getState().sessionListFailed).toBe(false);
  });
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
  beforeAll(() => {
    i18next.addResourceBundle('pt-BR', 'translation', { terminal: ptBR.translation.terminal }, true, true);
    i18next.addResourceBundle('en', 'translation', { terminal: en.translation.terminal }, true, true);
  });

  beforeEach(() => {
    registerAnnouncerSink(message => announce(message));
    announce.mockClear();
    mockGetTerminalHistory.mockReset();
    terminalEventHandlers.clear();
    useTerminalStore.setState({
      sessions: [],
      historyBySession: {},
      partialHistoryBySession: {},
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

  it.each([true, false])('abre a aba durante o comando e preserva o resultado (criação antes do início: %s)', async createdFirst => {
    const lifecycle = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    const emit = (event: string, payload: object) => terminalEventHandlers.get('terminal:' + event)?.(payload);
    const data = { sessionId: 's1', commandId: 'c1' };
    try {
      const created = () => emit('session_created', terminal.SessionInfo.createFrom({ id: 's1', state: 'idle' }));
      if (createdFirst) created();
      emit('command_start', { ...data, command: 'date', source: 'llm' });
      if (!createdFirst) created();
      emit('command_output', { ...data, output: 'antes' });
      expect(useTerminalStore.getState().sessions[0].state).toBe('running');
      const snapshot = createDeferred<terminal.HistoryEntry[]>();
      mockGetTerminalHistory.mockReturnValueOnce(snapshot.promise);
      const page = useTerminalStore.getState().setupEventListeners();
      try {
        const load = useTerminalStore.getState().loadHistory('s1');
        expect(useTerminalStore.getState().partialHistoryBySession.s1).toBe(true);
        expect(useTerminalStore.getState().historyBySession.s1[0].output).toBe('antes');
        emit('command_end', { ...data, output: 'antes e depois', exitCode: 7 });
        snapshot.resolve([]);
        await load;
        expect(useTerminalStore.getState().historyBySession.s1).toEqual([expect.objectContaining({ id: 'c1', exitCode: 7, output: 'antes e depois' })]);
        expect(useTerminalStore.getState().sessions[0].state).toBe('idle');
        expect(useTerminalStore.getState().partialHistoryBySession.s1).toBeUndefined();
      } finally { page(); }
    } finally { lifecycle(); }
  });

  it.each([
    { chunk: 'local', stored: '', final: 'local!' },
    { chunk: 'local', stored: 'histórico', final: 'histórico!' },
    { chunk: 'a'.repeat(51197) + '🌍', stored: '', final: 'a'.repeat(51197) + '…' },
  ])('preserva a saída e o limite do stream manual durante a hidratação (caso $#)', async ({ chunk, stored, final }) => {
    const cleanup = useTerminalStore.getState().setupEventListeners();
    const snapshot = createDeferred<terminal.HistoryEntry[]>();
    mockGetTerminalHistory.mockReturnValueOnce(snapshot.promise);
    try {
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 's1' }));
      const load = useTerminalStore.getState().loadHistory('s1');
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'c', command: 'pwd', source: 'user-raw' });
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: chunk });
      snapshot.resolve([terminal.HistoryEntry.createFrom({ id: 'c', command: 'pwd', source: 'user-raw', output: stored, endedAt: '0001-01-01T00:00:00Z' })]);
      await load;
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: '!' });
      expect(useTerminalStore.getState().historyBySession.s1).toEqual([expect.objectContaining({ id: 'c', output: final })]);
    } finally { cleanup(); }
  });

  it('mantém o cache parcial após falha para a próxima abertura recuperar o histórico', async () => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    try {
      const data = { sessionId: 'b', commandId: 'c', source: 'llm' };
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 'b' }));
      terminalEventHandlers.get('terminal:command_start')?.(data);
      mockGetTerminalHistory.mockRejectedValueOnce(new Error('indisponível'));
      await useTerminalStore.getState().loadHistory('b');
      expect(useTerminalStore.getState().partialHistoryBySession.b).toBe(true);
      mockGetTerminalHistory.mockResolvedValueOnce([terminal.HistoryEntry.createFrom({ id: 'anterior' })]);
      await useTerminalStore.getState().loadHistory('b');
      expect(useTerminalStore.getState().historyBySession.b.map(entry => entry.id)).toEqual(['anterior', 'c']);
      expect(useTerminalStore.getState().partialHistoryBySession.b).toBeUndefined();
    } finally { cleanup(); }
  });

  it.each([true, false])('preserva a saída antes da conclusão e da hidratação (criação antes de abrir a página: %s)', async createdFirst => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    try {
      const data = { sessionId: 'b', commandId: 'recente' };
      const create = () => terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 'b' }));
      if (createdFirst) create();
      terminalEventHandlers.get('terminal:command_start')?.({ ...data, command: 'date', source: 'llm' });
      terminalEventHandlers.get('terminal:command_output')?.({ ...data, output: 'hoje' });
      const closeOtherPage = useTerminalStore.getState().setupEventListeners();
      try {
        if (!createdFirst) create();
        terminalEventHandlers.get('terminal:command_end')?.({ ...data, output: '', exitCode: 0 });
        expect(useTerminalStore.getState().historyBySession.b[0].output).toBe('hoje');
        expect(announce).toHaveBeenCalledWith(i18next.t('terminal.announce.outputWithExit', { output: 'hoje', exitCode: 0 }));
        expect(useTerminalStore.getState().partialHistoryBySession.b).toBe(true);
        mockGetTerminalHistory.mockResolvedValueOnce([terminal.HistoryEntry.createFrom({ id: 'antigo' })]);
        await useTerminalStore.getState().loadHistory('b');
        expect(useTerminalStore.getState().historyBySession.b.map(entry => entry.id)).toEqual(['antigo', 'recente']);
        expect(useTerminalStore.getState().partialHistoryBySession.b).toBeUndefined();
      } finally { closeOtherPage(); }
    } finally { cleanup(); }
  });

  it.each([['command_start', true], ['command_start', false], ['command_end', true], ['command_end', false]] as const)('preserva %s recebido durante uma listagem (já no catálogo: %s)', async (event, known) => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    try {
      const start = event === 'command_start';
      const old = terminal.SessionInfo.createFrom({ id: 'b', state: start ? 'idle' : 'running' });
      useTerminalStore.setState({ sessions: known ? [old] : [] });
      const pending = createDeferred<terminal.SessionInfo[]>();
      vi.mocked(ListTerminalSessions).mockReturnValueOnce(pending.promise);
      const request = useTerminalStore.getState().loadSessions();
      terminalEventHandlers.get('terminal:' + event)?.({ sessionId: 'b', commandId: 'c', source: 'llm' });
      pending.resolve([old]);
      await request;
      expect(useTerminalStore.getState().sessions[0].state).toBe(start ? 'running' : 'idle');
    } finally { cleanup(); }
  });

  it('a consulta superada mantém os eventos da listagem mais recente', async () => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    try {
      const older = createDeferred<terminal.SessionInfo[]>();
      const latest = createDeferred<terminal.SessionInfo[]>();
      vi.mocked(ListTerminalSessions).mockReturnValueOnce(older.promise).mockReturnValueOnce(latest.promise);
      const first = useTerminalStore.getState().loadSessions();
      const second = useTerminalStore.getState().loadSessions();
      const start = (sessionId: string) => terminalEventHandlers.get('terminal:command_start')?.({ sessionId, commandId: 'c', source: 'llm' });
      start('antes');
      older.resolve([]);
      expect(await first).toBe(false);
      start('depois');
      latest.resolve(['antes', 'depois'].map(id => terminal.SessionInfo.createFrom({ id, state: 'idle' })));
      expect(await second).toBe(true);
      expect(useTerminalStore.getState().sessions.map(session => session.state)).toEqual(['running', 'running']);
    } finally { cleanup(); }
  });

  it('conserva comandos executados enquanto um histórico já carregado está fora da tela', async () => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    const closePage = useTerminalStore.getState().setupEventListeners();
    terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 's1' }));
    const entry = terminal.HistoryEntry.createFrom({ id: 'prev', command: 'pwd', exitCode: 0 });
    mockGetTerminalHistory.mockResolvedValueOnce([entry]);
    await useTerminalStore.getState().loadHistory('s1');
    closePage();
    try {
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'c1', command: 'date', source: 'llm' });
      terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 's1', commandId: 'c1', output: 'quarta', exitCode: 0 });
      expect(useTerminalStore.getState().historyBySession.s1).toEqual([
        entry, expect.objectContaining({ id: 'c1', output: 'quarta', exitCode: 0 }),
      ]);
      expect(useTerminalStore.getState().activeEntryBySession.s1).toBeNull();
      expect(announce).not.toHaveBeenCalled();
    } finally { cleanup(); }
  });

  it('conserva a conclusão com código -999 quando chega outro comando', () => {
    const cleanup = useTerminalStore.getState().setupEventListeners();
    const emit = (event: string, commandId: string, data: object) =>
      terminalEventHandlers.get('terminal:' + event)?.({ sessionId: 's1', commandId, ...data });
    try {
      emit('command_start', 'a', { command: 'a', source: 'llm' });
      emit('command_end', 'a', { output: 'primeiro', exitCode: -999 });
      emit('command_start', 'b', { command: 'b', source: 'llm' });
      emit('command_output', 'a', { output: 'evento atrasado' });
      emit('command_output', 'b', { output: 'segundo' });
      emit('command_end', 'b', { output: '', exitCode: 0 });
      expect(useTerminalStore.getState().historyBySession.s1).toEqual([
        expect.objectContaining({ id: 'a', exitCode: -999, output: 'primeiro' }),
        expect.objectContaining({ id: 'b', exitCode: 0, output: 'segundo' }),
      ]);
    } finally { cleanup(); }
  });

  it.each(['raw_output', 'command_output'])('limita entradas e bytes UTF-8 do histórico em segundo plano (%s)', (outputEvent) => {
    const cleanup = useTerminalStore.getState().setupEventListeners({ lifecycleOnly: true });
    try {
      useTerminalStore.setState({ historyBySession: { s1: [] } });
      const source = outputEvent === 'raw_output' ? 'user-raw' : 'llm';
      for (let i = 0; i <= 200; i++) {
        terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: String(i), command: 'echo', source });
        if (source === 'llm' && i < 200) {
          terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 's1', commandId: String(i), output: 'ok', exitCode: 0 });
        }
      }
      const emit = (output: string) => terminalEventHandlers.get('terminal:' + outputEvent)?.({ sessionId: 's1', commandId: '200', output });
      // Faltam cinco bytes: cabe o primeiro emoji, mas não o segundo.
      const prefix = 'é'.repeat(25597) + 'x';
      emit(prefix);
      emit('😀😀fim');
      emit('z');
      const history = useTerminalStore.getState().historyBySession.s1;
      expect(history).toHaveLength(200);
      expect(history[0].id).toBe('1');
      expect(history[199].output).toBe(prefix + '😀…');
    } finally { cleanup(); }
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

  it.each([
    { title: 'Build', before: '', after: '', expected: 'Build' },
    { title: '', before: '', after: '', expected: undefined },
    { title: 'Build', before: 'Antigo', after: 'Novo', expected: 'Novo' },
    { title: 'Build', before: 'Antigo', after: '', expected: 'Build' },
    { title: 'Build', before: 'Antigo', after: null, expected: 'Antigo' },
    { title: 'Build', before: '', after: null, expected: 'Build' },
    { title: '', before: '', after: null, expected: 'Terminal' },
    { title: 'Reserva', before: '', after: 'Atual', expected: 'Reserva', failLookup: true },
  ])('identifica a sessão na entrega (aba "$title", nome "$before" → "$after")', async ({ title, before, after, expected, failLookup }) => {
    const language = i18next.language;
    vi.useFakeTimers();
    const original = useWorkspaceStore.getState().workspace;
    const workspace: WorkspaceData = { id: 'w', name: 'W', activeTabId: 'tab-a', tabs: [
      { id: 'tab-a', type: 'terminal', title, position: 0, state: { sessionId: 'a' } },
      { id: 'tab-b', type: 'terminal', title: 'Logs', position: 1, state: { sessionId: 'b' } },
    ] };
    useWorkspaceStore.setState({ workspace });
    const reset = registerVoiceAccessibilityActiveResolver(origin =>
      isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace));
    const page = useTerminalStore.getState().setupEventListeners();
    try {
      announceWithOrigin({ message: 'Leitura em curso', protectsReading: true });
      const emit = (event: string, sessionId: string, output: string) =>
        terminalEventHandlers.get('terminal:' + event)?.({ sessionId, commandId: 'cmd', output, exitCode: 7 });
      emit('raw_output', 'b', 'inativa');
      emit('raw_output', 'a', 'prévia da aba anterior');
      vi.advanceTimersByTime(500);
      useTerminalStore.setState({ sessions: [terminal.SessionInfo.createFrom({ id: 'a', name: before })] });
      emit('command_end', 'a', 'detalhe da execução');
      if (!failLookup) await i18next.changeLanguage('pt-BR');
      useTerminalStore.setState({ sessions: after === null ? [] : [terminal.SessionInfo.createFrom({ id: 'a', name: after })] });
      useWorkspaceStore.setState({ workspace: { ...workspace, tabs: after === null ? workspace.tabs.slice(1) : [...workspace.tabs].reverse(), activeTabId: 'tab-b' } });
      const lookup = failLookup ? vi.spyOn(useTerminalStore.getState().sessions, 'find')
        .mockImplementationOnce(() => { throw new Error('catálogo indisponível'); }) : undefined;
      vi.runAllTimers();
      if (lookup) expect(lookup).toHaveBeenCalledTimes(1);
      expect(announce.mock.calls.map(([message]) => message)).toEqual([
        'Leitura em curso', i18next.t('terminal.announce.backgroundResult', { name: expected ?? i18next.t('terminal.announce.unnamedTab', { position: 2 }), exitCode: 7 }),
      ]);
    } finally { vi.restoreAllMocks(); page(); reset(); useWorkspaceStore.setState({ workspace: original }); vi.useRealTimers(); await i18next.changeLanguage(language); }
  });

  it.each(['evento final', 'saída acumulada', 'stream manual'].flatMap(path =>
    [{ tail: '', truncated: false }, { tail: '\n', truncated: false }, { tail: '\n z', truncated: true }]
      .map(test => ({ path, ...test }))))('preserva o emoji na posição 300 ($path, excedente "$tail")', ({ path, tail, truncated }) => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners();
    const prefix = 'a'.repeat(299) + '🌍';
    const event = { sessionId: 'a', commandId: 'cmd' };
    try {
      if (path === 'stream manual') {
        terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 'a', output: prefix.slice(0, -1) });
        terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 'a', output: prefix.slice(-1) + tail });
        vi.runAllTimers();
      } else {
        terminalEventHandlers.get('terminal:command_start')?.({ ...event, command: 'pwd', source: 'llm' });
        if (path === 'saída acumulada') terminalEventHandlers.get('terminal:command_output')?.({ ...event, output: prefix + tail });
        terminalEventHandlers.get('terminal:command_end')?.({ ...event, output: path === 'evento final' ? prefix + tail : '', exitCode: 0 });
      }
      const output = prefix + (truncated ? i18next.t('terminal.announce.truncatedSuffix') : '');
      const key = path === 'stream manual' ? 'terminal.announce.output' : 'terminal.announce.outputWithExit';
      expect(announce).toHaveBeenLastCalledWith(i18next.t(key, { output, exitCode: 0 }));
    } finally { cleanup(); vi.useRealTimers(); }
  });

  it('entrega o resultado ao leitor antes do encerramento e indica saída cortada após espaços', () => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners();
    try {
      for (const output of ['A' + ' '.repeat(300), '   ', 'B']) {
        terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 'a', output });
      }
      vi.runAllTimers();
      expect(announce.mock.calls[0][0]).toBe(i18next.t('terminal.announce.output', {
        output: 'A' + i18next.t('terminal.announce.truncatedSuffix'),
      }));
      terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 'a', commandId: 'cmd', output: 'fim', exitCode: 7 });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 'a' });
      expect(announce).toHaveBeenCalledTimes(1);
      vi.advanceTimersToNextTimer();
      expect(announce.mock.calls[1][0]).toContain('fim');
      expect(announce.mock.calls[1][0]).toContain('7');
      vi.runAllTimers();
      expect(announce.mock.calls[2][0]).toBe(i18next.t('terminal.announce.terminated'));
    } finally { cleanup(); vi.useRealTimers(); }
  });

  it('preserva o anúncio pendente de outra sessão ao receber saída ou iniciar um comando', () => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners();
    vi.mocked(announce).mockClear();
    try {
      const raw = terminalEventHandlers.get('terminal:raw_output');
      raw?.({ sessionId: 'a', output: 'primeira' });
      raw?.({ sessionId: 'b', output: 'anterior' });
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 'b', commandId: 'c1', command: 'pwd', source: 'llm' });
      raw?.({ sessionId: 'b', output: 'segunda' });
      vi.runAllTimers();
      expect(announce).toHaveBeenCalledTimes(3);
      expect(announce).toHaveBeenCalledWith(`${i18next.t('terminal.history.commandPrefix')} pwd`);
      for (const output of ['primeira', 'segunda']) {
        expect(announce).toHaveBeenCalledWith(i18next.t('terminal.announce.output', { output }));
      }
    } finally { cleanup(); vi.useRealTimers(); }
  });

  it('anuncia saída espontânea uma vez e ignora eventos tardios sem bloquear comando antecipado', () => {
    vi.useFakeTimers();
    const cleanup = useTerminalStore.getState().setupEventListeners();
    vi.mocked(announce).mockClear();
    try {
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'early', command: 'pwd', source: 'llm' });
      expect(useTerminalStore.getState().historyBySession.s1).toHaveLength(1);
      expect(announce.mock.calls).toEqual([[`${i18next.t('terminal.history.commandPrefix')} pwd`]]);
      announce.mockClear();
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 's1', name: 'Principal' }));
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: 'pendente' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's1', terminalId: 's1', error: 'falha' });
      vi.runAllTimers();
      expect(useTerminalStore.getState().sessions).toEqual([]);
      expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('s1');
      expect(announce).toHaveBeenCalledTimes(2);
      expect(announce).toHaveBeenNthCalledWith(1, i18next.t('terminal.announce.output', { output: 'pendente' }));
      expect(announce).toHaveBeenNthCalledWith(2, i18next.t('terminal.announce.exited', { name: 'Principal' }));
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's1' });
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's1' });
      terminalEventHandlers.get('terminal:command_start')?.({ sessionId: 's1', commandId: 'late', command: 'pwd', source: 'llm' });
      terminalEventHandlers.get('terminal:command_output')?.({ sessionId: 's1', commandId: 'late', output: 'tardio' });
      terminalEventHandlers.get('terminal:raw_output')?.({ sessionId: 's1', output: 'tardio' });
      terminalEventHandlers.get('terminal:command_end')?.({ sessionId: 's1', commandId: 'late', output: 'tardio', exitCode: 1 });
      expect(useTerminalStore.getState().historyBySession).not.toHaveProperty('s1');
      expect(useTerminalStore.getState().activeEntryBySession).not.toHaveProperty('s1');
      expect(announce).toHaveBeenCalledTimes(2);

      useTerminalStore.setState({ sessions: [terminal.SessionInfo.createFrom({ id: 's2', name: 'Outra' })] });
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's2' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's2' });
      expect(useTerminalStore.getState().sessions).toEqual([]);
      vi.runAllTimers();
      expect(announce).toHaveBeenCalledTimes(3);
      expect(announce).toHaveBeenLastCalledWith(i18next.t('terminal.announce.exited', { name: 'Outra' }));
      terminalEventHandlers.get('terminal:session_closed')?.({ sessionId: 's3' });
      terminalEventHandlers.get('terminal:exited')?.({ sessionId: 's3' });
      vi.runAllTimers();
      expect(announce).toHaveBeenCalledTimes(4);
      expect(announce).toHaveBeenLastCalledWith(i18next.t('terminal.announce.terminated'));
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
      partialHistoryBySession: {},
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

  it('anuncia só o início recebido ao vivo na sessão visível durante a hidratação', async () => {
    const cleanup = useTerminalStore.getState().setupEventListeners();
    const reset = registerVoiceAccessibilityActiveResolver(origin => !origin || origin.terminalSessionId === 'a');
    const snapshot = createDeferred<terminal.HistoryEntry[]>();
    mockGetTerminalHistory.mockReturnValueOnce(snapshot.promise);
    try {
      terminalEventHandlers.get('terminal:session_created')?.(terminal.SessionInfo.createFrom({ id: 'a' }));
      const load = useTerminalStore.getState().loadHistory('a');
      const start = (sessionId: string, id: string, source: string) =>
        terminalEventHandlers.get('terminal:command_start')?.({ sessionId, commandId: id, command: id, source });
      start('b', 'oculto', 'llm');
      start('a', 'novo', 'llm');
      snapshot.resolve(Array.from({ length: 200 }, (_, i) => terminal.HistoryEntry.createFrom({ id: String(i), command: 'antigo' })));
      await load;
      announceWithOrigin({ message: 'Leitura em curso', protectsReading: true });
      start('b', 'manual oculto', 'user-raw');
      start('a', 'manual', 'user-raw');
      const prefix = i18next.t('terminal.history.commandPrefix');
      expect(announce.mock.calls.map(([message]) => message)).toEqual([`${prefix} novo`, 'Leitura em curso', `${prefix} manual`]);
      expect(useTerminalStore.getState().historyBySession.a).toHaveLength(200);
    } finally { reset(); cleanup(); }
  });
});
