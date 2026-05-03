import i18next from 'i18next';
import type {
  VoiceAccessibilityOrigin,
  VoiceAccessibilityRequestBase,
} from './types';
import { getVoiceAccessibilityOriginLabel } from './types';

export type AnnouncePriority = 'polite' | 'assertive';

export interface VoiceAnnounceRequest extends VoiceAccessibilityRequestBase {
  message: string;
  announcePriority?: AnnouncePriority;
}

type AnnounceSink = (message: string, priority: AnnouncePriority) => void;
type OriginActiveResolver = (origin?: VoiceAccessibilityOrigin) => boolean;

let announceSink: AnnounceSink | null = null;
let originActiveResolver: OriginActiveResolver = () => true;

const INACTIVE_ALLOWED_EVENTS = new Set(['completion', 'error', 'system']);

export function registerAnnouncerSink(sink: AnnounceSink) {
  announceSink = sink;
}

export function unregisterAnnouncerSink() {
  announceSink = null;
}

export function registerVoiceAccessibilityActiveResolver(resolver: OriginActiveResolver): () => void {
  originActiveResolver = resolver;
  return () => {
    if (originActiveResolver === resolver) {
      originActiveResolver = () => true;
    }
  };
}

export function isVoiceAccessibilityOriginCurrentlyActive(origin?: VoiceAccessibilityOrigin): boolean {
  return originActiveResolver(origin);
}

function formatInactiveAnnouncement(request: VoiceAnnounceRequest): string {
  const label = getVoiceAccessibilityOriginLabel(request.origin);
  if (!label) return request.message;

  return i18next.t('voiceAccessibility.inactiveAnnouncement', {
    title: label,
    message: request.message,
    defaultValue: '{{title}}: {{message}}',
  });
}

function shouldAnnounce(request: VoiceAnnounceRequest): boolean {
  if (!request.message.trim()) return false;
  if (originActiveResolver(request.origin)) return true;

  return INACTIVE_ALLOWED_EVENTS.has(request.eventType ?? 'progress');
}

export function announceWithOrigin(request: VoiceAnnounceRequest): boolean {
  if (!shouldAnnounce(request)) return false;

  const message = originActiveResolver(request.origin)
    ? request.message
    : formatInactiveAnnouncement(request);
  const priority = request.announcePriority ?? (request.eventType === 'error' ? 'assertive' : 'polite');

  if (announceSink) {
    announceSink(message, priority);
    return true;
  }

  queueMicrotask(() => {
    if (announceSink) {
      announceSink(message, priority);
    }
  });
  return true;
}
