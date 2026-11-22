import { logger } from '../utils/logger';
import { create } from 'zustand';
import { useWorkspaceStore } from './workspaceStore';
import { useAuthStore } from './authStore';
import {
  ListTerminalSessions,
  CreateTerminalSession,
  CloseTerminalSession,
  SendTerminalInput,
  InterruptTerminalCommand,
  GetTerminalHistory,
} from '@wailsjs/go/wailsapi/Terminal';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { terminal } from '../../wailsjs/go/models';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { type VoiceAnnounceRequest, announceWithOrigin, isVoiceAccessibilityOriginCurrentlyActive } from '../services/voiceAccessibility/announcerBroker';
import i18next from 'i18next';

// Debounce para anúncio de output (acumula chunks e espera streaming parar)
const pendingAnnouncements = new Map<string, { output: string; truncated: boolean; timer: ReturnType<typeof setTimeout> }>();
let terminalEventListenerRefCount = 0;
let terminalOutputListenerRefCount = 0;
let terminalSessionReadGeneration = 0;
// Só a consulta mais recente pode publicar. Seus eventos podem preceder o primeiro catálogo.
let listingStates: Map<string, string> | undefined;
const terminalSessionReadReplacements = new Map<number, Promise<boolean>>();
let terminalEventListenerCleanup: (() => void) | null = null;
let legacyCommandSequence = 0;

export function resolveTerminalCommandId(sessionId: string, commandId?: string): string {
  return commandId || `legacy-${sessionId}-${Date.now()}-${legacyCommandSequence++}`;
}

function terminalOrigin(sessionId: string) {
  return { surfaceType: 'terminal' as const, terminalSessionId: sessionId };
}

function announceTerminalResult(sessionId: string, message: string, inactiveMessage: NonNullable<VoiceAnnounceRequest['inactiveMessage']>) {
  announceWithOrigin({ origin: terminalOrigin(sessionId), message, inactiveMessage, eventType: 'completion',
    waitsForReading: true, protectsReading: true });
}

function cancelOutputAnnouncement(sessionId: string) {
  const pending = pendingAnnouncements.get(sessionId);
  if (pending) clearTimeout(pending.timer);
  pendingAnnouncements.delete(sessionId);
}

function flushOutputAnnouncement(sessionId: string) {
  const pending = pendingAnnouncements.get(sessionId);
  if (!pending) return;
  cancelOutputAnnouncement(sessionId);
  const text = pending.output.trim();
  if (!text) return;
  const preview = pending.truncated ? text + i18next.t('terminal.announce.truncatedSuffix') : text;
  if (announceWithOrigin({ origin: terminalOrigin(sessionId), message: i18next.t('terminal.announce.output', { output: preview }),
    eventType: 'progress', waitsForReading: true, protectsReading: true })) playReceiveSound();
}

function scheduleOutputAnnounce(sessionId: string, chunk: string) {
  if (!isVoiceAccessibilityOriginCurrentlyActive(terminalOrigin(sessionId))) return;
  const pending = pendingAnnouncements.get(sessionId);
  const previous = pending?.output ?? '';
  cancelOutputAnnouncement(sessionId);
  const content = previous ? chunk : chunk.trimStart();
  const remaining = 300 - previous.length;
  const output = previous + content.slice(0, remaining);
  const truncated = pending?.truncated || (content.length > remaining && /\S/.test(content.slice(remaining)));
  const timer = setTimeout(() => flushOutputAnnouncement(sessionId), 500);
  pendingAnnouncements.set(sessionId, { output, truncated, timer });
}

// Re-export types for components
export type SessionInfo = terminal.SessionInfo;
export type HistoryEntry = terminal.HistoryEntry;

