import { waitForWailsBridge } from './waitForWailsBridge';
import type { BackendCommandExecutionResult, CommandBackendExecutionPort } from './commandBackendExecution';

interface BackendAppAPI {
  ExecutePaletteCommand(commandID: string, args: Record<string, unknown>): Promise<BackendCommandExecutionResult>;
}
type BackendWindow = Window & { go?: { app?: { App?: Partial<BackendAppAPI> } } };
export interface CommandBackendExecutionWailsOptions { readonly signal?: AbortSignal; readonly target?: BackendWindow; readonly timeoutMs?: number; }

async function appFor(options: CommandBackendExecutionWailsOptions): Promise<BackendAppAPI> {
  const target = options.target ?? (window as BackendWindow);
  await waitForWailsBridge({ signal: options.signal, target, timeoutMs: options.timeoutMs });
  const app = target.go?.app?.App;
  if (typeof app?.ExecutePaletteCommand !== 'function') throw new Error('Backend command execution Wails API is not available');
  return app as BackendAppAPI;
}

export function createCommandBackendExecutionWailsPort(options: CommandBackendExecutionWailsOptions = {}): CommandBackendExecutionPort {
  return Object.freeze({
    executeCommand: async (commandID: string, args: Record<string, unknown>) => (await appFor(options)).ExecutePaletteCommand(commandID, args),
  });
}
