import {
  createCommandUIExecution,
  type CommandUIExecution,
  type CommandUIExecutionOptions,
  type CommandUIExecutionPort,
  type UICommandTakeResponse,
} from './commandUIExecution';
import { EDITOR_MODE_COMMAND_IDS, isEditorModeCommand } from './commandEditorMode';

export const WORKSPACE_CHAT_CREATE_COMMAND_ID = 'workspace.tab.chat.create';
export const WORKSPACE_CHAT_OPEN_COMMAND_ID = 'workspace.chat.open';
export const WORKSPACE_TAB_CLOSE_COMMAND_ID = 'workspace.tab.close';
export const WORKSPACE_CREATE_COMMAND_ID = 'workspace.create';
export const WORKSPACE_TAB_CREATE_COMMAND_IDS = [
  WORKSPACE_CHAT_CREATE_COMMAND_ID, 'workspace.tab.editor.create', 'workspace.tab.tasklist.create',
  'workspace.tab.terminal.create',
] as const;

export function isWorkspaceTabCreateCommand(commandID: unknown): commandID is typeof WORKSPACE_TAB_CREATE_COMMAND_IDS[number] {
  return WORKSPACE_TAB_CREATE_COMMAND_IDS.some((id) => id === commandID);
}

export const WORKSPACE_TAB_MUTATION_COMMAND_IDS = [...WORKSPACE_TAB_CREATE_COMMAND_IDS, WORKSPACE_TAB_CLOSE_COMMAND_ID] as const;

export function isWorkspaceTabMutationCommand(commandID: unknown): commandID is typeof WORKSPACE_TAB_MUTATION_COMMAND_IDS[number] {
  return isWorkspaceTabCreateCommand(commandID) || commandID === WORKSPACE_TAB_CLOSE_COMMAND_ID;
}

export const WORKSPACE_MUTATION_COMMAND_IDS = [
  ...WORKSPACE_TAB_MUTATION_COMMAND_IDS,
  WORKSPACE_CREATE_COMMAND_ID,
  WORKSPACE_CHAT_OPEN_COMMAND_ID,
  ...EDITOR_MODE_COMMAND_IDS,
] as const;

export function isWorkspaceMutationCommand(commandID: unknown): commandID is typeof WORKSPACE_MUTATION_COMMAND_IDS[number] {
  return isWorkspaceTabMutationCommand(commandID) ||
    commandID === WORKSPACE_CREATE_COMMAND_ID || commandID === WORKSPACE_CHAT_OPEN_COMMAND_ID ||
    (typeof commandID === 'string' && isEditorModeCommand(commandID));
}

export interface CommandContextualBackendPort extends CommandUIExecutionPort {
  /** Submissão da escrita; não é uma confirmação de efeito visual. */
  commitBackendCommand(ticket: string, handoffID: string): Promise<void>;
}

export interface CommandBackendTarget {
  isCurrent(): boolean;
  /** Preparação síncrona do domínio UI, antes de submeter a escrita. */
  prepare?(): boolean;
  dispose(): void;
}

export interface CommandContextualBackendOptions
  extends Pick<CommandUIExecutionOptions, 'trustedSession' | 'surfaceID' | 'canCommitEffect'> {
  prepareTarget(commandID: string): CommandBackendTarget | undefined;
}

/**
 * Reutiliza apenas o protocolo de handoff e seus guards. O efeito local é a
 * submissão síncrona à porta de backend: não cria abas nem aplica resultados.
 * A conclusão aguarda a escrita real e consulta o ledger. CompleteUICommand
 * nunca recebe succeeded/failed da UI para este comando.
 */