// Até 200 entradas e 50 KiB de conteúdo por entrada, como no histórico do backend.
// O marcador de corte é adicional: ‘…’ no stream e o aviso original no resultado final.
// Antes da primeira abertura, só o comando gerenciado em curso precisa acompanhar o stream.
// Hidratar, concluir ou encerrar a sessão remove sua entrada. O último listener limpa as restantes.
const unobservedCommands = new Map<string, HistoryEntry>();
const historyEntryLimit = 200;
const historyOutputLimit = 50 * 1024;
const historyEncoder = new TextEncoder();
const historyBuffer = new Uint8Array(historyOutputLimit);
const historyBytes = new WeakMap<HistoryEntry, number>();

function appendHistoryOutput(entry: HistoryEntry, chunk: string): HistoryEntry {
  if (!chunk) return entry;
  let used = historyBytes.get(entry);
  if (used === undefined) {
    const existing = historyEncoder.encodeInto(entry.output, historyBuffer);
    used = existing.read === entry.output.length ? existing.written : historyOutputLimit + 1;
    historyBytes.set(entry, used);
  }
  if (used > historyOutputLimit) return entry;
  const remaining = historyBuffer.subarray(0, historyOutputLimit - used);
  const { read, written } = historyEncoder.encodeInto(chunk, remaining);
  const truncated = read !== chunk.length;
  const addition = truncated ? chunk.slice(0, read) + '…' : chunk;
  const updated = terminal.HistoryEntry.createFrom({ ...entry, output: entry.output + addition });
  // Não permita que bytes seguintes ultrapassem um caractere que não coube.
  historyBytes.set(updated, truncated ? historyOutputLimit + 1 : used + written);
  return updated;
}

function changeSessionState(sessions: SessionInfo[], sessionId: string, state: string) {
  return sessions.map(session => session.id === sessionId ? terminal.SessionInfo.createFrom({ ...session, state }) : session);
}

interface TerminalState {
  sessions: SessionInfo[];
  historyBySession: Record<string, HistoryEntry[]>;
  partialHistoryBySession: Record<string, boolean>;
  /** ID do último entry que está recebendo raw output (modo interativo) */
  activeEntryBySession: Record<string, string | null>;
  isLoadingSessions: boolean;
  sessionListFailed: boolean;
  loadingHistoryBySession: Record<string, boolean>;

  // Actions
  loadSessions: () => Promise<boolean>;
  createSession: (name?: string) => Promise<string | null>;
  closeSession: (id: string) => Promise<boolean>;
  sendInput: (sessionId: string, input: string) => Promise<boolean>;
  interrupt: (sessionId: string) => Promise<void>;
  loadHistory: (sessionId: string) => Promise<void>;
  setupEventListeners: (options?: { lifecycleOnly?: boolean }) => () => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  historyBySession: {},
  partialHistoryBySession: {},
  activeEntryBySession: {},
  isLoadingSessions: false,
  sessionListFailed: false,
  loadingHistoryBySession: {},

  loadSessions: async () => {
    const observed = new Map<string, string>();
    listingStates = observed;
    const request = ++terminalSessionReadGeneration;
    const owner = useAuthStore.getState();
    let currentOwner = true;
    const unsubscribe = useAuthStore.subscribe(state => {
      if (state.isAuthenticated !== owner.isAuthenticated || state.user?.userId !== owner.user?.userId ||
          state.user?.sessionId !== owner.user?.sessionId) currentOwner = false;
    });
    set({ isLoadingSessions: true, sessionListFailed: false });
    try {
      const sessions = await ListTerminalSessions();
      if (!currentOwner) return false;
      if (request !== terminalSessionReadGeneration) {
        const replacement = terminalSessionReadReplacements.get(request);
        if (!replacement) return false;
        const refreshed = await replacement;
        return currentOwner && refreshed;
      }
      set({ sessions: (sessions || []).map(session => {
        const state = observed.get(session.id);
        return state ? terminal.SessionInfo.createFrom({ ...session, state }) : session;
      }) });
      return true;
    } catch (err) {
      const replacement = terminalSessionReadReplacements.get(request);
      if (currentOwner && replacement) {
        const refreshed = await replacement;
        return currentOwner && refreshed;
      }
      if (currentOwner && request === terminalSessionReadGeneration) set({ sessionListFailed: true });
      logger.error('[Terminal] Erro ao carregar sessões:', err);
      return false;
    } finally {
      unsubscribe();
      if (listingStates === observed) listingStates = undefined;
      terminalSessionReadReplacements.delete(request);
      if (request === terminalSessionReadGeneration) set({ isLoadingSessions: false });
    }
  },

