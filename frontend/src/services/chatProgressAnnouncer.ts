import type { ToolOrigin } from '../types/chat';
import type { ChatSurfaceOrigin } from './chatSessionRegistry';

export const CHAT_PROGRESS_BURST_WINDOW_MS = 250;

export interface ChatProgressTool {
  callId: string;
  name: string;
  args?: string;
  origin?: ToolOrigin;
  serverLabel?: string;
  surfaceOrigin?: ChatSurfaceOrigin;
}

interface PendingTool extends ChatProgressTool {
  state: 'pending' | 'running' | 'completed' | 'done';
}

interface ChatProgressAnnouncerOptions {
  announce: (groups: ChatProgressAnnouncementGroup[]) => void;
  burstWindowMs?: number;
}

export interface ChatProgressAnnouncementGroup {
  state: 'running' | 'done';
  tools: ChatProgressTool[];
}

export interface ChatProgressAnnouncer {
  toolStarted: (tool: ChatProgressTool) => void;
  toolEnded: (callId: string, status?: string) => void;
  toolFailed: (callId: string, willRetry?: boolean) => void;
  finishSegment: () => void;
  dispose: () => void;
}

/**
 * Agrupa o começo de ferramentas do mesmo passo sem deixar uma ferramenta
 * longa silenciosa. Cada controller possui uma instância própria: nenhum
 * anúncio de uma conversa pode consumir o estado de outra.
 */
export function createChatProgressAnnouncer({
  announce,
  burstWindowMs = CHAT_PROGRESS_BURST_WINDOW_MS,
}: ChatProgressAnnouncerOptions): ChatProgressAnnouncer {
  const pending = new Map<string, PendingTool>();
  let timer: ReturnType<typeof setTimeout> | null = null;
  let disposed = false;

  const clearTimer = () => {
    if (timer !== null) {
      clearTimeout(timer);
      timer = null;
    }
  };

  const getUnannounced = () => Array.from(pending.values()).filter(
    (tool) => tool.state === 'pending' || tool.state === 'completed',
  );
  const publicTool = ({ callId, name, args, origin, serverLabel, surfaceOrigin }: PendingTool): ChatProgressTool => ({
    callId,
    name,
    args,
    origin,
    serverLabel,
    surfaceOrigin,
  });

  const flush = () => {
    if (disposed) return;
    clearTimer();
    const doneTools = Array.from(pending.values()).filter((tool) => tool.state === 'completed');
    const runningTools = Array.from(pending.values()).filter(
      (tool) => tool.state === 'pending',
    );
    const groups: ChatProgressAnnouncementGroup[] = [];
    if (doneTools.length > 0) {
      groups.push({ state: 'done', tools: doneTools.map(publicTool) });
    }
    if (runningTools.length > 0) {
      groups.push({ state: 'running', tools: runningTools.map(publicTool) });
    }
    if (groups.length > 0) {
      announce(groups);
      for (const tool of doneTools) tool.state = 'done';
      for (const tool of runningTools) tool.state = 'running';
    }
  };

  const schedule = () => {
    if (disposed || timer !== null || getUnannounced().length === 0) return;
    timer = setTimeout(() => {
      timer = null;
      flush();
    }, Math.max(0, burstWindowMs));
  };

  return {
    toolStarted: (tool) => {
      if (disposed || !tool.callId || !tool.name) return;
      const current = pending.get(tool.callId);
      if (current) {
        if (current.state === 'done') return;
        current.name = tool.name;
        current.args = tool.args ?? current.args;
        current.origin = tool.origin ?? current.origin;
        current.serverLabel = tool.serverLabel ?? current.serverLabel;
        current.surfaceOrigin = tool.surfaceOrigin ?? current.surfaceOrigin;
        // A duplicate start for an already announced or completed call must
        // not reopen it. A retry removes the old entry before this point.
        return;
      }
      pending.set(tool.callId, { ...tool, state: 'pending' });
      schedule();
    },

    toolEnded: (callId, status) => {
      if (disposed || !callId) return;
      const tool = pending.get(callId);
      if (!tool) return;
      if (status === 'error') {
        pending.delete(callId);
      } else {
        // Uma ferramenta já anunciada como running entra no mesmo agrupamento
        // de conclusão. Ferramentas rápidas permanecem na janela para que
        // várias conclusões próximas saiam juntas como done.
        if (tool.state === 'running') {
          tool.state = 'completed';
          schedule();
        } else if (tool.state === 'pending') {
          tool.state = 'completed';
          schedule();
        }
      }
      if (getUnannounced().length === 0) clearTimer();
    },

    toolFailed: (callId, _willRetry = false) => {
      if (disposed || !callId) return;
      pending.delete(callId);
      if (getUnannounced().length === 0) clearTimer();
    },

    finishSegment: () => {
      flush();
      pending.clear();
      clearTimer();
    },

    dispose: () => {
      disposed = true;
      clearTimer();
      pending.clear();
    },
  };
}
