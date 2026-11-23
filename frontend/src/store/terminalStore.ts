import { codePointPrefix } from '../utils/format';
import { hasTerminalEndTime, isManagedCommandRunning, recentTerminalHistory, sessionOutputSource } from '../lib/terminalHistory';
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
interface OutputPreview {
  output: string;
  prefixComplete: boolean;
  truncated: boolean;
}
const pendingAnnouncements = new Map<string, OutputPreview & { timer: ReturnType<typeof setTimeout> }>();
let terminalEventListenerRefCount = 0;
let terminalOutputListenerRefCount = 0;
let terminalSessionReadGeneration = 0;
// Só a consulta mais recente pode publicar. Seus eventos podem preceder o primeiro catálogo.
let listingStates: Map<string, string> | undefined;
const terminalSessionReadReplacements = new Map<number, Promise<boolean>>();
let terminalEventListenerCleanup: (() => void) | null = null;
function terminalOrigin(sessionId: string) {
  return { surfaceType: 'terminal' as const, terminalSessionId: sessionId };
}

function announceTerminalResult(sessionId: string, message: string, inactiveMessage: NonNullable<VoiceAnnounceRequest['inactiveMessage']>) {
  announceWithOrigin({ origin: terminalOrigin(sessionId), message, inactiveMessage, eventType: 'completion',
    waitsForReading: true, protectsReading: true });
}

function unnamedTerminalTab(index: number): string {
  return index >= 0 ? i18next.t('terminal.announce.unnamedTab', { position: index + 1 }) : i18next.t('terminal.pageTitle');
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
  const preview = formatAnnouncementPreview(pending.output, pending.truncated);
  if (!preview) return;
  if (announceWithOrigin({ origin: terminalOrigin(sessionId), message: i18next.t('terminal.announce.output', { output: preview }),
    eventType: 'progress', waitsForReading: true, protectsReading: true })) playReceiveSound();
}

const announcementCharacterLimit = 300;

function formatAnnouncementPreview(prefix: string, truncated: boolean): string {
  const text = prefix.trimEnd();
  return truncated ? text + i18next.t('terminal.announce.truncatedSuffix') : text;
}

function hasOmittedText(content: string, consumed: number): boolean {
  return content.length > consumed && /\S/.test(content.slice(consumed));
}

function appendOutputPreview(pending: OutputPreview | undefined, chunk: string): OutputPreview {
  if (pending?.prefixComplete) {
    return pending.truncated || !hasOmittedText(chunk, 0) ? pending : { ...pending, truncated: true };
  }
  const previous = pending?.output ?? '';
  const content = previous ? chunk : chunk.trimStart();
  // Dois elementos UTF-16 por ponto de código bastam para montar toda a prévia.
  const candidate = previous + content.slice(0, 2 * announcementCharacterLimit - previous.length);
  const output = codePointPrefix(candidate, announcementCharacterLimit);
  const consumed = output.length - previous.length;
  return { output, prefixComplete: content.length > consumed, truncated: hasOmittedText(content, consumed) };
}

function scheduleOutputAnnounce(sessionId: string, chunk: string) {
  if (!isVoiceAccessibilityOriginCurrentlyActive(terminalOrigin(sessionId))) return;
  const preview = appendOutputPreview(pendingAnnouncements.get(sessionId), chunk);
  cancelOutputAnnouncement(sessionId);
  const timer = setTimeout(() => flushOutputAnnouncement(sessionId), 500);
  pendingAnnouncements.set(sessionId, { ...preview, timer });
}

// Re-export types for components
export type SessionInfo = terminal.SessionInfo;
export type HistoryEntry = terminal.HistoryEntry;

// Até 200 comandos, como no backend, e 200 blocos da sessão. Cada entrada guarda até 50 KiB.
// O marcador de corte é adicional: ‘…’ no stream e o aviso original no resultado final.
// Antes da primeira abertura, só o comando gerenciado em curso precisa acompanhar o stream.
// Hidratar, concluir ou encerrar a sessão remove sua entrada. O último listener limpa as restantes.
const unobservedCommands = new Map<string, HistoryEntry>();
// O evento pode chegar antes de a primeira aba buscar a entrada manual no backend.
const manualTargetsAwaitingHistory = new Map<string, string>();

function manualEntryIdentity(entry: HistoryEntry): HistoryEntry {
  return entry.output ? terminal.HistoryEntry.createFrom({ ...entry, output: '' }) : entry;
}
const historyEntryLimit = 200;
let sessionOutputSequence = 0;
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

interface TerminalCommandEnd {
  sessionId: string;
  commandId: string;
  output: string;
  exitCode: number;
  activeManualCommandId?: string;
  announcementPreview?: Pick<OutputPreview, 'output' | 'truncated'>;
}

