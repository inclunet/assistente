import { waitForWailsBridge } from './waitForWailsBridge';
import type { CommandKeyboardTrigger } from './commandShortcut';
import type { BackendCommandExecutionResult } from './commandBackendExecution';
import type { UICommandBeginResponse } from './commandUIExecution';
import type { LocalCommandContextualBinding, LocalCommandPaletteCondition } from './commandLocalKeyboard';

export interface LocalCommandKeyboardContext {
  surfaceId: string;
  surfaceType: string;
  profile?: string;
  appPage?: import('./commandAppPage').AppPage;
  isCurrent?: () => boolean;
}

export interface LocalCommandKeyboardBinding {
  shortcut: CommandKeyboardTrigger;
  commandId: string;
  handler: 'backend' | 'ui' | 'contextual' | 'local_ui';
}

export interface LocalCommandKeyboardMap {
  validUntil?: number;
  generation: string;
  ownerId?: string;
  sessionId?: string;
  workspaceId?: string;
  bindings: LocalCommandKeyboardBinding[];
  localPaletteCommands?: string[];
  localPaletteConditions?: LocalCommandPaletteCondition[];
  contextualPaletteConditions?: LocalCommandPaletteCondition[];
  contextualBindings?: LocalCommandContextualBinding[];
}

interface LocalCommandKeyboardAppAPI {
  GetLocalCommandKeyboardMap(): Promise<LocalCommandKeyboardMap>;
  DispatchLocalCommandKey(generation: string, shortcut: CommandKeyboardTrigger, kind: string, repeat: boolean): Promise<BackendCommandExecutionResult | null>;
  BeginLocalCommandUIKey(generation: string, shortcut: CommandKeyboardTrigger, repeat: boolean): Promise<UICommandBeginResponse | null>;
  DispatchContextualLocalCommandKey(generation: string, shortcut: CommandKeyboardTrigger, kind: string, repeat: boolean, context: LocalCommandKeyboardContext): Promise<BackendCommandExecutionResult | null>;
  BeginContextualLocalCommandUIKey(generation: string, shortcut: CommandKeyboardTrigger, repeat: boolean, context: LocalCommandKeyboardContext): Promise<UICommandBeginResponse | null>;
  ResetLocalCommandKeyboard(generation: string): Promise<void>;
}

type LocalCommandKeyboardWindow = Window & { go?: { app?: { App?: Partial<LocalCommandKeyboardAppAPI> } } };

export interface CommandLocalKeyboardWailsPort {
  loadMap(): Promise<LocalCommandKeyboardMap>;
  dispatchLocalCommandKey(generation: string, shortcut: CommandKeyboardTrigger, kind: 'down' | 'up', repeat: boolean, context?: LocalCommandKeyboardContext): Promise<BackendCommandExecutionResult | null>;
  beginLocalCommandUIKey(generation: string, shortcut: CommandKeyboardTrigger, repeat: boolean, context?: LocalCommandKeyboardContext): Promise<UICommandBeginResponse | null>;
  resetLocalCommandKeyboard(generation: string): Promise<void>;
}

export interface CommandLocalKeyboardWailsOptions {
  readonly target?: LocalCommandKeyboardWindow;
  readonly signal?: AbortSignal;
  readonly timeoutMs?: number;
}

function cloneKeyboardTrigger(trigger: CommandKeyboardTrigger): CommandKeyboardTrigger {
  if (trigger.version === 1) return { version: 1, code: trigger.code, modifiers: [...trigger.modifiers] };
  return {
    version: 2,
    steps: trigger.steps.map(step => ({ code: step.code, modifiers: [...step.modifiers] })) as [typeof trigger.steps[0], typeof trigger.steps[1]],
  };
}

function cloneKeyboardBinding(binding: LocalCommandKeyboardBinding | null): LocalCommandKeyboardBinding | null {
  return binding && { ...binding, shortcut: cloneKeyboardTrigger(binding.shortcut) };
}

function clonePaletteCondition(condition: LocalCommandPaletteCondition): LocalCommandPaletteCondition {
  const bySurfaceId = condition.bySurfaceId === undefined ? undefined : Object.create(null) as Record<string, Record<string, boolean>>;
  if (bySurfaceId) {
    for (const [surfaceType, surfaceIds] of Object.entries(condition.bySurfaceId!)) {
      const clonedIds: Record<string, boolean> = Object.create(null);
      for (const [surfaceId, enabled] of Object.entries(surfaceIds)) clonedIds[surfaceId] = enabled;
      bySurfaceId[surfaceType] = clonedIds;
    }
  }
  const byProfile = condition.byProfile === undefined ? undefined : Object.create(null) as Record<string, LocalCommandPaletteCondition>;
  if (byProfile) {
    for (const [profile, nested] of Object.entries(condition.byProfile!)) byProfile[profile] = clonePaletteCondition(nested);
  }
  const byPage = condition.byPage === undefined ? undefined : Object.create(null) as Record<string, LocalCommandPaletteCondition>;
  if (byPage) {
    for (const [page, nested] of Object.entries(condition.byPage!)) byPage[page] = clonePaletteCondition(nested);
  }
  return {
    commandId: condition.commandId,
    bySurface: { ...condition.bySurface },
    ...(bySurfaceId ? { bySurfaceId } : {}),
    ...(byProfile ? { byProfile } : {}),
    ...(byPage ? { byPage } : {}),
    fallback: condition.fallback,
  };
}

function cloneKeyboardMap(map: LocalCommandKeyboardMap): LocalCommandKeyboardMap {
  return {
    ...map,
    bindings: map.bindings.map(binding => ({ ...binding, shortcut: cloneKeyboardTrigger(binding.shortcut) })),
    localPaletteCommands: map.localPaletteCommands?.slice(),
    localPaletteConditions: map.localPaletteConditions?.map(clonePaletteCondition),
    contextualPaletteConditions: map.contextualPaletteConditions?.map(clonePaletteCondition),
    contextualBindings: map.contextualBindings?.map(cloneContextualBinding),
  };
}

