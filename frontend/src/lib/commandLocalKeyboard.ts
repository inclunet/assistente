import {
  commandShortcutFromKeyboardEvent,
  createCommandModifierState,
  isCommandKeyboardTrigger,
  normalizeCommandShortcut,
  normalizeCommandShortcutSequence,
  serializeCommandKeyboardTrigger,
  serializeCommandShortcut,
} from './commandShortcut';
import type { CommandKeyboardTrigger, CommandShortcut, CommandShortcutSequenceStep } from './commandShortcut';
import { observeCommandShortcutComposition } from './commandFocusContext';
import { isLocalUICommand } from './commandLocalUI';

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

export interface LocalCommandPaletteCondition {
  commandId: string;
  bySurface: Record<string, boolean>;
  bySurfaceId?: Record<string, Record<string, boolean>>;
  byProfile?: Record<string, LocalCommandPaletteCondition>;
  fallback: boolean;
}

/** Resoluções calculadas pelo host; null é uma barreira, não fallback. */
export interface LocalCommandContextualBinding {
  fallbackToSequences?: boolean;
  /** v1 NoMatch explícito: surfaceType -> surfaceId ('' = não listado/por tipo). */
  sequenceFallbacks?: Record<string, Record<string, true>>;
  shortcut: CommandKeyboardTrigger;
  bySurface: Record<string, LocalCommandKeyboardBinding | null>;
  bySurfaceId?: Record<string, Record<string, LocalCommandKeyboardBinding | null>>;
  /** Projeções por perfil; a raiz é a projeção para perfis não listados. */
  byProfile?: Record<string, LocalCommandContextualBinding>;
  fallback: LocalCommandKeyboardBinding | null;
}

export type LocalCommandKeyboardHandler = 'backend' | 'ui' | 'contextual' | 'local_ui';

export interface LocalCommandKeyboardBinding {
  shortcut: CommandKeyboardTrigger;
  commandId: string;
  handler: LocalCommandKeyboardHandler;
}

export interface LocalCommandKeyRequest {
  generation: string;
  shortcut: CommandKeyboardTrigger;
  commandId: string;
  handler: 'backend' | 'ui' | 'contextual' | 'local_ui';
  kind: 'down' | 'up';
  repeat: boolean;
  context?: LocalCommandKeyContext;
}

export interface LocalCommandKeyContext {
  surfaceId: string;
  surfaceType: string;
  profile?: string;
}

export interface LocalCommandContextLease extends LocalCommandKeyContext {
  isCurrent: () => boolean;
  /** Narrow exceptions may admit only a fixed set of already-resolved commands. */
  allowedCommandIds?: readonly string[];
}

export interface LocalCommandKeyboardOptions {
  target: Window;
  /** Native ownership is checked synchronously before local resolution. */
  ownedGlobally?: (event: KeyboardEvent) => boolean;
  loadMap: () => Promise<LocalCommandKeyboardMap>;
  onDown: (request: LocalCommandKeyRequest) => Promise<void>;
  onUp: (request: LocalCommandKeyRequest) => Promise<void>;
  reset: (generation: string) => Promise<void>;
  /** Bloqueio global; recebe o comando resolvido para exceções estreitas como ajuda em modal. */
  blocked: (commandID?: string, event?: KeyboardEvent) => boolean;
  /** Restrição local por comando; executada somente para uma tecla mapeada. */
  canHandle?: (commandID: string, event: KeyboardEvent) => boolean;
  /** Exceção explícita para comandos mapeados que podem consumir teclas editáveis. */
  canHandleEditable?: (commandID: string, event: KeyboardEvent) => boolean;
  /** Repeat físico seletivo; a tecla precisa continuar pressionada. */
  canRepeat?: (commandID: string) => boolean;
  readSurfaceType?: (event: KeyboardEvent) => string | undefined;
  readContext?: (event: KeyboardEvent) => LocalCommandContextLease | undefined;
  acceptMap?: (map: LocalCommandKeyboardMap) => boolean;
  onMapAccepted?: (map: LocalCommandKeyboardMap) => void;
  onMapInvalidated?: () => void;
  onSequenceStarted?: (bindings: readonly LocalCommandKeyboardBinding[]) => void | Promise<void>;
  onSequenceCancelled?: (reason: CommandSequenceCancelReason) => void;
  sequenceTimeoutMs?: number;
}

export type CommandSequenceCancelReason = 'timeout' | 'escape' | 'unexpected' | 'menu-navigation' | 'blocked' | 'ime' | 'altgraph' | 'blur' | 'refresh' | 'dispose';

export interface LocalCommandKeyboardController {
  refresh(): Promise<void>;
  cancelSequence(): void;
  dispose(): void;
}

type Press = {
  surfaceType?: string;
  context?: LocalCommandKeyContext;
  generation: string;
  shortcut: CommandKeyboardTrigger;
  commandId: string;
  handler: 'backend' | 'ui' | 'contextual' | 'local_ui';
  downPromise: Promise<void>;
};

const MODIFIER_ONLY = /^(Control|Alt|Shift|Meta)(Left|Right)?$/;
const MENU_NAVIGATION_CODES = new Set(['ArrowUp', 'ArrowDown', 'Home', 'End', 'Tab', 'Enter']);

