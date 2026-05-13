import { isVoiceAccessibilityOriginCurrentlyActive } from './announcerBroker';
import type { VoiceAccessibilityOrigin } from './types';

interface STTSession {
  origin?: VoiceAccessibilityOrigin;
  cancel: () => void;
}

interface STTStartRequest {
  origin?: VoiceAccessibilityOrigin;
  cancel: () => void;
}

let activeSession: STTSession | null = null;

function isSameOrigin(a?: VoiceAccessibilityOrigin, b?: VoiceAccessibilityOrigin): boolean {
  if (!a || !b) return a === b;
  return Boolean(
    (a.tabId && a.tabId === b.tabId)
    || (a.surfaceId && a.surfaceId === b.surfaceId)
    || (a.sessionKey && a.sessionKey === b.sessionKey),
  );
}

export function canStartSTT(origin?: VoiceAccessibilityOrigin): boolean {
  return isVoiceAccessibilityOriginCurrentlyActive(origin);
}

export function requestSTTStart(request: STTStartRequest): boolean {
  if (!canStartSTT(request.origin)) return false;

  if (activeSession && !isSameOrigin(activeSession.origin, request.origin)) {
    activeSession.cancel();
  }

  activeSession = request;
  return true;
}

export function finishSTTSession(origin?: VoiceAccessibilityOrigin) {
  if (!activeSession || !isSameOrigin(activeSession.origin, origin)) return;
  activeSession = null;
}

export function cancelInactiveSTTSession() {
  if (!activeSession || canStartSTT(activeSession.origin)) return;

  const session = activeSession;
  activeSession = null;
  session.cancel();
}

export function resetSTTGateForTests() {
  activeSession = null;
}