  createSession: async (name?: string) => {
    try {
      const info = await CreateTerminalSession(name || '');
      if (info) {
        return info.id;
      }
      return null;
    } catch (err) {
      logger.error('[Terminal] Erro ao criar sessão:', err);
      return null;
    }
  },

  closeSession: async (id: string) => {
    try {
      await CloseTerminalSession(id);
      return true;
    } catch (err) {
      logger.error('[Terminal] Erro ao fechar sessão:', err);
      return false;
    }
  },

  sendInput: async (sessionId: string, input: string) => {
    if (!sessionId) return false;

    try {
      playSendSound();
      await SendTerminalInput(sessionId, input);
      return true;
    } catch (err) {
      logger.error('[Terminal] Erro ao enviar input:', err);
      return false;
    }
  },

  interrupt: async (sessionId: string) => {
    if (!sessionId) return;

    try {
      await InterruptTerminalCommand(sessionId);
    } catch (err) {
      logger.error('[Terminal] Erro ao interromper:', err);
    }
  },

  loadHistory: async (sessionId: string) => {
    const pending = unobservedCommands.get(sessionId);
    if (pending) unobservedCommands.delete(sessionId);
    set(state => ({
      ...(pending && {
        historyBySession: { ...state.historyBySession, [sessionId]: [pending] },
        partialHistoryBySession: { ...state.partialHistoryBySession, [sessionId]: true },
        activeEntryBySession: { ...state.activeEntryBySession, [sessionId]: pending.id },
      }),
      loadingHistoryBySession: {
        ...state.loadingHistoryBySession,
        [sessionId]: true,
      },
    }));
    try {
      const history = await GetTerminalHistory(sessionId);
      set(state => {
        const sessionExists = state.sessions.some(session => session.id === sessionId);
        if (!sessionExists) return state;

        const partial = { ...state.partialHistoryBySession };
        delete partial[sessionId];
        const entries = new Map((history || []).map(entry => [entry.id, entry]));
        for (const entry of state.historyBySession[sessionId] ?? []) {
          const stored = entries.get(entry.id);
          if (!stored || (!stored.endedAt && entry.endedAt)) entries.set(entry.id, entry);
        }
        return {
          historyBySession: {
            ...state.historyBySession,
            [sessionId]: Array.from(entries.values()).slice(-historyEntryLimit),
          },
          partialHistoryBySession: partial,
        };
      });
    } catch (err) {
      logger.error('[Terminal] Erro ao carregar histórico:', err);
    } finally {
      set(state => {
        const nextLoading = { ...state.loadingHistoryBySession };
        const sessionExists = state.sessions.some(session => session.id === sessionId);
        if (sessionExists) {
          nextLoading[sessionId] = false;
        } else {
          delete nextLoading[sessionId];
        }
        return { loadingHistoryBySession: nextLoading };
      });
    }
  },