export function createCommandContextualBackendExecution(
  port: CommandContextualBackendPort,
  options: CommandContextualBackendOptions,
): CommandUIExecution {
  if (typeof port?.commitBackendCommand !== 'function' || typeof options?.prepareTarget !== 'function') {
    throw new TypeError('contextual-backend-invalid-port');
  }
  // Cada ID tem estado de handoff/submissão próprio. Chamadas concorrentes
  // de tipos diferentes nunca compartilham tickets ou resultados.
  const coordinators = new Map<string, CommandUIExecution>();
  let disposed = false;
  return Object.freeze({
    execute(commandID: string) {
      if (disposed || !isWorkspaceMutationCommand(commandID)) {
        return Promise.resolve({ invocationId: '', status: 'cancelled' as const,
          errorCode: disposed ? 'disposed' : 'ui-handler-missing' });
      }
      let coordinator = coordinators.get(commandID);
      if (!coordinator) {
        coordinator = createWorkspaceTabExecution(commandID, port, options);
        coordinators.set(commandID, coordinator);
      }
      return coordinator.execute(commandID);
    },
    async cancel(commandID?: string) {
      if (commandID !== undefined) await coordinators.get(commandID)?.cancel(commandID);
      else await Promise.all([...coordinators.values()].map((coordinator) => coordinator.cancel()));
    },
    dispose() {
      if (disposed) return;
      disposed = true;
      coordinators.forEach((coordinator) => coordinator.dispose());
    },
  });
}

function createWorkspaceTabExecution(
  commandID: string,
  port: CommandContextualBackendPort,
  options: CommandContextualBackendOptions,
): CommandUIExecution {
  // Uma instância aceita apenas este comando. O coordenador deduplica chamadas
  // concorrentes e não reutiliza a preparação depois de um desfecho terminal.
  let taken: UICommandTakeResponse | undefined;
  let submitted: Promise<void> | undefined;
  let preparationRejected = false;
  const coordinator = createCommandUIExecution({
    ...port,
    beginUICommand: (commandID) => {
      taken = undefined;
      submitted = undefined;
      preparationRejected = false;
      return port.beginUICommand(commandID);
    },
    takeUICommand: async (ticket) => {
      const handoff = await port.takeUICommand(ticket);
      taken = handoff;
      return handoff;
    },
    cancelUICommand: (ticket) => {
      if (preparationRejected && taken?.ticket === ticket && !submitted) {
        return port.completeUICommand(ticket, taken.handoffId, 'cancelled');
      }
      return port.cancelUICommand(ticket);
    },
    completeUICommand: async (ticket, handoffID, status) => {
      if (status === 'cancelled') {
        await port.completeUICommand(ticket, handoffID, status);
        return;
      }
      if (!taken || taken.ticket !== ticket || taken.handoffId !== handoffID || !submitted) {
        throw new Error('contextual-backend-not-submitted');
      }
      // O status real virá de GetUICommandResult, inclusive se a confirmação
      // de transporte se perder. Não inferir falha nem repetir a mutação.
      await submitted;
    },
  }, new Map([[commandID, () => {
    throw new Error('contextual-backend-target-required');
  }]]), {
    ...options,
    prepareEffect: () => {
      const target = options.prepareTarget(commandID);
      if (!target) return undefined;
      return {
        isCurrent: () => target.isCurrent(),
        dispose: () => target.dispose(),
        effect: () => {
          if (!target.isCurrent() || !taken || taken.commandId !== commandID) {
            throw new Error('contextual-backend-stale-target');
          }
          try {
            if ((target.prepare && target.prepare() !== true) || !target.isCurrent()) throw new Error('contextual-backend-stale-target');
          } catch (error) {
            preparationRejected = true;
            throw error;
          }
          // Não adicionar await: revalidação e chamada da porta pertencem ao
          // mesmo trecho síncrono. A fachada também submete sem espera local.
          submitted = port.commitBackendCommand(taken.ticket, taken.handoffId);
          // Anexar observador imediatamente evita rejeição sem observador
          // antes de o coordenador chegar à fase de confirmação.
          void submitted.catch(() => undefined);
          return undefined;
        },
      };
    },
  });
  return coordinator;
}
