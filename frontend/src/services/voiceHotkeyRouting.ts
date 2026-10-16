import { getModalRegistrySnapshot } from '../lib/modalRegistry';

export interface VoiceHotkeyEvent {
  triggerType: string;
  bringToFront: boolean;
}

export interface VoiceHotkeyRecipient {
  /** Perfil efetivo que esta instância realmente carregou. */
  getProfileSlug: () => string | null;
  /** Monotonic profile-load generation; changes on every profile reload. */
  getProfileGeneration: () => number;
  isEligible: () => boolean;
  deliver: (event: VoiceHotkeyEvent) => void;
}

export interface VoiceHotkeyTarget {
  isCurrent(): boolean;
  execute(): boolean;
  dispose(): void;
}

const recipients = new Map<symbol, VoiceHotkeyRecipient>();

export function parseVoiceHotkeyEvent(data: unknown): VoiceHotkeyEvent | null {
  if (!data || typeof data !== 'object') return null;

  const event = data as Record<string, unknown>;
  if (typeof event.triggerType !== 'string' || event.triggerType.trim() === '') return null;
  if (typeof event.bringToFront !== 'boolean') return null;

  return {
    triggerType: event.triggerType,
    bringToFront: event.bringToFront,
  };
}

function readProfileSlug(recipient: VoiceHotkeyRecipient): string | null {
  try {
    const slug = recipient.getProfileSlug();
    return typeof slug === 'string' && slug.length > 0 ? slug : null;
  } catch {
    return null;
  }
}

function readProfileGeneration(recipient: VoiceHotkeyRecipient): number | null {
  try {
    const generation = recipient.getProfileGeneration();
    return Number.isSafeInteger(generation) && generation >= 0 ? generation : null;
  } catch {
    return null;
  }
}

function eligibleRecipientForProfile(profileSlug: string): [symbol, VoiceHotkeyRecipient] | undefined {
  const matches: Array<[symbol, VoiceHotkeyRecipient]> = [];
  for (const entry of recipients.entries()) {
    const [, recipient] = entry;
    if (readProfileSlug(recipient) !== profileSlug) continue;
    try {
      if (recipient.isEligible()) matches.push(entry);
    } catch {
      // Eligibility is a trust boundary: a broken surface cannot receive voice.
    }
  }
  return matches.length === 1 ? matches[0] : undefined;
}

/**
 * Captura uma única superfície de voz para uma reserva global já admitida.
 * O slug e o evento só restringem o alvo; não concedem autoridade adicional.
 * O lease nunca procura outro receptor depois de capturado.
 */
export function captureVoiceHotkeyTarget(
  profileSlug: string,
  event: VoiceHotkeyEvent,
): VoiceHotkeyTarget | undefined {
  if (typeof profileSlug !== 'string' || profileSlug.length === 0) return undefined;
  const parsedEvent = parseVoiceHotkeyEvent(event);
  if (!parsedEvent) return undefined;

  let capturedModalGeneration: string;
  try {
    const modal = getModalRegistrySnapshot();
    if (modal.topID !== null) return undefined;
    capturedModalGeneration = modal.generation;
  } catch {
    return undefined;
  }

  const captured = eligibleRecipientForProfile(profileSlug);
  if (!captured) return undefined;
  const [token, recipient] = captured;
  const capturedProfileGeneration = readProfileGeneration(recipient);
  if (capturedProfileGeneration === null) return undefined;
  let disposed = false;
  let invalid = false;
  let executed = false;

  const isCurrent = (): boolean => {
    if (disposed || invalid || recipients.get(token) !== recipient) return false;
    try {
      const modal = getModalRegistrySnapshot();
      const currentRecipient = eligibleRecipientForProfile(profileSlug);
      const current = modal.topID === null && modal.generation === capturedModalGeneration &&
        currentRecipient?.[0] === token && currentRecipient?.[1] === recipient &&
        readProfileGeneration(recipient) === capturedProfileGeneration;
      if (!current) invalid = true;
      return current;
    } catch {
      invalid = true;
      return false;
    }
  };

  const target: VoiceHotkeyTarget = {
    isCurrent,
    execute(): boolean {
      if (executed || !isCurrent()) return false;
      // Mark before delivery so a throwing or re-entrant callback is still one-shot.
      executed = true;
      recipient.deliver(parsedEvent);
      return true;
    },
    dispose(): void {
      disposed = true;
      invalid = true;
    },
  };

  // Close the synchronous capture window as well: if eligibility, modal state,
  // or the profile generation changed while selecting, do not return a lease.
  if (!target.isCurrent()) {
    target.dispose();
    return undefined;
  }
  return target;
}

export function registerVoiceHotkeyRecipient(recipient: VoiceHotkeyRecipient): () => void {
  const token = Symbol('voice-hotkey-recipient');
  recipients.set(token, recipient);

  let registered = true;
  return () => {
    if (!registered) return;
    registered = false;
    recipients.delete(token);
  };
}
