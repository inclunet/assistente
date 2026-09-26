import { waitForWailsBridge } from './waitForWailsBridge';
import type { AppPage } from './commandAppPage';

interface CommandDeckPagePresentationAppAPI {
  PublishCommandDeckPagePresentation(appPage: AppPage, generation: string, revision: number): Promise<void>;
  ClearCommandDeckPagePresentation(generation: string, revision: number): Promise<void>;
}

type CommandDeckPagePresentationWindow = Window & { go?: { app?: { App?: Partial<CommandDeckPagePresentationAppAPI> } } };

export interface CommandDeckPagePresentationPort {
  publish(appPage: AppPage, generation: string): Promise<void>;
  clear(generation: string): Promise<void>;
}

let lastRevision = Date.now() * 1000;

export function nextCommandDeckPagePresentationRevision(): number {
  lastRevision = Math.max(lastRevision + 1, Date.now() * 1000);
  return lastRevision;
}

export interface CommandDeckPagePresentationWailsOptions {
  readonly target?: CommandDeckPagePresentationWindow;
  readonly signal?: AbortSignal;
  readonly timeoutMs?: number;
}

async function appFor(options: CommandDeckPagePresentationWailsOptions): Promise<CommandDeckPagePresentationAppAPI> {
  const target = options.target ?? (window as CommandDeckPagePresentationWindow);
  await waitForWailsBridge({ target, signal: options.signal, timeoutMs: options.timeoutMs });
  const app = target.go?.app?.App;
  if (typeof app?.PublishCommandDeckPagePresentation !== 'function' ||
      typeof app.ClearCommandDeckPagePresentation !== 'function') {
    throw new Error('Command Deck page presentation Wails API is not available');
  }
  return app as CommandDeckPagePresentationAppAPI;
}

export function createCommandDeckPagePresentationWailsPort(
  options: CommandDeckPagePresentationWailsOptions = {},
): CommandDeckPagePresentationPort {
  return Object.freeze({
    publish: async (appPage: AppPage, generation: string) => {
      const revision = nextCommandDeckPagePresentationRevision();
      await (await appFor(options)).PublishCommandDeckPagePresentation(appPage, generation, revision);
    },
    clear: async (generation: string) => {
      const revision = nextCommandDeckPagePresentationRevision();
      await (await appFor(options)).ClearCommandDeckPagePresentation(generation, revision);
    },
  });
}
