import { logger } from '../utils/logger';
import { create } from 'zustand';
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
import { announce } from '../hooks/useAnnouncer';
import i18next from 'i18next';

// Debounce para anúncio de output (acumula chunks e espera streaming parar)
let announceTimer: ReturnType<typeof setTimeout> | null = null;
let pendingOutput = '';
let pendingOutputSessionId = '';
let terminalEventListenerRefCount = 0;
let terminalOutputListenerRefCount = 0;
let terminalSessionReadGeneration = 0;
const terminalSessionReadReplacements = new Map<number, Promise<boolean>>();
let terminalEventListenerCleanup: (() => void) | null = null;
let legacyCommandSequence = 0;

export function resolveTerminalCommandId(sessionId: string, commandId?: string): string {
  return commandId || `legacy-${sessionId}-${Date.now()}-${legacyCommandSequence++}`;
}

function scheduleOutputAnnounce(sessionId: string, chunk: string) {
  if (pendingOutputSessionId !== sessionId) pendingOutput = '';
  pendingOutputSessionId = sessionId;
  pendingOutput += chunk;
  if (announceTimer) clearTimeout(announceTimer);
  announceTimer = setTimeout(() => {
    const text = pendingOutput.trim();
    if (text) {
      playReceiveSound();
      const truncated = text.length > 300
        ? text.slice(0, 300) + i18next.t('terminal.announce.truncatedSuffix')
        : text;
      announce(i18next.t('terminal.announce.output', { output: truncated }));
    }
    pendingOutput = '';
    pendingOutputSessionId = '';
    announceTimer = null;
  }, 500); // Espera 500ms sem novos chunks antes de anunciar
}

// Re-export types for components
export type SessionInfo = terminal.SessionInfo;
export type HistoryEntry = terminal.HistoryEntry;

interface TerminalState {
  sessions: SessionInfo[];
  historyBySession: Record<string, HistoryEntry[]>;
  /** ID do último entry que está recebendo raw output (modo interativo) */
  activeEntryBySession: Record<string, string | null>;
  isLoadingSessions: boolean;
  loadingHistoryBySession: Record<string, boolean>;

  // Actions
  loadSessions: () => Promise<boolean>;
  createSession: (name?: string) => Promise<string | null>;
  closeSession: (id: string) => Promise<boolean>;
  sendInput: (sessionId: string, input: string) => Promise<void>;
  interrupt: (sessionId: string) => Promise<void>;
  loadHistory: (sessionId: string) => Promise<void>;
  setupEventListeners: (options?: { lifecycleOnly?: boolean }) => () => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  historyBySession: {},
  activeEntryBySession: {},
  isLoadingSessions: false,
  loadingHistoryBySession: {},