function hasCommandModifier(shortcut: Pick<CommandShortcut, 'code' | 'modifiers'>): boolean {
  return shortcut.modifiers.some((modifier) => modifier === 'Control' || modifier === 'Alt' || modifier === 'Meta');
}

function isBareWhitelistedFunctionKey(shortcut: Pick<CommandShortcut, 'code' | 'modifiers'>): boolean {
  return ((shortcut.code === 'F1' || shortcut.code === 'F5') && shortcut.modifiers.length === 0) ||
    (shortcut.code === 'F6' && (shortcut.modifiers.length === 0 ||
      shortcut.modifiers.length === 1 && shortcut.modifiers[0] === 'Shift'));
}

function isEditableTarget(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) return false;
  if (target instanceof HTMLInputElement || target instanceof HTMLTextAreaElement || target instanceof HTMLSelectElement) {
    return true;
  }
  const editable = target.closest('[contenteditable]');
  if (editable && editable.getAttribute('contenteditable') !== 'false') return true;
  return Boolean(target.closest('.monaco-editor'));
}

function shortcutForEvent(event: KeyboardEvent, explicitControlAlt = false): CommandShortcut | null {
  if (event.isComposing || event.keyCode === 229 || event.getModifierState('AltGraph')) return null;
  if (MODIFIER_ONLY.test(event.code)) return null;
  return commandShortcutFromKeyboardEvent({
    code: event.code,
    repeat: false,
    isComposing: false,
    keyCode: event.keyCode,
    ctrlKey: event.ctrlKey,
    altKey: event.altKey,
    shiftKey: event.shiftKey,
    metaKey: event.metaKey,
    getModifierState: event.getModifierState.bind(event),
  }, explicitControlAlt);
}

function validCommandId(value: unknown): value is string {
  return typeof value === 'string' && /^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$/.test(value);
}

function cloneShortcut(shortcut: CommandKeyboardTrigger): CommandKeyboardTrigger {
  if (shortcut.version === 1) return { version: 1, code: shortcut.code, modifiers: [...shortcut.modifiers] };
  return {
    version: 2,
    steps: shortcut.steps.map((step) => ({ code: step.code, modifiers: [...step.modifiers] })) as [CommandShortcutSequenceStep, CommandShortcutSequenceStep],
  };
}

type ValidatedBindings = {
  simple: Map<string, LocalCommandKeyboardBinding>;
  sequences: Map<string, LocalCommandKeyboardBinding[]>;
  contextual: Map<string, LocalCommandContextualBinding>;
  contextualSequences: Map<string, LocalCommandContextualBinding[]>;
};

function stepKey(step: CommandShortcutSequenceStep): string {
  return serializeCommandShortcut({ version: 1, code: step.code, modifiers: step.modifiers });
}

function contextualKey(trigger: CommandKeyboardTrigger): string {
  return trigger.version === 1 ? serializeCommandShortcut(trigger) : stepKey(trigger.steps[0]);
}

function sameContext(left: LocalCommandKeyContext, right: LocalCommandKeyContext): boolean {
  return left.surfaceId === right.surfaceId && left.surfaceType === right.surfaceType &&
    (left.profile === undefined && right.profile === undefined || left.profile !== undefined && right.profile !== undefined && left.profile === right.profile);
}

export interface LocalCommandContextualResolution {
  matched: boolean;
  branch: LocalCommandKeyboardBinding | null;
  barrier: boolean;
  requiresLease: boolean;
}