function cloneContextualBinding(entry: LocalCommandContextualBinding): LocalCommandContextualBinding {
  const bySurface: Record<string, LocalCommandKeyboardBinding | null> = Object.create(null);
  for (const [surfaceType, branch] of Object.entries(entry.bySurface)) bySurface[surfaceType] = cloneKeyboardBinding(branch);
  const bySurfaceId = entry.bySurfaceId === undefined ? undefined : Object.create(null) as NonNullable<typeof entry.bySurfaceId>;
  if (bySurfaceId) {
    for (const [surfaceType, surfaceIds] of Object.entries(entry.bySurfaceId!)) {
      const clonedIds: Record<string, LocalCommandKeyboardBinding | null> = Object.create(null);
      for (const [surfaceId, branch] of Object.entries(surfaceIds)) clonedIds[surfaceId] = cloneKeyboardBinding(branch);
      bySurfaceId[surfaceType] = clonedIds;
    }
  }
  const sequenceFallbacks = entry.sequenceFallbacks === undefined ? undefined : Object.create(null) as NonNullable<typeof entry.sequenceFallbacks>;
  if (sequenceFallbacks) {
    for (const [surfaceType, surfaceIds] of Object.entries(entry.sequenceFallbacks!)) {
      const clonedIds: Record<string, true> = Object.create(null);
      for (const [surfaceId, flag] of Object.entries(surfaceIds)) clonedIds[surfaceId] = flag;
      sequenceFallbacks[surfaceType] = clonedIds;
    }
  }
  const byProfile = entry.byProfile === undefined ? undefined : Object.create(null) as NonNullable<typeof entry.byProfile>;
  if (byProfile) {
    for (const [profile, projection] of Object.entries(entry.byProfile!)) {
      byProfile[profile] = cloneContextualBinding(projection);
    }
  }
  const byPage = entry.byPage === undefined ? undefined : Object.create(null) as NonNullable<typeof entry.byPage>;
  if (byPage) {
    for (const [page, projection] of Object.entries(entry.byPage!)) byPage[page] = cloneContextualBinding(projection);
  }
  return {
    ...entry,
    shortcut: cloneKeyboardTrigger(entry.shortcut),
    bySurface,
    ...(bySurfaceId ? { bySurfaceId } : {}),
    ...(sequenceFallbacks ? { sequenceFallbacks } : {}),
    ...(byProfile ? { byProfile } : {}),
    ...(byPage ? { byPage } : {}),
    fallback: cloneKeyboardBinding(entry.fallback),
  };
}

async function appFor(options: CommandLocalKeyboardWailsOptions): Promise<LocalCommandKeyboardAppAPI> {
  const target = options.target ?? (window as LocalCommandKeyboardWindow);
  await waitForWailsBridge({ target, signal: options.signal, timeoutMs: options.timeoutMs });
  const app = target.go?.app?.App;
  if (typeof app?.GetLocalCommandKeyboardMap !== 'function' ||
      typeof app.DispatchLocalCommandKey !== 'function' ||
      typeof app.BeginLocalCommandUIKey !== 'function' ||
      typeof app.ResetLocalCommandKeyboard !== 'function') {
    throw new Error('Local command keyboard Wails API is not available');
  }
  return app as LocalCommandKeyboardAppAPI;
}

export function createCommandLocalKeyboardWailsPort(
  options: CommandLocalKeyboardWailsOptions = {},
): CommandLocalKeyboardWailsPort {
  return Object.freeze({
    loadMap: async (): Promise<LocalCommandKeyboardMap> => cloneKeyboardMap(await (await appFor(options)).GetLocalCommandKeyboardMap()),
    dispatchLocalCommandKey: async (generation: string, shortcut: CommandKeyboardTrigger, kind: 'down' | 'up', repeat: boolean, context?: LocalCommandKeyboardContext) => {
      const app = await appFor(options);
      if (!context) return app.DispatchLocalCommandKey(generation, shortcut, kind, repeat);
      if (context.isCurrent && !context.isCurrent()) throw new Error('Contextual keyboard context stale');
      if (typeof app.DispatchContextualLocalCommandKey !== 'function') throw new Error('Contextual keyboard API unavailable');
      return app.DispatchContextualLocalCommandKey(generation, shortcut, kind, repeat, {
        surfaceId: context.surfaceId,
        surfaceType: context.surfaceType,
        ...(context.profile !== undefined ? { profile: context.profile } : {}),
        ...(context.appPage !== undefined ? { appPage: context.appPage } : {}),
      });
    },
    beginLocalCommandUIKey: async (generation: string, shortcut: CommandKeyboardTrigger, repeat: boolean, context?: LocalCommandKeyboardContext) => {
      const app = await appFor(options);
      if (!context) return app.BeginLocalCommandUIKey(generation, shortcut, repeat);
      if (context.isCurrent && !context.isCurrent()) throw new Error('Contextual keyboard context stale');
      if (typeof app.BeginContextualLocalCommandUIKey !== 'function') throw new Error('Contextual keyboard API unavailable');
      return app.BeginContextualLocalCommandUIKey(generation, shortcut, repeat, {
        surfaceId: context.surfaceId,
        surfaceType: context.surfaceType,
        ...(context.profile !== undefined ? { profile: context.profile } : {}),
        ...(context.appPage !== undefined ? { appPage: context.appPage } : {}),
      });
    },
    resetLocalCommandKeyboard: async (generation: string) => {
      await (await appFor(options)).ResetLocalCommandKeyboard(generation);
    },
  });
}
