import { logger } from '../utils/logger';
import { create } from 'zustand';
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
let terminalEventListenerRefCount = 0;
let terminalEventListenerCleanup: (() => void) | null = null;

function scheduleOutputAnnounce(chunk: string) {
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
  loadSessions: () => Promise<void>;
  createSession: (name?: string) => Promise<string | null>;
  closeSession: (id: string) => Promise<void>;
  sendInput: (sessionId: string, input: string) => Promise<void>;
  interrupt: (sessionId: string) => Promise<void>;
  loadHistory: (sessionId: string) => Promise<void>;
  setupEventListeners: () => () => void;
}

export const useTerminalStore = create<TerminalState>((set) => ({
  sessions: [],
  historyBySession: {},
  activeEntryBySession: {},
  isLoadingSessions: false,
  loadingHistoryBySession: {},

  loadSessions: async () => {
    set({ isLoadingSessions: true });
    try {
      const sessions = await ListTerminalSessions();
      set({ sessions: sessions || [] });
    } catch (err) {
      logger.error('[Terminal] Erro ao carregar sessões:', err);
    } finally {
      set({ isLoadingSessions: false });
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
    } catch (err) {
      logger.error('[Terminal] Erro ao fechar sessão:', err);
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

  setupEventListeners: () => {
    terminalEventListenerRefCount += 1;
    if (terminalEventListenerCleanup) {
      return () => {
        terminalEventListenerRefCount = Math.max(0, terminalEventListenerRefCount - 1);
        if (terminalEventListenerRefCount === 0 && terminalEventListenerCleanup) {
          terminalEventListenerCleanup();
          terminalEventListenerCleanup = null;
        }
      };
    }

    const unsubs: Array<() => void> = [];

    // Sessão criada
    unsubs.push(EventsOn('terminal:session_created', (data: SessionInfo) => {
      set(state => ({
        sessions: [...state.sessions, data],
        historyBySession: {
          ...state.historyBySession,
          [data.id]: [],
        },
      }));
    }));

    // Sessão fechada
    unsubs.push(EventsOn('terminal:session_closed', (data: { sessionId: string }) => {
      set(state => {
        const newHistory = { ...state.historyBySession };
        delete newHistory[data.sessionId];
        const newActiveEntry = { ...state.activeEntryBySession };
        delete newActiveEntry[data.sessionId];
        const newLoadingHistory = { ...state.loadingHistoryBySession };
        delete newLoadingHistory[data.sessionId];
        return {
          sessions: state.sessions.filter(s => s.id !== data.sessionId),
          historyBySession: newHistory,
          activeEntryBySession: newActiveEntry,
          loadingHistoryBySession: newLoadingHistory,
        };
      });
    }));

    // Comando iniciado (raw mode — cria entry para receber output)
    unsubs.push(EventsOn('terminal:command_start', (data: { sessionId: string; command: string; source: string }) => {
      // Limpa pending output do comando anterior
      pendingOutput = '';
      if (announceTimer) { clearTimeout(announceTimer); announceTimer = null; }

      const tempEntry: HistoryEntry = terminal.HistoryEntry.createFrom({
        id: `raw-${Date.now()}`,
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
        };
      });
    }));

    // Raw output (streaming contínuo do PTY — vai para o último entry da sessão)
    unsubs.push(EventsOn('terminal:raw_output', (data: { sessionId: string; output: string }) => {
      // Agenda anúncio do output (com debounce para esperar fim do streaming)
      scheduleOutputAnnounce(data.output);

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
        };
      });
    }));

    terminalEventListenerCleanup = () => {
      unsubs.forEach(fn => fn());
    };

    return () => {
      terminalEventListenerRefCount = Math.max(0, terminalEventListenerRefCount - 1);
      if (terminalEventListenerRefCount === 0 && terminalEventListenerCleanup) {
        terminalEventListenerCleanup();
        terminalEventListenerCleanup = null;
      }
    };
  },
}));