  loadSessions: async () => {
    const request = ++terminalSessionReadGeneration;
    const owner = useAuthStore.getState();
    let currentOwner = true;
    const unsubscribe = useAuthStore.subscribe(state => {
      if (state.isAuthenticated !== owner.isAuthenticated || state.user?.userId !== owner.user?.userId ||
          state.user?.sessionId !== owner.user?.sessionId) currentOwner = false;
    });
    set({ isLoadingSessions: true });
    try {
      const sessions = await ListTerminalSessions();
      if (!currentOwner) return false;
      if (request !== terminalSessionReadGeneration) {
        const replacement = terminalSessionReadReplacements.get(request);
        if (!replacement) return false;
        const refreshed = await replacement;
        return currentOwner && refreshed;
      }
      set({ sessions: sessions || [] });
      return true;
    } catch (err) {
      const replacement = terminalSessionReadReplacements.get(request);
      if (currentOwner && replacement) {
        const refreshed = await replacement;
        return currentOwner && refreshed;
      }
      logger.error('[Terminal] Erro ao carregar sessões:', err);
      return false;
    } finally {
      unsubscribe();
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
    if (!sessionId) return;

    try {
      playSendSound();
      await SendTerminalInput(sessionId, input);
    } catch (err) {
      logger.error('[Terminal] Erro ao enviar input:', err);
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
    set(state => ({
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

        return {
          historyBySession: {
            ...state.historyBySession,
            [sessionId]: history || [],
          },
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
          if (terminalOutputListenerRefCount === 0 && announceTimer) {
            clearTimeout(announceTimer);
            announceTimer = null;
            pendingOutput = '';
            pendingOutputSessionId = '';
          }
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
          sessions: [...state.sessions, data],
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
      endedSessions.add(sessionId);
      if (endedSessions.size > 1024) endedSessions.delete(endedSessions.values().next().value!);
      if (pendingOutputSessionId === sessionId) {
        if (announceTimer) clearTimeout(announceTimer);
        announceTimer = null;
        pendingOutput = '';
        pendingOutputSessionId = '';
      }
      set(state => {
        const newHistory = { ...state.historyBySession };
        delete newHistory[sessionId];
        const newActiveEntry = { ...state.activeEntryBySession };
        delete newActiveEntry[sessionId];
        const newLoadingHistory = { ...state.loadingHistoryBySession };
        delete newLoadingHistory[sessionId];
        return {
          sessions: state.sessions.filter(s => s.id !== sessionId),
          isLoadingSessions: false,
          historyBySession: newHistory,
          activeEntryBySession: newActiveEntry,
          loadingHistoryBySession: newLoadingHistory,
        };
      });
      if (refresh) terminalSessionReadReplacements.set(superseded, useTerminalStore.getState().loadSessions());
    };
    const announceSessionEnded = (data: { sessionId?: string; terminalId?: string }) => {
      const sessionId = data.terminalId || data.sessionId;
      const session = useTerminalStore.getState().sessions.find((item) => item.id === sessionId);
      const firstEvent = Boolean(sessionId && !endedSessions.has(sessionId));
      removeExitedSession(data);
      // Eventos repetidos não anunciam duas vezes, inclusive antes da listagem inicial.
      if (firstEvent) {
        announce(session?.name?.trim()
          ? i18next.t('terminal.announce.exited', { name: session.name })
          : i18next.t('terminal.announce.terminated'));
      }
    };
    unsubs.push(EventsOn('terminal:session_closed', announceSessionEnded));
    unsubs.push(EventsOn('terminal:exited', announceSessionEnded));

    // Comando iniciado (raw mode — cria entry para receber output)
    unsubs.push(EventsOn('terminal:command_start', (data: { sessionId: string; commandId?: string; command: string; source: string }) => {
      if (terminalOutputListenerRefCount === 0 || endedSessions.has(data.sessionId)) return;
      // Limpa pending output do comando anterior
      pendingOutput = '';
      pendingOutputSessionId = '';
      if (announceTimer) { clearTimeout(announceTimer); announceTimer = null; }

      const tempEntry: HistoryEntry = terminal.HistoryEntry.createFrom({
        id: resolveTerminalCommandId(data.sessionId, data.commandId),
        command: data.command,
        output: '',
        exitCode: -999, // sentinel para "em execução / raw"
        startedAt: new Date().toISOString(),
        endedAt: '',
        source: data.source,
      });

      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: [...sessionHistory, tempEntry],
          },
          activeEntryBySession: {
            ...state.activeEntryBySession,
            [data.sessionId]: tempEntry.id,
          },
          sessions: data.source === 'user-raw'
            ? state.sessions
            : state.sessions.map(session => (
              session.id === data.sessionId
                ? terminal.SessionInfo.createFrom({ ...session, state: 'running' })
                : session
            )),
        };
      });
    }));

    // Raw output (streaming contínuo do PTY — vai para o último entry da sessão)
    unsubs.push(EventsOn('terminal:raw_output', (data: { sessionId: string; output: string }) => {
      if (terminalOutputListenerRefCount === 0 || endedSessions.has(data.sessionId)) return;
      // Agenda anúncio do output (com debounce para esperar fim do streaming)
      scheduleOutputAnnounce(data.sessionId, data.output);

      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId];
        if (!sessionHistory || sessionHistory.length === 0) return state;

        const activeEntryId = state.activeEntryBySession[data.sessionId];

        // Appenda output ao entry ativo (ou ao último entry)
        const updatedHistory = sessionHistory.map(entry => {
          if (activeEntryId ? entry.id === activeEntryId : entry === sessionHistory[sessionHistory.length - 1]) {
            return terminal.HistoryEntry.createFrom({
              ...entry,
              output: entry.output + data.output,
            });
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
      if (terminalOutputListenerRefCount === 0 || endedSessions.has(data.sessionId)) return;
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        const updatedHistory = sessionHistory.map(entry => {
          if (entry.exitCode === -999 && entry.source !== 'user-raw') {
            return terminal.HistoryEntry.createFrom({
              ...entry,
              output: entry.output + data.output,
            });
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
      if (terminalOutputListenerRefCount === 0 || endedSessions.has(data.sessionId)) return;
      playReceiveSound();
      const outputPreview = data.output
        ? (data.output.length > 300 ? data.output.slice(0, 300) + i18next.t('terminal.announce.truncatedSuffix') : data.output)
        : i18next.t('terminal.announce.emptyOutput');
      announce(i18next.t('terminal.announce.outputWithExit', { output: outputPreview, exitCode: data.exitCode }));
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
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
          // Marca sessão como idle
          sessions: state.sessions.map(s =>
            s.id === data.sessionId ? terminal.SessionInfo.createFrom({ ...s, state: 'idle' }) : s
          ),
          activeEntryBySession: {
            ...state.activeEntryBySession,
            [data.sessionId]: null,
          },
        };
      });
    }));

    terminalEventListenerCleanup = () => {
      unsubs.forEach(fn => fn());
    };

    return () => {
      terminalEventListenerRefCount = Math.max(0, terminalEventListenerRefCount - 1);
      if (!options?.lifecycleOnly) {
        terminalOutputListenerRefCount = Math.max(0, terminalOutputListenerRefCount - 1);
        if (terminalOutputListenerRefCount === 0 && announceTimer) {
          clearTimeout(announceTimer);
          announceTimer = null;
          pendingOutput = '';
          pendingOutputSessionId = '';
        }
      }
      if (terminalEventListenerRefCount === 0 && terminalEventListenerCleanup) {
        terminalEventListenerCleanup();
        terminalEventListenerCleanup = null;
      }
    };
  },
}));