/** Seleção pura da projeção contextual; frescor/lease pertencem ao dispatcher. */
export function resolveLocalCommandContextualBinding(
  entry: LocalCommandContextualBinding,
  surfaceType: string,
  context?: LocalCommandKeyContext,
): LocalCommandContextualResolution {
  const hasProfileBranches = entry.byProfile !== undefined;
  if (hasProfileBranches && !WORKSPACE_SURFACE_TYPES.has(surfaceType)) {
    return { matched: false, branch: null, barrier: true, requiresLease: true };
  }
  if (hasProfileBranches) {
    if (!context || context.surfaceType !== surfaceType || context.profile === undefined) {
      return { matched: false, branch: null, barrier: true, requiresLease: true };
    }
    entry = entry.byProfile![context.profile] ?? entry;
  }
  const byType = entry.bySurfaceId && Object.prototype.hasOwnProperty.call(entry.bySurfaceId, surfaceType)
    ? entry.bySurfaceId[surfaceType] : undefined;
  const hasIdBranches = Boolean(byType && Object.keys(byType).length > 0);
  if (hasIdBranches) {
    if (!context || context.surfaceType !== surfaceType) return { matched: false, branch: null, barrier: true, requiresLease: true };
    if (Object.prototype.hasOwnProperty.call(byType!, context.surfaceId)) {
      const branch = byType![context.surfaceId];
      if (branch === null && entry.shortcut.version === 1 && entry.sequenceFallbacks?.[surfaceType]?.[context.surfaceId] === true) {
        return { matched: false, branch: null, barrier: false, requiresLease: true };
      }
      return { matched: true, branch, barrier: branch === null, requiresLease: true };
    }
    if (entry.shortcut.version === 1 && entry.sequenceFallbacks?.[surfaceType]?.[''] === true) {
      return { matched: false, branch: null, barrier: false, requiresLease: true };
    }
  }
  if (Object.prototype.hasOwnProperty.call(entry.bySurface, surfaceType)) {
    const branch = entry.bySurface[surfaceType];
    if (branch === null && entry.shortcut.version === 1 && entry.sequenceFallbacks?.[surfaceType]?.[''] === true) {
      return { matched: false, branch: null, barrier: false, requiresLease: hasIdBranches || hasProfileBranches };
    }
    return { matched: true, branch, barrier: branch === null, requiresLease: hasIdBranches || hasProfileBranches };
  }
  if (entry.fallback) return { matched: true, branch: entry.fallback, barrier: false, requiresLease: hasIdBranches || hasProfileBranches };
  if (entry.shortcut.version === 2 || entry.fallbackToSequences === true) {
    return { matched: false, branch: null, barrier: false, requiresLease: hasIdBranches || hasProfileBranches };
  }
  return { matched: false, branch: null, barrier: true, requiresLease: hasIdBranches || hasProfileBranches };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function safeContextKey(value: string): boolean {
  return value.length > 0 && value.trim() === value && !['__proto__', 'constructor', 'prototype'].includes(value);
}

const WORKSPACE_SURFACE_TYPES = new Set(['chat', 'editor', 'terminal', 'tasklist']);

function validateContextualEntry(raw: unknown, allowProfiles: boolean, workspaceOnly = false): LocalCommandContextualBinding | null {
  if (!isRecord(raw) || !isCommandKeyboardTrigger(raw.shortcut) ||
      (!hasCommandModifier(raw.shortcut.version === 1 ? raw.shortcut : raw.shortcut.steps[0]) &&
        !isBareWhitelistedFunctionKey(raw.shortcut.version === 1 ? raw.shortcut : raw.shortcut.steps[0])) ||
      !isRecord(raw.bySurface)) return null;
  const key = contextualKey(raw.shortcut);
  if (raw.fallbackToSequences !== undefined && typeof raw.fallbackToSequences !== 'boolean') return null;
  if (raw.shortcut.version === 2 && raw.fallbackToSequences !== undefined) return null;
  if (raw.sequenceFallbacks !== undefined && raw.shortcut.version !== 1) return null;
  const bySurface: Record<string, LocalCommandKeyboardBinding | null> = Object.create(null);
  const bySurfaceId: Record<string, Record<string, LocalCommandKeyboardBinding | null>> | undefined =
    raw.bySurfaceId === undefined ? undefined : Object.create(null);
  const validateBranch = (branch: unknown): LocalCommandKeyboardBinding | null | undefined => {
    if (branch === null) return null;
    if (!isRecord(branch) || !validCommandId(branch.commandId) ||
        (branch.handler !== 'backend' && branch.handler !== 'ui' && branch.handler !== 'contextual' && branch.handler !== 'local_ui') ||
        !isCommandKeyboardTrigger(branch.shortcut) ||
        contextualKey(branch.shortcut) !== key ||
        serializeCommandKeyboardTrigger(branch.shortcut) !== serializeCommandKeyboardTrigger(raw.shortcut as CommandKeyboardTrigger) ||
        (isLocalUICommand(branch.commandId) && branch.handler !== 'local_ui') ||
        (branch.handler === 'local_ui' && !isLocalUICommand(branch.commandId))) return undefined;
    return { commandId: branch.commandId, handler: branch.handler, shortcut: cloneShortcut(branch.shortcut) };
  };
  for (const [surface, branch] of Object.entries(raw.bySurface)) {
    if (!safeContextKey(surface)) return null;
    if (workspaceOnly && !WORKSPACE_SURFACE_TYPES.has(surface)) return null;
    const validated = validateBranch(branch);
    if (validated === undefined) return null;
    bySurface[surface] = validated;
  }
  if (Object.keys(bySurface).length === 0) return null;
  if (!allowProfiles && raw.byProfile !== undefined) return null;
  if (allowProfiles && raw.byProfile !== undefined) {
    if (!isRecord(raw.byProfile) || Object.keys(raw.byProfile).length === 0 ||
        Object.keys(bySurface).some((surface) => !WORKSPACE_SURFACE_TYPES.has(surface))) return null;
  }
  if (raw.bySurfaceId !== undefined) {
    if (!isRecord(raw.bySurfaceId)) return null;
    for (const [surfaceType, surfaceIds] of Object.entries(raw.bySurfaceId)) {
      if (!safeContextKey(surfaceType) || !isRecord(surfaceIds)) return null;
      const validatedIds: Record<string, LocalCommandKeyboardBinding | null> = Object.create(null);
      for (const [surfaceId, branch] of Object.entries(surfaceIds)) {
        if (!safeContextKey(surfaceId)) return null;
        const validated = validateBranch(branch);
        if (validated === undefined) return null;
        validatedIds[surfaceId] = validated;
      }
      bySurfaceId![surfaceType] = validatedIds;
    }
  }
  const fallback = validateBranch(raw.fallback);
  if (fallback === undefined || (fallback && raw.fallbackToSequences)) return null;
  const sequenceFallbacks: Record<string, Record<string, true>> | undefined =
    raw.sequenceFallbacks === undefined ? undefined : Object.create(null);
  if (raw.sequenceFallbacks !== undefined) {
    if (!isRecord(raw.sequenceFallbacks)) return null;
    for (const [surfaceType, surfaceIds] of Object.entries(raw.sequenceFallbacks)) {
      if (!safeContextKey(surfaceType) || !Object.prototype.hasOwnProperty.call(bySurface, surfaceType) ||
          !isRecord(surfaceIds) || Object.keys(surfaceIds).length === 0) return null;
      const idFlags: Record<string, true> = Object.create(null);
      const idBranches = bySurfaceId?.[surfaceType];
      for (const [surfaceId, flag] of Object.entries(surfaceIds)) {
        if ((surfaceId !== '' && !safeContextKey(surfaceId)) || flag !== true ||
            (surfaceId === '' && bySurface[surfaceType] !== null) ||
            (surfaceId !== '' && (!idBranches || !Object.prototype.hasOwnProperty.call(idBranches, surfaceId) || idBranches[surfaceId] !== null))) return null;
        idFlags[surfaceId] = true;
      }
      sequenceFallbacks![surfaceType] = idFlags;
    }
  }
  const validated: LocalCommandContextualBinding = {
    shortcut: raw.shortcut.version === 1 ? normalizeCommandShortcut(raw.shortcut) : normalizeCommandShortcutSequence(raw.shortcut),
    bySurface, fallback, fallbackToSequences: raw.fallbackToSequences,
  };
  if (bySurfaceId) validated.bySurfaceId = bySurfaceId;
  if (sequenceFallbacks) validated.sequenceFallbacks = sequenceFallbacks;
  if (allowProfiles && raw.byProfile !== undefined) {
    const byProfile: Record<string, LocalCommandContextualBinding> = Object.create(null);
    for (const [profile, projection] of Object.entries(raw.byProfile as Record<string, unknown>)) {
      if (!safeContextKey(profile) || !isRecord(projection) ||
          !isCommandKeyboardTrigger(projection.shortcut) ||
          serializeCommandKeyboardTrigger(projection.shortcut) !== serializeCommandKeyboardTrigger(raw.shortcut)) return null;
      const leaf = validateContextualEntry(projection, false, true);
      if (!leaf) return null;
      byProfile[profile] = leaf;
    }
    validated.byProfile = byProfile;
  }
  return validated;
}

function validateMap(map: LocalCommandKeyboardMap): ValidatedBindings | null {
  if (!isRecord(map) || typeof map.generation !== 'string' || map.generation.length === 0 || !Array.isArray(map.bindings)) return null;
  const simple = new Map<string, LocalCommandKeyboardBinding>();
  const sequences = new Map<string, LocalCommandKeyboardBinding[]>();
  for (const entry of map.bindings) {
    if (!entry || typeof entry !== 'object' || !isCommandKeyboardTrigger(entry.shortcut) || !validCommandId(entry.commandId) ||
        (entry.handler !== 'backend' && entry.handler !== 'ui' && entry.handler !== 'contextual' && entry.handler !== 'local_ui')) return null;
    if (isLocalUICommand(entry.commandId) && entry.handler !== 'local_ui') return null;
    const shortcut = entry.shortcut.version === 1 ? normalizeCommandShortcut(entry.shortcut) : normalizeCommandShortcutSequence(entry.shortcut);
    if (shortcut.version === 1) {
      if (!hasCommandModifier(shortcut) && !isBareWhitelistedFunctionKey(shortcut)) return null;
      const key = serializeCommandShortcut(shortcut);
      const existing = simple.get(key);
      if (existing && (existing.commandId !== entry.commandId || existing.handler !== entry.handler)) return null;
      simple.set(key, { shortcut: cloneShortcut(shortcut), commandId: entry.commandId, handler: entry.handler });
    } else {
      const key = stepKey(shortcut.steps[0]);
      const list = sequences.get(key) ?? [];
      const sequenceKey = serializeCommandKeyboardTrigger(shortcut);
      if (list.some((existing) => serializeCommandKeyboardTrigger(existing.shortcut) === sequenceKey &&
          (existing.commandId !== entry.commandId || existing.handler !== entry.handler))) return null;
      list.push({ shortcut: cloneShortcut(shortcut), commandId: entry.commandId, handler: entry.handler });
      sequences.set(key, list);
    }
  }
  const contextual = new Map<string, LocalCommandContextualBinding>();
  const contextualSequences = new Map<string, LocalCommandContextualBinding[]>();
  if (map.contextualBindings !== undefined && !Array.isArray(map.contextualBindings)) return null;
  for (const entry of map.contextualBindings ?? []) {
    const validatedEntry = validateContextualEntry(entry, true);
    if (!validatedEntry) return null;
    const key = contextualKey(validatedEntry.shortcut);
    if (validatedEntry.shortcut.version === 1 && (contextual.has(key) || simple.has(key))) return null;
    if (validatedEntry.shortcut.version === 1) contextual.set(key, validatedEntry);
    else {
      const list = contextualSequences.get(key) ?? [];
      const serialized = serializeCommandKeyboardTrigger(validatedEntry.shortcut);
      if (list.some(existing => serializeCommandKeyboardTrigger(existing.shortcut) === serialized)) return null;
      list.push(validatedEntry);
      contextualSequences.set(key, list);
    }
  }
  for (const [key, entries] of contextualSequences) {
    const flat = sequences.get(key) ?? [];
    if (entries.some(entry => flat.some(binding =>
      serializeCommandKeyboardTrigger(binding.shortcut) === serializeCommandKeyboardTrigger(entry.shortcut)))) return null;
  }
  return { simple, sequences, contextual, contextualSequences };
}

export function createLocalCommandKeyboard(options: LocalCommandKeyboardOptions): LocalCommandKeyboardController {
  let disposed = false;
  let refreshId = 0;
  let generation: string | null = null;
  let validUntil = 0;
  let expiryTimer: ReturnType<typeof setTimeout> | undefined;
  let bindings: ValidatedBindings = { simple: new Map(), sequences: new Map(), contextual: new Map(), contextualSequences: new Map() };
  const pressed = new Map<string, Press>();
  const modifiers = createCommandModifierState();
  let pendingSequence: {
    bindings: LocalCommandKeyboardBinding[];
    finalByCode: Map<string, LocalCommandKeyboardBinding[]>;
    timer: ReturnType<typeof setTimeout>;
    surfaceType?: string;
    context?: LocalCommandKeyContext;
    contextLease?: LocalCommandContextLease;
    contextualTriggers?: Set<string>;
    globallyOwned: boolean;
  } | null = null;
  let resetTail = Promise.resolve();

  const safeCall = (callback: () => Promise<void>): Promise<void> => {
    try {
      return Promise.resolve(callback()).catch(() => undefined);
    } catch {
      // Callbacks are an integration boundary; one failure must not break DOM capture.
      return Promise.resolve();
    }
  };

  const swallow = (callback: () => Promise<void>): void => {
    void safeCall(callback);
  };

  const queueReset = (value: string | null): Promise<void> => {
    if (value === null) return resetTail;
    resetTail = resetTail.then(() => safeCall(() => options.reset(value)));
    return resetTail;
  };

  const clearState = (value: string | null, queueBackendReset: boolean): Promise<void> => {
    clearTimeout(expiryTimer);
    expiryTimer = undefined;
    validUntil = 0;
    cancelSequence('refresh');
    pressed.clear();
    modifiers.clear();
    bindings = { simple: new Map(), sequences: new Map(), contextual: new Map(), contextualSequences: new Map() };
    generation = null;
    options.onMapInvalidated?.();
    return queueBackendReset ? queueReset(value) : resetTail;
  };

  const cancelSequence = (reason: CommandSequenceCancelReason): void => {
    if (!pendingSequence) return;
    clearTimeout(pendingSequence.timer);
    pendingSequence = null;
    try {
      options.onSequenceCancelled?.(reason);
    } catch {
      // Optional host notification must not break keyboard capture.
    }
  };

  const canHandleBinding = (binding: LocalCommandKeyboardBinding, event: KeyboardEvent): boolean => {
    if (isEditableTarget(event.target)) {
      try {
        if (!options.canHandleEditable?.(binding.commandId, event)) return false;
      } catch {
        return false;
      }
    }
    try {
      return !options.canHandle || options.canHandle(binding.commandId, event);
    } catch {
      return false;
    }
  };

  const readSurface = (event: KeyboardEvent): string | undefined => {
    try {
      const value = options.readSurfaceType?.(event);
      return typeof value === 'string' && value.length > 0 && value.trim() === value ? value : undefined;
    } catch {
      return undefined;
    }
  };

  const readLease = (event: KeyboardEvent): LocalCommandContextLease | undefined => {
    try {
      const lease = options.readContext?.(event);
      if (!lease || typeof lease.surfaceId !== 'string' || !lease.surfaceId || lease.surfaceId.trim() !== lease.surfaceId ||
          typeof lease.surfaceType !== 'string' || !lease.surfaceType || lease.surfaceType.trim() !== lease.surfaceType ||
          (lease.profile !== undefined && (typeof lease.profile !== 'string' || !lease.profile || lease.profile.trim() !== lease.profile)) ||
          (lease.allowedCommandIds !== undefined && (!Array.isArray(lease.allowedCommandIds) || lease.allowedCommandIds.length === 0 ||
            lease.allowedCommandIds.some(commandID => !validCommandId(commandID)))) ||
          typeof lease.isCurrent !== 'function') return undefined;
      return lease;
    } catch {
      return undefined;
    }
  };

  const leaseIsCurrent = (lease: LocalCommandContextLease): boolean => {
    try {
      return lease.isCurrent();
    } catch {
      return false;
    }
  };

  const resolveContextual = (
    entry: LocalCommandContextualBinding,
    surfaceType: string,
    lease: LocalCommandContextLease | undefined,
  ): LocalCommandContextualResolution => {
    const resolution = resolveLocalCommandContextualBinding(entry, surfaceType, lease);
    if (resolution.requiresLease && (!lease || lease.surfaceType !== surfaceType || !leaseIsCurrent(lease))) {
      return { matched: false, branch: null, barrier: true, requiresLease: true };
    }
    return resolution;
  };

  const startSequence = (
    candidates: LocalCommandKeyboardBinding[],
    event: KeyboardEvent,
    sequenceContext: { surfaceType?: string; lease?: LocalCommandContextLease; contextualBindings?: readonly LocalCommandKeyboardBinding[]; globallyOwned: boolean } = { globallyOwned: false },
  ): void => {
    const finalByCode = new Map<string, LocalCommandKeyboardBinding[]>();
    for (const binding of candidates) {
      if (binding.shortcut.version !== 2) continue;
      const finalCode = binding.shortcut.steps[1].code;
      const list = finalByCode.get(finalCode) ?? [];
      list.push(binding);
      finalByCode.set(finalCode, list);
    }
    if (finalByCode.size === 0) return;
    const timeoutMs = options.sequenceTimeoutMs ?? 1500;
    const timer = setTimeout(() => cancelSequence('timeout'), timeoutMs);
    pendingSequence = {
      bindings: candidates,
      finalByCode,
      timer,
      surfaceType: sequenceContext.surfaceType,
      context: sequenceContext.lease && {
        surfaceId: sequenceContext.lease.surfaceId,
        surfaceType: sequenceContext.lease.surfaceType,
        ...(sequenceContext.lease.profile !== undefined ? { profile: sequenceContext.lease.profile } : {}),
      },
      contextLease: sequenceContext.lease,
      contextualTriggers: sequenceContext.contextualBindings && new Set(sequenceContext.contextualBindings.map(binding => serializeCommandKeyboardTrigger(binding.shortcut))),
      globallyOwned: sequenceContext.globallyOwned,
    };
    try {
      const started = options.onSequenceStarted?.(candidates.map((binding) => ({
        shortcut: cloneShortcut(binding.shortcut), commandId: binding.commandId, handler: binding.handler,
      })));
      if (started) void Promise.resolve(started).catch(() => cancelSequence('unexpected'));
    } catch {
      cancelSequence('unexpected');
      return;
    }
    observeCommandShortcutComposition(event);
    event.preventDefault();
    event.stopImmediatePropagation();
  };

  const onKeyDown = (event: KeyboardEvent): void => {
    if (disposed) return;
    if (validUntil > 0 && Date.now() >= validUntil) {
      void refresh();
      return;
    }
    const globallyOwned = Boolean(options.ownedGlobally?.(event));
    if (globallyOwned) {
      cancelSequence('blocked');
      pressed.delete(event.code);
      event.preventDefault();
      event.stopImmediatePropagation();
      return;
    }
    modifiers.observe(event);
    if (event.defaultPrevented) {
      if (pendingSequence) cancelSequence('blocked');
      return;
    }
    if (pendingSequence) {
      if (event.repeat) {
        // A repeated prefix/final key remains owned by the pending chord;
        // consume it without restarting the timeout or invoking the command.
        event.preventDefault();
        event.stopImmediatePropagation();
        return;
      }
      if (options.blocked()) {
        cancelSequence('blocked');
        return;
      }
      if (event.isComposing || event.keyCode === 229) {
        cancelSequence('ime');
        return;
      }
      if (event.getModifierState('AltGraph')) {
        cancelSequence('altgraph');
        return;
      }
      if (event.code === 'Escape') {
        cancelSequence('escape');
        event.preventDefault();
        event.stopImmediatePropagation();
        return;
      }
      const sequenceLease = pendingSequence.contextLease;
      if (pendingSequence.globallyOwned !== Boolean(options.ownedGlobally?.(event)) ||
          (sequenceLease && (!leaseIsCurrent(sequenceLease) || (() => {
            const currentLease = readLease(event);
            return !currentLease || !sameContext(sequenceLease, currentLease) ||
              (readSurface(event) !== undefined && readSurface(event) !== currentLease.surfaceType);
          })())) ||
          (!sequenceLease && pendingSequence.surfaceType !== undefined && readSurface(event) !== pendingSequence.surfaceType)) {
        cancelSequence('unexpected');
        return;
      }
      const finalShortcut = shortcutForEvent(event, modifiers.allowsControlAlt());
      const finalCandidates = finalShortcut && finalShortcut.modifiers.length === 0
        ? pendingSequence.finalByCode.get(finalShortcut.code) ?? []
        : [];
      if (finalCandidates.length > 0) {
        const accepted = finalCandidates.filter((candidate) => canHandleBinding(candidate, event));
        const unique = accepted.filter((candidate, index, all) => all.findIndex(other =>
          other.commandId === candidate.commandId && other.handler === candidate.handler &&
          serializeCommandKeyboardTrigger(other.shortcut) === serializeCommandKeyboardTrigger(candidate.shortcut)) === index);
        const mapped = unique.length === 1 ? unique[0] : null;
        if (!mapped || generation === null || pressed.has(event.code)) {
          cancelSequence('unexpected');
          return;
        }
        const completedSequence = pendingSequence;
        cancelPendingWithoutNotification();
        observeCommandShortcutComposition(event);
        event.preventDefault();
        event.stopImmediatePropagation();
        const request: LocalCommandKeyRequest = {
          generation,
          shortcut: cloneShortcut(mapped.shortcut),
          commandId: mapped.commandId,
          handler: mapped.handler,
          kind: 'down',
          repeat: false,
          context: completedSequence.contextualTriggers?.has(serializeCommandKeyboardTrigger(mapped.shortcut))
            ? completedSequence.context : undefined,
        };
        const downPromise = safeCall(() => options.onDown(request));
        pressed.set(event.code, {
          generation, shortcut: cloneShortcut(mapped.shortcut), commandId: mapped.commandId, handler: mapped.handler, downPromise,
          surfaceType: completedSequence.surfaceType,
          context: completedSequence.contextualTriggers?.has(serializeCommandKeyboardTrigger(mapped.shortcut))
            ? completedSequence.context : undefined,
        });
        return;
      }
      if (finalShortcut && finalShortcut.modifiers.length === 0 && MENU_NAVIGATION_CODES.has(finalShortcut.code)) {
        cancelSequence('menu-navigation');
        // The menu owns this unmodified key; leave it untouched for its host.
        return;
      }
      cancelSequence('unexpected');
      // The unexpected key is deliberately processed as a standalone key below.
    }
    const shortcut = shortcutForEvent(event, modifiers.allowsControlAlt());
    if (!shortcut) return;
    const key = serializeCommandShortcut(shortcut);
    const contextual = bindings.contextual.get(key);
    const contextualSequenceEntries = bindings.contextualSequences.get(key) ?? [];
    let mapped = contextual ? null : bindings.simple.get(key);
    let surfaceType: string | undefined;
    let contextLease: LocalCommandContextLease | undefined;
    if (contextual || (!mapped && contextualSequenceEntries.length > 0)) {
      surfaceType = readSurface(event);
      contextLease = readLease(event);
      if (surfaceType === undefined) surfaceType = contextLease?.surfaceType;
    }
    let contextualNoMatch = false;
    let contextualBarrier = false;
    let contextualLeaseRequired = false;
    const contextualSequenceCandidates: LocalCommandKeyboardBinding[] = [];
    if (contextual) {
      if (!surfaceType) contextualBarrier = true;
      else {
        const resolution = resolveContextual(contextual, surfaceType, contextLease);
        contextualLeaseRequired = resolution.requiresLease;
        if (resolution.matched) {
          mapped = resolution.branch;
          if (resolution.barrier) contextualBarrier = true;
        } else if (resolution.barrier) {
          contextualBarrier = true;
        } else {
          contextualNoMatch = true;
        }
      }
    }
    if (!mapped && !contextualBarrier && contextualSequenceEntries.length > 0) {
      if (!surfaceType) contextualBarrier = true;
      else {
        for (const entry of contextualSequenceEntries) {
          const resolution = resolveContextual(entry, surfaceType, contextLease);
          contextualLeaseRequired ||= resolution.requiresLease;
          // A null branch suppresses only this v2 candidate. It must not
          // suppress another contextual sequence sharing the same prefix.
          if (resolution.barrier && !(resolution.matched && resolution.branch === null)) {
            contextualBarrier = true;
            continue;
          }
          if (resolution.branch) contextualSequenceCandidates.push(resolution.branch);
        }
      }
    }
    if (options.blocked(mapped?.commandId, event)) return;
    let contextualLeaseInvalid = false;
    const selectedContextualBindings = contextual ? (mapped ? [mapped] : contextualSequenceCandidates) : contextualSequenceCandidates;
    if (contextualLeaseRequired && (!contextLease || !surfaceType || contextLease.surfaceType !== surfaceType || !leaseIsCurrent(contextLease))) {
      contextualLeaseInvalid = true;
    }
    if (selectedContextualBindings.length > 0) {
      const contextSurfaceType = contextLease?.surfaceType;
      if (contextLease && surfaceType !== undefined && contextSurfaceType !== surfaceType) contextualLeaseInvalid = true;
      if (selectedContextualBindings.some(binding => binding.handler !== 'local_ui') && !contextLease) contextualLeaseInvalid = true;
      if (contextLease && !leaseIsCurrent(contextLease)) contextualLeaseInvalid = true;
    }
    const flatSequenceCandidates = bindings.sequences.get(key) ?? [];
    const allSequenceCandidates = [...contextualSequenceCandidates, ...flatSequenceCandidates];
    const allowedCommandIds = contextLease?.allowedCommandIds;
    const candidateCommands = [...(mapped ? [mapped] : []), ...allSequenceCandidates];
    const contextCommandDenied = allowedCommandIds !== undefined && candidateCommands.some(
      (candidate) => !allowedCommandIds.includes(candidate.commandId),
    );
    const sequenceFallback = contextualNoMatch && !contextualBarrier && !!surfaceType && flatSequenceCandidates.length > 0;
    if (contextualLeaseInvalid || contextCommandDenied || contextualBarrier ||
        (contextual && !mapped && contextualSequenceCandidates.length === 0 && contextualSequenceEntries.length === 0 && !sequenceFallback) ||
        (contextualSequenceEntries.length > 0 && !mapped && allSequenceCandidates.length === 0)) {
      event.preventDefault();
      event.stopImmediatePropagation();
      return;
    }
    if (!mapped && allSequenceCandidates.length > 0 && generation !== null) {
      if (event.repeat) {
        event.preventDefault();
        event.stopImmediatePropagation();
        return;
      }
      const acceptedSequenceCandidates = allSequenceCandidates.filter(candidate => canHandleBinding(candidate, event));
      if (acceptedSequenceCandidates.length === 0) return;
      startSequence(acceptedSequenceCandidates, event, {
        surfaceType: contextualSequenceCandidates.length > 0 ? surfaceType : undefined,
        lease: contextualSequenceCandidates.length > 0 ? contextLease : undefined,
        contextualBindings: contextualSequenceCandidates,
        globallyOwned,
      });
      return;
    }
    if (mapped && generation !== null) {
      if (!canHandleBinding(mapped, event)) return;

      observeCommandShortcutComposition(event);
      event.preventDefault();
      event.stopImmediatePropagation();
      if (event.repeat) {
        const press = pressed.get(event.code);
        if (!options.canRepeat?.(mapped.commandId) || !press ||
            press.surfaceType !== surfaceType ||
            press.generation !== generation ||
            serializeCommandKeyboardTrigger(press.shortcut) !== key ||
            press.commandId !== mapped.commandId || press.handler !== mapped.handler) return;
          if ((contextLease || press.context) && (!contextLease || !press.context || !sameContext(press.context, contextLease))) return;
      } else if (pressed.has(event.code)) return;

      const request: LocalCommandKeyRequest = {
        generation, shortcut: cloneShortcut(mapped.shortcut), commandId: mapped.commandId, handler: mapped.handler,
        kind: 'down', repeat: event.repeat,
        context: contextLease && {
          surfaceId: contextLease.surfaceId,
          surfaceType: contextLease.surfaceType,
          ...(contextLease.profile !== undefined ? { profile: contextLease.profile } : {}),
        },
      };
      const downPromise = safeCall(() => options.onDown(request));
      pressed.set(event.code, {
        generation, shortcut: cloneShortcut(mapped.shortcut), commandId: mapped.commandId, handler: mapped.handler, downPromise,
        surfaceType, context: contextLease && {
          surfaceId: contextLease.surfaceId,
          surfaceType: contextLease.surfaceType,
          ...(contextLease.profile !== undefined ? { profile: contextLease.profile } : {}),
        },
      });
      return;
    }
  };

  const onKeyUp = (event: KeyboardEvent): void => {
    if (disposed) return;
    if (options.ownedGlobally?.(event)) {
      pressed.delete(event.code);
      event.preventDefault();
      event.stopImmediatePropagation();
      return;
    }
    modifiers.observe(event);
    const press = pressed.get(event.code);
    if (!press) return;
    pressed.delete(event.code);
    const release = () => options.onUp({
      generation: press.generation, shortcut: cloneShortcut(press.shortcut), commandId: press.commandId,
      handler: press.handler, kind: 'up', repeat: false, context: press.context,
    });
    if (press.handler === 'local_ui') swallow(release);
    else void press.downPromise.then(() => safeCall(release));
  };

  const onBlur = (event: Event): void => {
    // Window blur is wrapped differently by some DOM shims. A child blur always
    // has a Node target; only the Window-level target (or a null shim target) is valid.
    if (!disposed && (event.target === options.target || event.target === null || !(event.target instanceof Node))) {
      ++refreshId;
      const oldGeneration = generation;
      cancelSequence('blur');
      void clearState(oldGeneration, true);
    }
  };

  options.target.addEventListener('keydown', onKeyDown, true);
  options.target.addEventListener('keyup', onKeyUp, true);
  options.target.addEventListener('blur', onBlur, true);

  const refresh = async (): Promise<void> => {
    if (disposed) return;
    const id = ++refreshId;
    const oldGeneration = generation;
    const resetBeforeLoad = clearState(oldGeneration, true);
    try {
      await resetBeforeLoad;
      if (disposed || id !== refreshId) return;
      const map = await options.loadMap();
      if (disposed || id !== refreshId) return;
      const next = validateMap(map);
      if (!next || (map.validUntil !== undefined && (!Number.isSafeInteger(map.validUntil) || map.validUntil <= Date.now())) || (options.acceptMap && !options.acceptMap(map))) {
        options.onMapInvalidated?.();
        return;
      }
      generation = map.generation;
      validUntil = map.validUntil ?? 0;
      bindings = next;
      options.onMapAccepted?.(map);
      if (validUntil > 0) expiryTimer = setTimeout(() => { void refresh(); }, Math.min(2_147_483_647, Math.max(0, validUntil - Date.now())));
    } catch {
      // A failed refresh deliberately leaves the empty, fail-closed map in place.
    }
  };

  const dispose = (): void => {
    if (disposed) return;
    const oldGeneration = generation;
    void queueReset(oldGeneration);
    disposed = true;
    clearTimeout(expiryTimer);
    ++refreshId;
    options.target.removeEventListener('keydown', onKeyDown, true);
    options.target.removeEventListener('keyup', onKeyUp, true);
    options.target.removeEventListener('blur', onBlur, true);
    modifiers.clear();
    pressed.clear();
    cancelSequence('dispose');
    bindings = { simple: new Map(), sequences: new Map(), contextual: new Map(), contextualSequences: new Map() };
    generation = null;
    options.onMapInvalidated?.();
  };

  function cancelPendingWithoutNotification(): void {
    if (!pendingSequence) return;
    clearTimeout(pendingSequence.timer);
    pendingSequence = null;
  }

  return { refresh, cancelSequence: () => cancelSequence('unexpected'), dispose };
}
