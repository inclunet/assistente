import { create } from 'zustand';
import {
  ListTerminalSessions,
  CreateTerminalSession,
  CloseTerminalSession,
  SendTerminalInput,
  InterruptTerminalCommand,
  GetTerminalHistory,
} from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';
import { terminal } from '../../wailsjs/go/models';
import { playSendSound, playReceiveSound } from '../services/audioFeedback';
import { announce } from '../hooks/useAnnouncer';

// Debounce para anúncio de output (acumula chunks e espera streaming parar)
let announceTimer: ReturnType<typeof setTimeout> | null = null;
let pendingOutput = '';

function scheduleOutputAnnounce(chunk: string) {
  pendingOutput += chunk;
  if (announceTimer) clearTimeout(announceTimer);
  announceTimer = setTimeout(() => {
    const text = pendingOutput.trim();
    if (text) {
      playReceiveSound();
      const truncated = text.length > 300
        ? text.slice(0, 300) + '… truncado'
        : text;
      announce(`Saída: ${truncated}`);
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
  activeSessionId: string | null;
  historyBySession: Record<string, HistoryEntry[]>;
  /** ID do último entry que está recebendo raw output (modo interativo) */
  activeEntryBySession: Record<string, string | null>;
  isLoading: boolean;

  // Actions
  loadSessions: () => Promise<void>;
  createSession: (name?: string) => Promise<void>;
  closeSession: (id: string) => Promise<void>;
  setActiveSession: (id: string) => void;
  sendInput: (input: string) => Promise<void>;
  interrupt: () => Promise<void>;
  loadHistory: (sessionId: string) => Promise<void>;
  setupEventListeners: () => () => void;
}

export const useTerminalStore = create<TerminalState>((set, get) => ({
  sessions: [],
  activeSessionId: null,
  historyBySession: {},
  activeEntryBySession: {},
  isLoading: false,

  loadSessions: async () => {
    set({ isLoading: true });
    try {
      const sessions = await ListTerminalSessions();
      set({ sessions: sessions || [] });

      // Se tem sessões mas nenhuma ativa, seleciona a primeira
      const state = get();
      if ((sessions || []).length > 0 && !state.activeSessionId) {
        set({ activeSessionId: sessions[0].id });
      }
    } catch (err) {
      console.error('[Terminal] Erro ao carregar sessões:', err);
    } finally {
      set({ isLoading: false });
    }
  },

  createSession: async (name?: string) => {
    try {
      const info = await CreateTerminalSession(name || '');
      if (info) {
        set({ activeSessionId: info.id });
      }
    } catch (err) {
      console.error('[Terminal] Erro ao criar sessão:', err);
    }
  },

  closeSession: async (id: string) => {
    try {
      await CloseTerminalSession(id);

      const state = get();
      if (state.activeSessionId === id) {
        const remaining = state.sessions.filter(s => s.id !== id);
        set({ activeSessionId: remaining.length > 0 ? remaining[0].id : null });
      }
    } catch (err) {
      console.error('[Terminal] Erro ao fechar sessão:', err);
    }
  },

  setActiveSession: (id: string) => {
    set({ activeSessionId: id });

    const state = get();
    if (!state.historyBySession[id]) {
      get().loadHistory(id);
    }
  },

  sendInput: async (input: string) => {
    const state = get();
    if (!state.activeSessionId) return;

    try {
      playSendSound();
      await SendTerminalInput(state.activeSessionId, input);
    } catch (err) {
      console.error('[Terminal] Erro ao enviar input:', err);
    }
  },

  interrupt: async () => {
    const state = get();
    if (!state.activeSessionId) return;

    try {
      await InterruptTerminalCommand(state.activeSessionId);
    } catch (err) {
      console.error('[Terminal] Erro ao interromper:', err);
    }
  },

  loadHistory: async (sessionId: string) => {
    try {
      const history = await GetTerminalHistory(sessionId);
      set(state => ({
        historyBySession: {
          ...state.historyBySession,
          [sessionId]: history || [],
        },
      }));
    } catch (err) {
      console.error('[Terminal] Erro ao carregar histórico:', err);
    }
  },

  setupEventListeners: () => {
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
        return {
          sessions: state.sessions.filter(s => s.id !== data.sessionId),
          historyBySession: newHistory,
          activeEntryBySession: newActiveEntry,
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
        ? (data.output.length > 300 ? data.output.slice(0, 300) + '… truncado' : data.output)
        : 'vazia';
      announce(`Saída: ${outputPreview}. Código de saída: ${data.exitCode}`);
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

    return () => {
      unsubs.forEach(fn => fn());
    };
  },
}));
