import { waitForWailsBridge } from '../lib/waitForWailsBridge';

export interface CommandDeckCaptureWailsAPI {
  BeginCommandDeckCapture(requestID: string): Promise<void>;
  CancelCommandDeckCapture(requestID: string): Promise<void>;
}

export type CommandDeckCaptureWindow = Window & {
  go?: { app?: { App?: Partial<CommandDeckCaptureWailsAPI> } };
};

function resolveApp(target: CommandDeckCaptureWindow): CommandDeckCaptureWailsAPI {
  const app = target.go?.app?.App;
  if (
    typeof app?.BeginCommandDeckCapture !== 'function' ||
    typeof app.CancelCommandDeckCapture !== 'function'
  ) throw new Error('Command Deck capture Wails API is not available');
  return app as CommandDeckCaptureWailsAPI;
}

async function appFor(target?: CommandDeckCaptureWindow): Promise<CommandDeckCaptureWailsAPI> {
  const resolvedTarget = target ?? (window as CommandDeckCaptureWindow);
  await waitForWailsBridge({ target: resolvedTarget });
  return resolveApp(resolvedTarget);
}

export async function beginCommandDeckCapture(
  requestId: string,
  target?: CommandDeckCaptureWindow
): Promise<void> {
  await (await appFor(target)).BeginCommandDeckCapture(requestId);
}

export async function cancelCommandDeckCapture(
  requestId: string,
  target?: CommandDeckCaptureWindow
): Promise<void> {
  await (await appFor(target)).CancelCommandDeckCapture(requestId);
}