function completeHistoryCommand(history: HistoryEntry[], event: TerminalCommandEnd): HistoryEntry[] {
  const index = history.findIndex(entry => entry.id === event.commandId && isManagedCommandRunning(entry));
  if (index < 0) return history;
  const entry = history[index];
  const result = history.slice();
  result[index] = terminal.HistoryEntry.createFrom({
    ...entry, output: event.output || entry.output, exitCode: event.exitCode, endedAt: new Date().toISOString(),
  });
  return result;
}

function appendSessionOutput(history: HistoryEntry[], output: string): HistoryEntry[] {
  const entries = history.slice();
  const last = entries[entries.length - 1];
  let block = last?.source === sessionOutputSource ? last : undefined;
  let used = block ? historyBytes.get(block) ?? historyEncoder.encode(block.output).length : 0;
  while (output) {
    const { read, written } = historyEncoder.encodeInto(output, historyBuffer.subarray(used));
    if (read > 0) {
      const text = output.slice(0, read);
      const updated = terminal.HistoryEntry.createFrom(block
        ? { ...block, output: block.output + text }
        : { id: `session-output-${++sessionOutputSequence}`, source: sessionOutputSource, command: '', output: text });
      historyBytes.set(updated, used + written);
      if (block) entries[entries.length - 1] = updated;
      else entries.push(updated);
      output = output.slice(read);
    }
    // Quando o próximo ponto de código não cabe, ele inicia um bloco inteiro.
    block = undefined;
    used = 0;
  }
  return entries.length === history.length ? entries : recentTerminalHistory(entries, historyEntryLimit);
}

