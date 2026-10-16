import { waitForWailsBridge } from './waitForWailsBridge';
import type {
  CommandUIExecutionPort,
  UICommandBeginResponse,
  UICommandResult,
  UICommandTakeResponse,
  UICommandCompletionStatus,
} from './commandUIExecution';

interface CommandUIAppAPI {
  BeginUICommand(commandID: string): Promise<UICommandBeginResponse>;
  TakeUICommand(ticket: string): Promise<UICommandTakeResponse>;
  CompleteUICommand(ticket: string, handoffId: string, status: UICommandCompletionStatus): Promise<void>;
  GetUICommandResult(ticket: string): Promise<UICommandResult>;
  CancelUICommand(ticket: string): Promise<void>;
}

type WailsCommandUIWindow = Window & {
  go?: { app?: { App?: Partial<CommandUIAppAPI> } };
};

export interface CommandUIExecutionWailsOptions {
  readonly signal?: AbortSignal;
  readonly target?: WailsCommandUIWindow;
  readonly timeoutMs?: number;
}

function resolveApp(target: WailsCommandUIWindow): CommandUIAppAPI {
  const app = target.go?.app?.App;
  if (
    typeof app?.BeginUICommand !== 'function' ||
    typeof app.TakeUICommand !== 'function' ||
    typeof app.CompleteUICommand !== 'function' ||
    typeof app.GetUICommandResult !== 'function' ||
    typeof app.CancelUICommand !== 'function'
  ) {
    throw new Error('Command UI execution Wails API is not available');
  }
  return app as CommandUIAppAPI;
}

async function appFor(options: CommandUIExecutionWailsOptions): Promise<CommandUIAppAPI> {
  const target = options.target ?? (window as WailsCommandUIWindow);
  await waitForWailsBridge({ signal: options.signal, target, timeoutMs: options.timeoutMs });
  return resolveApp(target);
}

export function createCommandUIExecutionWailsPort(
  options: CommandUIExecutionWailsOptions = {},
): CommandUIExecutionPort {
  return Object.freeze({
    beginUICommand: async (commandID: string) => (await appFor(options)).BeginUICommand(commandID),
    takeUICommand: async (ticket: string) => (await appFor(options)).TakeUICommand(ticket),
    completeUICommand: async (ticket: string, handoffId: string, status: UICommandCompletionStatus) => {
      await (await appFor(options)).CompleteUICommand(ticket, handoffId, status);
    },
    getUICommandResult: async (ticket: string) => (await appFor(options)).GetUICommandResult(ticket),
    cancelUICommand: async (ticket: string) => {
      await (await appFor(options)).CancelUICommand(ticket);
    },
  });
}
