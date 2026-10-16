import { waitForWailsBridge } from './waitForWailsBridge';
import type { UICommandCompletionStatus } from './commandUIExecution';
import type { VoiceInputCommandPort, VoiceInputHandoff } from './commandVoiceInput';
import type { UICommandResult } from './commandUIExecution';

interface CommandVoiceInputAppAPI {
  TakeGlobalVoiceCommand(ticket: string): Promise<VoiceInputHandoff>;
  CompleteUICommand(ticket: string, handoffId: string, status: UICommandCompletionStatus): Promise<void>;
  GetUICommandResult(ticket: string): Promise<UICommandResult>;
  CancelUICommand(ticket: string): Promise<void>;
  AdmitGlobalCommandOccurrence(invocationId: string, admitted: boolean): Promise<boolean>;
}

type WailsCommandVoiceInputWindow = Window & {
  go?: { app?: { App?: Partial<CommandVoiceInputAppAPI> } };
};

export interface CommandVoiceInputWailsOptions {
  readonly target?: WailsCommandVoiceInputWindow;
  readonly signal?: AbortSignal;
  readonly timeoutMs?: number;
  /** Revalidates auth, ownership, and the captured modal generation after bridge readiness. */
  readonly isGlobalJobAdmissionCurrent?: (modalGeneration: string) => boolean;
}

export interface CommandVoiceInputWailsPort extends VoiceInputCommandPort {
  admitGlobalCommandOccurrence(
    invocationId: string,
    admitted: boolean,
    capturedModalGeneration?: string,
  ): Promise<boolean>;
}

function resolveApp(target: WailsCommandVoiceInputWindow): CommandVoiceInputAppAPI {
  const app = target.go?.app?.App;
  if (
    typeof app?.TakeGlobalVoiceCommand !== 'function' ||
    typeof app.CompleteUICommand !== 'function' ||
    typeof app.GetUICommandResult !== 'function' ||
    typeof app.CancelUICommand !== 'function' ||
    typeof app.AdmitGlobalCommandOccurrence !== 'function'
  ) {
    throw new Error('Global voice command Wails API is not available');
  }
  return app as CommandVoiceInputAppAPI;
}

async function appFor(options: CommandVoiceInputWailsOptions): Promise<CommandVoiceInputAppAPI> {
  const target = options.target ?? (window as WailsCommandVoiceInputWindow);
  await waitForWailsBridge({ signal: options.signal, target, timeoutMs: options.timeoutMs });
  return resolveApp(target);
}

export function createCommandVoiceInputWailsPort(
  options: CommandVoiceInputWailsOptions = {},
): CommandVoiceInputWailsPort {
  return Object.freeze({
    takeGlobalVoiceCommand: async (ticket: string) => (await appFor(options)).TakeGlobalVoiceCommand(ticket),
    completeUICommand: async (ticket: string, handoffId: string, status: UICommandCompletionStatus) => {
      await (await appFor(options)).CompleteUICommand(ticket, handoffId, status);
    },
    getUICommandResult: async (ticket: string) => (await appFor(options)).GetUICommandResult(ticket),
    cancelUICommand: async (ticket: string) => {
      await (await appFor(options)).CancelUICommand(ticket);
    },
    admitGlobalCommandOccurrence: async (
      invocationId: string,
      admitted: boolean,
      capturedModalGeneration?: string,
    ) => {
      const app = await appFor(options);
      let current = true;
      if (options.isGlobalJobAdmissionCurrent && capturedModalGeneration !== undefined) {
        try {
          current = options.isGlobalJobAdmissionCurrent(capturedModalGeneration);
        } catch {
          current = false;
        }
      }
      return app.AdmitGlobalCommandOccurrence(invocationId, admitted && current);
    },
  });
}
