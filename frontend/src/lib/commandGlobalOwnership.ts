const UUID_PATTERN = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i;

const FRAME_KEYS = new Set(['version', 'instanceId', 'revision', 'platform', 'combinations']);
const COMBINATION_KEYS = new Set(['key', 'modifiers']);

export const WINDOWS_GLOBAL_MODIFIER = {
  alt: 1,
  ctrl: 2,
  shift: 4,
  meta: 8,
} as const;

export interface CommandGlobalOwnershipCombination {
  key: number;
  modifiers: number;
}

export interface CommandGlobalOwnershipFrame {
  version: 1;
  instanceId: string;
  revision: number;
  platform: 'windows';
  combinations: CommandGlobalOwnershipCombination[];
}

export interface CommandGlobalOwnershipAck {
  instanceId: string;
  revision: number;
}

export interface CommandGlobalOwnershipOptions {
  ack: (frame: CommandGlobalOwnershipAck) => void;
  onChange?: () => void;
}

export interface CommandGlobalOwnershipController {
  applyFrame(raw: unknown): boolean;
  owns(event: KeyboardEvent): boolean;
  dispose(): void;
}

type ValidatedFrame = {
  instanceId: string;
  revision: number;
  combinations: Set<number>;
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function hasOnlyKeys(value: Record<string, unknown>, keys: Set<string>): boolean {
  return Object.keys(value).every((key) => keys.has(key));
}

function isUUID(value: unknown): value is string {
  return typeof value === 'string' && UUID_PATTERN.test(value);
}

function isSafePositiveInteger(value: unknown): value is number {
  return typeof value === 'number' && Number.isSafeInteger(value) && value > 0;
}

function isIntegerInRange(value: unknown, min: number, max: number): value is number {
  return typeof value === 'number' && Number.isInteger(value) && value >= min && value <= max;
}

function validateFrame(raw: unknown): ValidatedFrame | null {
  if (!isRecord(raw) || !hasOnlyKeys(raw, FRAME_KEYS) || raw.version !== 1 ||
      !isUUID(raw.instanceId) || !isSafePositiveInteger(raw.revision) || raw.platform !== 'windows' ||
      !Array.isArray(raw.combinations)) return null;

  const combinations = new Set<number>();
  for (const rawCombination of raw.combinations) {
    if (!isRecord(rawCombination) || !hasOnlyKeys(rawCombination, COMBINATION_KEYS) ||
        !isIntegerInRange(rawCombination.key, 1, 254) || !isIntegerInRange(rawCombination.modifiers, 0, 15)) {
      return null;
    }

    const key = rawCombination.key;
    const modifiers = rawCombination.modifiers;
    combinations.add(combinationId(key, modifiers));
  }

  return { instanceId: raw.instanceId, revision: raw.revision, combinations };
}

function combinationId(key: number, modifiers: number): number {
  return key * 16 + modifiers;
}

function sameCombinations(left: Set<number>, right: Set<number>): boolean {
  if (left.size !== right.size) return false;
  for (const combination of left) {
    if (!right.has(combination)) return false;
  }
  return true;
}

function eventModifiers(event: KeyboardEvent): number {
  return (event.altKey ? WINDOWS_GLOBAL_MODIFIER.alt : 0) |
    (event.ctrlKey ? WINDOWS_GLOBAL_MODIFIER.ctrl : 0) |
    (event.shiftKey ? WINDOWS_GLOBAL_MODIFIER.shift : 0) |
    (event.metaKey ? WINDOWS_GLOBAL_MODIFIER.meta : 0);
}

/**
 * Mantém o ownership global publicado pelo adapter Windows/WebView2 no espaço
 * de virtual-key. A instância continua fixada até `dispose`, evitando aceitar
 * replay de uma instância antiga.
 */
export function createCommandGlobalOwnership(options: CommandGlobalOwnershipOptions): CommandGlobalOwnershipController {
  if (typeof options?.ack !== 'function') throw new TypeError('ack callback is required');

  let disposed = false;
  let instanceId: string | null = null;
  let revision: number | null = null;
  let combinations = new Set<number>();

  const notifyChange = (): void => {
    try {
      options.onChange?.();
    } catch {
      // A consumer callback must not break synchronous ownership updates.
    }
  };

  const acknowledge = (frame: CommandGlobalOwnershipAck): void => {
    try {
      options.ack({ instanceId: frame.instanceId, revision: frame.revision });
    } catch {
      // ACK delivery is an integration boundary; the local snapshot is valid.
    }
  };

  return {
    applyFrame(raw: unknown): boolean {
      if (disposed) return false;
      const frame = validateFrame(raw);
      if (!frame) return false;
      if (instanceId !== null && frame.instanceId !== instanceId) return false;
      if (revision !== null && frame.revision < revision) return false;
      if (revision !== null && frame.revision === revision && !sameCombinations(combinations, frame.combinations)) return false;

      const changedRevision = revision === null || frame.revision > revision;
      instanceId = frame.instanceId;
      revision = frame.revision;
      if (changedRevision) {
        combinations = new Set(frame.combinations);
        notifyChange();
      }
      acknowledge({ instanceId: frame.instanceId, revision: frame.revision });
      return true;
    },

    owns(event: KeyboardEvent): boolean {
      if (disposed || (event.type !== 'keydown' && event.type !== 'keyup') || event.isComposing ||
          !Number.isInteger(event.keyCode) || event.keyCode < 1 || event.keyCode > 254 || event.keyCode === 229) return false;
      return combinations.has(combinationId(event.keyCode, eventModifiers(event)));
    },

    dispose(): void {
      if (disposed) return;
      disposed = true;
      combinations.clear();
    },
  };
}
