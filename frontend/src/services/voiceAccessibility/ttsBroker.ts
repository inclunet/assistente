import { announceWithOrigin, isVoiceAccessibilityOriginCurrentlyActive } from './announcerBroker';
import type {
  VoiceAccessibilityOrigin,
  VoiceAccessibilityPriority,
  VoiceAccessibilityRequestBase,
} from './types';

export interface VoiceTTSRequest extends VoiceAccessibilityRequestBase {
  text?: string;
  interrupt?: boolean;
  stopCurrent?: () => void;
  speak: () => Promise<void | boolean>;
  inactiveAnnouncement?: string;
}

let currentTurn = 0;

function isInactiveAutomatic(priority: VoiceAccessibilityPriority | undefined, origin: VoiceAccessibilityOrigin | undefined): boolean {
  if (isVoiceAccessibilityOriginCurrentlyActive(origin)) return false;
  return !priority || priority === 'automatic-inactive' || priority === 'automatic-active';
}

export async function requestVoiceTTS(request: VoiceTTSRequest): Promise<boolean> {
  if (isInactiveAutomatic(request.priority, request.origin)) {
    const message = request.inactiveAnnouncement ?? request.text;
    if (message) {
      announceWithOrigin({
        message,
        origin: request.origin,
        eventType: request.eventType ?? 'completion',
      });
    }
    return false;
  }

  if (request.interrupt !== false) {
    request.stopCurrent?.();
  }

  const turn = ++currentTurn;
  const result = await request.speak();
  return turn === currentTurn && result !== false;
}

export function resetVoiceTTSBrokerForTests() {
  currentTurn = 0;
}