interface TerminalState {
  sessions: SessionInfo[];
  historyBySession: Record<string, HistoryEntry[]>;
  partialHistoryBySession: Record<string, boolean>;
  /** Comando ativo usado para validar a operação de interrupção da sessão. */
  activeEntryBySession: Record<string, string | null>;
  /** Identidade manual confirmada por command_end. O ID ativo autoriza seu uso após a expulsão do histórico. */
  manualEntryBySession: Record<string, HistoryEntry | undefined>;
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
  manualEntryBySession: {},
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
        // Ancore as entradas locais no próximo comando comum. O snapshot do backend
        // pode preservar uma entrada manual ativa e omitir outro comando da janela local.
        const beforeCommand = new Map<string, HistoryEntry[]>();
        let localEntries: HistoryEntry[] = [];
        for (const entry of state.historyBySession[sessionId] ?? []) {
          const stored = entries.get(entry.id);
          if (!stored) {
            localEntries.push(entry);
            continue;
          }
          if (localEntries.length) {
            beforeCommand.set(entry.id, localEntries);
            localEntries = [];
          }
          if (!stored.output && entry.source === 'user-raw' && entry.output) {
            // A sessão registra o comando manual. Sua saída chega à interface pelo stream.
            const hydrated = terminal.HistoryEntry.createFrom({ ...stored, output: entry.output });
            const used = historyBytes.get(entry);
            if (used !== undefined) historyBytes.set(hydrated, used);
            entries.set(entry.id, hydrated);
          } else if (!hasTerminalEndTime(stored) && hasTerminalEndTime(entry)) {
            entries.set(entry.id, entry);
          }
        }
        const ordered = Array.from(entries.values()).flatMap(entry => [...(beforeCommand.get(entry.id) ?? []), entry]);
        const mergedHistory = recentTerminalHistory(ordered.concat(localEntries), historyEntryLimit);
        const manualId = manualTargetsAwaitingHistory.get(sessionId);
        const candidate = manualId ? entries.get(manualId) ?? mergedHistory.find(entry => entry.id === manualId) : undefined;
        const restored = candidate?.source === 'user-raw' ? candidate : undefined;
        if (restored) manualTargetsAwaitingHistory.delete(sessionId);
        return {
          historyBySession: {
            ...state.historyBySession,
            [sessionId]: mergedHistory,
          },
          ...(restored && {
            activeEntryBySession: { ...state.activeEntryBySession, [sessionId]: restored.id },
            manualEntryBySession: { ...state.manualEntryBySession, [sessionId]: manualEntryIdentity(restored) },
          }),
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
    // O rodapé pode vir vazio. Preserve a prévia do stream antes do limite do histórico.
    const latestCommands = new Map<string, { id: string; preview?: OutputPreview; ended?: boolean }>();
    // Uma sessão já exibida conserva seu histórico entre montagens da página.
    // Continue aplicando eventos ao cache mesmo quando os anúncios estão suspensos.
    // Uma página de terminal montada também inicia o cache de outras sessões com saída.
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
          historyBySession: terminalOutputListenerRefCount > 0 && !unobservedCommands.has(data.id)
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
      manualTargetsAwaitingHistory.delete(sessionId);
      endedSessions.add(sessionId);
      if (endedSessions.size > 1024) endedSessions.delete(endedSessions.values().next().value!);
      latestCommands.delete(sessionId);
      flushOutputAnnouncement(sessionId);
      set(state => {
        const newHistory = { ...state.historyBySession };
        delete newHistory[sessionId];
        const partial = { ...state.partialHistoryBySession };
        delete partial[sessionId];
        const newActiveEntry = { ...state.activeEntryBySession };
        delete newActiveEntry[sessionId];
        const manualEntries = { ...state.manualEntryBySession };
        delete manualEntries[sessionId];
        const newLoadingHistory = { ...state.loadingHistoryBySession };
        delete newLoadingHistory[sessionId];
        return {
          sessions: state.sessions.filter(s => s.id !== sessionId),
          historyBySession: newHistory,
          partialHistoryBySession: partial,
          activeEntryBySession: newActiveEntry,
          manualEntryBySession: manualEntries,
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
    unsubs.push(EventsOn('terminal:command_start', (data: { sessionId: string; commandId: string; command: string; source: string }) => {
      if (endedSessions.has(data.sessionId)) return;
      manualTargetsAwaitingHistory.delete(data.sessionId);
      if (data.source !== 'user-raw') listingStates?.set(data.sessionId, 'running');
      latestCommands.set(data.sessionId, { id: data.commandId });
      // Limpa pending output do comando anterior
      cancelOutputAnnouncement(data.sessionId);

      if (terminalOutputListenerRefCount > 0) {
        announceWithOrigin({
          origin: terminalOrigin(data.sessionId),
          eventType: data.source === 'user-raw' ? 'user-action' : 'progress',
          message: `${i18next.t('terminal.history.commandPrefix')} ${data.command}`,
        });
      }

      const tempEntry: HistoryEntry = terminal.HistoryEntry.createFrom({
        // A sessão Go gera o UUID antes de emitir o início e o mantém até a conclusão.
        id: data.commandId,
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
        }
        set(state => ({
          ...(data.source !== 'user-raw' && { sessions: changeSessionState(state.sessions, data.sessionId, 'running') }),
          activeEntryBySession: { ...state.activeEntryBySession, [data.sessionId]: null },
        }));
        return;
      }
      unobservedCommands.delete(data.sessionId);
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: recentTerminalHistory([...sessionHistory, tempEntry], historyEntryLimit),
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

    // A saída manual pode chegar depois do início de outro comando na mesma sessão.
    unsubs.push(EventsOn('terminal:raw_output', (data: { sessionId: string; commandId: string; output: string }) => {
      if (!tracksHistory(data.sessionId)) return;
      // Agenda anúncio do output (com debounce para esperar fim do streaming)
      if (terminalOutputListenerRefCount > 0) scheduleOutputAnnounce(data.sessionId, data.output);
      if (!data.output) return;

      const pending = unobservedCommands.get(data.sessionId);
      if (pending) unobservedCommands.delete(data.sessionId);
      set(state => {
        // O primeiro bloco observado pode chegar entre a montagem da aba e loadHistory.
        const sessionHistory = state.historyBySession[data.sessionId] ?? (pending ? [pending] : []);
        const index = data.commandId ? sessionHistory.findIndex(entry => entry.id === data.commandId && entry.source === 'user-raw') : -1;
        let updatedHistory: HistoryEntry[];
        if (index < 0) {
          updatedHistory = appendSessionOutput(sessionHistory, data.output);
        } else {
          const entry = sessionHistory[index];
          const updated = appendHistoryOutput(entry, data.output);
          if (entry === updated) return state;
          updatedHistory = sessionHistory.slice();
          updatedHistory[index] = updated;
        }

        return {
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: updatedHistory,
          },
          partialHistoryBySession: state.historyBySession[data.sessionId] ? state.partialHistoryBySession
            : { ...state.partialHistoryBySession, [data.sessionId]: true },
          ...(pending && { activeEntryBySession: { ...state.activeEntryBySession, [data.sessionId]: pending.id } }),
        };
      });
    }));

    // Output filtrado (durante RunCommand do LLM com markers)
    unsubs.push(EventsOn('terminal:command_output', (data: { sessionId: string; commandId: string; output: string }) => {
      const tracked = latestCommands.get(data.sessionId);
      if (tracked?.id === data.commandId && !tracked.ended) tracked.preview = appendOutputPreview(tracked.preview, data.output);
      const pending = unobservedCommands.get(data.sessionId);
      if (pending && pending.id === data.commandId) {
        unobservedCommands.set(data.sessionId, appendHistoryOutput(pending, data.output));
        return;
      }
      if (!tracksHistory(data.sessionId)) return;
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] || [];
        const updatedHistory = sessionHistory.map(entry => {
          if (isManagedCommandRunning(entry) && entry.id === data.commandId) {
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
    unsubs.push(EventsOn('terminal:command_end', (data: TerminalCommandEnd) => {
      if (endedSessions.has(data.sessionId)) return;
      const tracked = latestCommands.get(data.sessionId);
      if (tracked && (tracked.id !== data.commandId || tracked.ended)) {
        // Uma conclusão atrasada ainda pertence ao histórico do seu comando.
        set(state => {
          const history = state.historyBySession[data.sessionId];
          if (!history) return state;
          const updated = completeHistoryCommand(history, data);
          return updated === history ? state : { historyBySession: { ...state.historyBySession, [data.sessionId]: updated } };
        });
        return;
      }
      listingStates?.set(data.sessionId, 'idle');
      const manualId = data.exitCode === -1 ? data.activeManualCommandId : undefined;
      if (manualId) manualTargetsAwaitingHistory.set(data.sessionId, manualId);
      else manualTargetsAwaitingHistory.delete(data.sessionId);
      const streamPreview = tracked?.preview;
      // Conserve a identidade até o próximo início para rejeitar términos duplicados ou atrasados.
      latestCommands.set(data.sessionId, { id: data.commandId, ended: true });
      const pending = unobservedCommands.get(data.sessionId);
      unobservedCommands.delete(data.sessionId);
      const workspace = useWorkspaceStore.getState().workspace;
      const tab = workspace?.tabs.find(candidate => candidate.type === 'terminal' && candidate.state?.sessionId === data.sessionId);
      if (terminalOutputListenerRefCount > 0 || tab) {
        if (isVoiceAccessibilityOriginCurrentlyActive(terminalOrigin(data.sessionId))) playReceiveSound();
        const output = data.output || (pending?.id === data.commandId && pending.output) || useTerminalStore.getState().historyBySession[data.sessionId]
          ?.find(entry => entry.id === data.commandId)?.output;
        const preview = data.announcementPreview ?? (data.output ? undefined : streamPreview) ??
          appendOutputPreview(undefined, output || '');
        const outputPreview = formatAnnouncementPreview(preview.output, preview.truncated) ||
          i18next.t('terminal.announce.emptyOutput');
        const { sessionId, exitCode } = data;
        const titleAtCompletion = tab?.title?.trim();
        const nameAtCompletion = useTerminalStore.getState().sessions.find(session => session.id === sessionId)?.name?.trim();
        const fallback = i18next.t('terminal.announce.backgroundResult', {
          name: nameAtCompletion || titleAtCompletion || i18next.t('terminal.pageTitle'), exitCode,
        });
        const resolve = () => {
          const tabs = useWorkspaceStore.getState().workspace?.tabs ?? [];
          const index = tabs.findIndex(candidate => candidate.type === 'terminal' && candidate.state?.sessionId === sessionId);
          const session = useTerminalStore.getState().sessions.find(candidate => candidate.id === sessionId);
          // O fechamento pode retirar a sessão do catálogo antes de a fila chegar a ela.
          const sessionName = session ? session.name?.trim() : nameAtCompletion;
          const name = sessionName || tabs[index]?.title?.trim() || (index < 0 && titleAtCompletion) || unnamedTerminalTab(index);
          return i18next.t('terminal.announce.backgroundResult', { name, exitCode });
        };
        announceTerminalResult(sessionId, i18next.t('terminal.announce.outputWithExit', { output: outputPreview, exitCode }), { resolve, fallback });
      }
      set(state => {
        const sessionHistory = state.historyBySession[data.sessionId] ?? (pending ? [pending] : []);
        const retained = state.manualEntryBySession[data.sessionId];
        const restored = manualId && (retained?.id === manualId ? retained :
          sessionHistory.find(entry => entry.id === manualId && entry.source === 'user-raw'));
        if (restored) manualTargetsAwaitingHistory.delete(data.sessionId);
        const nextTarget = {
          sessions: changeSessionState(state.sessions, data.sessionId, 'idle'),
          activeEntryBySession: { ...state.activeEntryBySession, [data.sessionId]: restored ? restored.id : null },
          manualEntryBySession: { ...state.manualEntryBySession, [data.sessionId]: restored ? manualEntryIdentity(restored) : undefined },
        };
        if (!tracksHistory(data.sessionId)) return nextTarget;
        const updatedHistory = completeHistoryCommand(sessionHistory, data);
        return {
          ...nextTarget,
          historyBySession: {
            ...state.historyBySession,
            [data.sessionId]: updatedHistory,
          },
          partialHistoryBySession: state.historyBySession[data.sessionId] ? state.partialHistoryBySession
            : { ...state.partialHistoryBySession, [data.sessionId]: true },
        };
      });
    }));

    terminalEventListenerCleanup = () => {
      unobservedCommands.clear();
      manualTargetsAwaitingHistory.clear();
      set({ manualEntryBySession: {} });
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