  setupEventListeners: (options) => {
    terminalEventListenerRefCount += 1;
    if (!options?.lifecycleOnly) terminalOutputListenerRefCount += 1;
    if (terminalEventListenerCleanup) {
      return () => {
        terminalEventListenerRefCount = Math.max(0, terminalEventListenerRefCount - 1);
        if (!options?.lifecycleOnly) {
          terminalOutputListenerRefCount = Math.max(0, terminalOutputListenerRefCount - 1);
          if (terminalOutputListenerRefCount === 0) pendingAnnouncements.forEach((_, id) => cancelOutputAnnouncement(id));
        }
        if (terminalEventListenerRefCount === 0 && terminalEventListenerCleanup) {
          terminalEventListenerCleanup();
          terminalEventListenerCleanup = null;
        }
      };
    }

    const unsubs: Array<() => void> = [];
    // Uma sessão pode produzir comandos antes do evento de criação chegar.
    // Só ignoramos eventos depois de seu encerramento explícito ou espontâneo.
    const endedSessions = new Set<string>();
    // Uma sessão já exibida conserva seu histórico entre montagens da página.
    // Continue aplicando eventos ao cache mesmo quando os anúncios estão suspensos.
    const tracksHistory = (sessionId: string) =>
      !endedSessions.has(sessionId) && (terminalOutputListenerRefCount > 0 ||
        useTerminalStore.getState().historyBySession[sessionId] !== undefined);

    // Sessão criada
    unsubs.push(EventsOn('terminal:session_created', (data: SessionInfo) => {
      if (endedSessions.has(data.id) || useTerminalStore.getState().sessions.some((session) => session.id === data.id)) return;
      const refresh = useTerminalStore.getState().isLoadingSessions;
      const superseded = terminalSessionReadGeneration;
      terminalSessionReadGeneration += 1;
      set(state => {
        // O evento pode cruzar com loadSessions(); a sessão já conhecida é
        // idempotente e não pode resetar histórico nem produzir uma segunda
        // entrada visual.
        if (state.sessions.some((session) => session.id === data.id)) return state;
        return {
          sessions: [...state.sessions, unobservedCommands.has(data.id)
            ? terminal.SessionInfo.createFrom({ ...data, state: 'running' }) : data],
          isLoadingSessions: false,
          historyBySession: terminalOutputListenerRefCount > 0
            ? { ...state.historyBySession, [data.id]: state.historyBySession[data.id] ?? [] }
            : state.historyBySession,
        };
      });
      if (refresh) terminalSessionReadReplacements.set(superseded, useTerminalStore.getState().loadSessions());
    }));

    // Sessão fechada
    const removeExitedSession = (data: { sessionId?: string; terminalId?: string }) => {
      const sessionId = data.terminalId || data.sessionId;
      if (!sessionId || endedSessions.has(sessionId)) return;
      const refresh = useTerminalStore.getState().isLoadingSessions;
      const superseded = terminalSessionReadGeneration;
      terminalSessionReadGeneration += 1;
      // IDs são únicos; limite a memória usada para descartar eventos tardios.
      unobservedCommands.delete(sessionId);
      endedSessions.add(sessionId);
      if (endedSessions.size > 1024) endedSessions.delete(endedSessions.values().next().value!);
      flushOutputAnnouncement(sessionId);
      set(state => {
        const newHistory = { ...state.historyBySession };
        delete newHistory[sessionId];
        const partial = { ...state.partialHistoryBySession };
        delete partial[sessionId];
        const newActiveEntry = { ...state.activeEntryBySession };
        delete newActiveEntry[sessionId];
        const newLoadingHistory = { ...state.loadingHistoryBySession };
        delete newLoadingHistory[sessionId];
        return {
          sessions: state.sessions.filter(s => s.id !== sessionId),
          isLoadingSessions: false,
          historyBySession: newHistory,
          partialHistoryBySession: partial,
          activeEntryBySession: newActiveEntry,
          loadingHistoryBySession: newLoadingHistory,
        };
      });
      if (refresh) terminalSessionReadReplacements.set(superseded, useTerminalStore.getState().loadSessions());
    };
    const announceSessionEnded = (data: { sessionId?: string; terminalId?: string }) => {
      const sessionId = data.terminalId || data.sessionId;
      const session = useTerminalStore.getState().sessions.find((item) => item.id === sessionId);
      if (!sessionId || endedSessions.has(sessionId)) return;
      removeExitedSession(data);
      // Eventos repetidos não anunciam duas vezes, inclusive antes da listagem inicial.
      const message = session?.name?.trim()
        ? i18next.t('terminal.announce.exited', { name: session.name })
        : i18next.t('terminal.announce.terminated');
      announceTerminalResult(sessionId, message, message);
    };
    unsubs.push(EventsOn('terminal:session_closed', announceSessionEnded));
    unsubs.push(EventsOn('terminal:exited', announceSessionEnded));

    // Comando iniciado (raw mode — cria entry para receber output)
    unsubs.push(EventsOn('terminal:command_start', (data: { sessionId: string; commandId?: string; command: string; source: string }) => {
      if (endedSessions.has(data.sessionId)) return;
      if (data.source !== 'user-raw') listingStates?.set(data.sessionId, 'running');
      // Limpa pending output do comando anterior
      cancelOutputAnnouncement(data.sessionId);

      const tempEntry: HistoryEntry = terminal.HistoryEntry.createFrom({
        id: resolveTerminalCommandId(data.sessionId, data.commandId),
        command: data.command,
        output: '',
        exitCode: -999, // sentinel para "em execução / raw"
        startedAt: new Date().toISOString(),
        endedAt: '',
        source: data.source,
      });

      if (!tracksHistory(data.sessionId)) {
        if (data.source !== 'user-raw') {
          unobservedCommands.set(data.sessionId, tempEntry);
          set(state => ({ sessions: changeSessionState(state.sessions, data.sessionId, 'running') }));
        }
        return;
      }
      unobservedCommands.delete(data.sessionId);
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        const recent = sessionHistory.length >= historyEntryLimit ? sessionHistory.slice(1 - historyEntryLimit) : sessionHistory;
        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: [...recent, tempEntry],
          },
          partialHistoryBySession: state.historyBySession[data.sessionId] ? state.partialHistoryBySession
            : { ...state.partialHistoryBySession, [data.sessionId]: true },
          activeEntryBySession: {
            ...state.activeEntryBySession,
            [data.sessionId]: tempEntry.id,
          },
          sessions: data.source === 'user-raw'
            ? state.sessions
            : changeSessionState(state.sessions, data.sessionId, 'running'),
        };
      });
    }));

    // Raw output (streaming contínuo do PTY — vai para o último entry da sessão)
    unsubs.push(EventsOn('terminal:raw_output', (data: { sessionId: string; output: string }) => {
      if (!tracksHistory(data.sessionId)) return;
      // Agenda anúncio do output (com debounce para esperar fim do streaming)
      if (terminalOutputListenerRefCount > 0) scheduleOutputAnnounce(data.sessionId, data.output);

      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId];
        if (!sessionHistory || sessionHistory.length === 0) return state;

        const activeEntryId = state.activeEntryBySession[data.sessionId];

        // Appenda output ao entry ativo (ou ao último entry)
        const updatedHistory = sessionHistory.map(entry => {
          if (activeEntryId ? entry.id === activeEntryId : entry === sessionHistory[sessionHistory.length - 1]) {
            return appendHistoryOutput(entry, data.output);
          }
          return entry;
        });

        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: updatedHistory,
          },
        };
      });
    }));

    // Output filtrado (durante RunCommand do LLM com markers)
    unsubs.push(EventsOn('terminal:command_output', (data: { sessionId: string; commandId: string; output: string }) => {
      const pending = unobservedCommands.get(data.sessionId);
      if (pending && pending.id === data.commandId) {
        unobservedCommands.set(data.sessionId, appendHistoryOutput(pending, data.output));
        return;
      }
      if (!tracksHistory(data.sessionId)) return;
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        const updatedHistory = sessionHistory.map(entry => {
          if (entry.exitCode === -999 && entry.source !== 'user-raw') {
            return appendHistoryOutput(entry, data.output);
          }
          return entry;
        });
        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: updatedHistory,
          },
        };
      });
    }));

    // Comando finalizado (apenas para RunCommand com markers — LLM)
    unsubs.push(EventsOn('terminal:command_end', (data: { sessionId: string; commandId: string; output: string; exitCode: number }) => {
      if (endedSessions.has(data.sessionId)) return;
      listingStates?.set(data.sessionId, 'idle');
      const pending = unobservedCommands.get(data.sessionId);
      unobservedCommands.delete(data.sessionId);
      const workspace = useWorkspaceStore.getState().workspace;
      const tabsAtCompletion = workspace?.tabs ?? [];
      const tabIndex = tabsAtCompletion.findIndex(candidate => candidate.type === 'terminal' && candidate.state?.sessionId === data.sessionId);
      const tab = tabsAtCompletion[tabIndex];
      if (terminalOutputListenerRefCount > 0 || tab) {
        if (isVoiceAccessibilityOriginCurrentlyActive(terminalOrigin(data.sessionId))) playReceiveSound();
        const outputPreview = data.output
          ? (data.output.length > 300 ? data.output.slice(0, 300) + i18next.t('terminal.announce.truncatedSuffix') : data.output)
          : i18next.t('terminal.announce.emptyOutput');
        const { sessionId, exitCode } = data;
        const titleAtCompletion = tab?.title?.trim();
        const nameAtCompletion = useTerminalStore.getState().sessions.find(session => session.id === sessionId)?.name?.trim();
        const tabLabelAtCompletion = titleAtCompletion || (tabIndex >= 0
          ? i18next.t('terminal.announce.unnamedTab', { position: tabIndex + 1 }) : i18next.t('terminal.pageTitle'));
        const fallback = i18next.t('terminal.announce.backgroundResult', { name: nameAtCompletion || tabLabelAtCompletion, exitCode });
        const resolve = () => {
          const tabs = useWorkspaceStore.getState().workspace?.tabs ?? [];
          const index = tabs.findIndex(candidate => candidate.type === 'terminal' && candidate.state?.sessionId === sessionId);
          const session = useTerminalStore.getState().sessions.find(candidate => candidate.id === sessionId);
          // O fechamento pode retirar a sessão do catálogo antes de a fila chegar a ela.
          const sessionName = session ? session.name?.trim() : nameAtCompletion;
          const name = sessionName || tabs[index]?.title?.trim() || (index >= 0
            ? i18next.t('terminal.announce.unnamedTab', { position: index + 1 }) : tabLabelAtCompletion);
          return i18next.t('terminal.announce.backgroundResult', { name, exitCode });
        };
        announceTerminalResult(sessionId, i18next.t('terminal.announce.outputWithExit', { output: outputPreview, exitCode }), { resolve, fallback });
      }
      if (!tracksHistory(data.sessionId)) {
        set(state => ({ sessions: changeSessionState(state.sessions, data.sessionId, 'idle') }));
        return;
      }
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] ?? (pending ? [pending] : []);
        const updatedHistory = sessionHistory.map(entry => {
          // Encontra o entry do LLM em execução (não raw)
          if (entry.exitCode === -999 && entry.source !== 'user-raw') {
            return terminal.HistoryEntry.createFrom({
              ...entry,
              id: data.commandId || entry.id,
              output: data.output || entry.output,
              exitCode: data.exitCode,
              endedAt: new Date().toISOString(),
            });
          }
          return entry;
        });
        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: updatedHistory,
          },
          partialHistoryBySession: state.historyBySession[data.sessionId] ? state.partialHistoryBySession
            : { ...state.partialHistoryBySession, [data.sessionId]: true },
          // Marca sessão como idle
          sessions: changeSessionState(state.sessions, data.sessionId, 'idle'),
          activeEntryBySession: {
            ...state.activeEntryBySession,
            [data.sessionId]: null,
          },
        };
      });
    }));

    terminalEventListenerCleanup = () => {
      unobservedCommands.clear();
      unsubs.forEach(fn => fn());
    };

    return () => {
      terminalEventListenerRefCount = Math.max(0, terminalEventListenerRefCount - 1);
      if (!options?.lifecycleOnly) {
        terminalOutputListenerRefCount = Math.max(0, terminalOutputListenerRefCount - 1);
        if (terminalOutputListenerRefCount === 0) pendingAnnouncements.forEach((_, id) => cancelOutputAnnouncement(id));
      }
      if (terminalEventListenerRefCount === 0 && terminalEventListenerCleanup) {
        terminalEventListenerCleanup();
        terminalEventListenerCleanup = null;
      }
    };
  },
}));
