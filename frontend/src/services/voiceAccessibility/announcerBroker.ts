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
  /**
   * Marca o anúncio como leitura do conteúdo do assistente. A live region é
   * única (AEP-0058): qualquer anúncio seguinte substitui o texto e o leitor de
   * telas abandona o que estava lendo. Anúncios secundários esperam a leitura
   * terminar em vez de atropelá-la.
   */
  protectsReading?: boolean;
}

type AnnounceSink = (message: string, priority: AnnouncePriority) => void;
type OriginActiveResolver = (origin?: VoiceAccessibilityOrigin) => boolean;

let announceSink: AnnounceSink | null = null;
let originActiveResolver: OriginActiveResolver = () => true;

const INACTIVE_ALLOWED_EVENTS = new Set(['completion', 'error', 'system']);

/**
 * Eventos que nunca esperam: erro precisa cortar mesmo, e `user-action` é
 * resposta direta a algo que a pessoa acabou de fazer.
 */
const NEVER_DEFERRED_EVENTS = new Set(['error', 'user-action']);

const MIN_READING_MS = 3_000;
const MS_PER_CHARACTER = 50;

/**
 * Estimativa de quanto o leitor de telas leva para ler um anúncio. Não existe
 * API que avise o fim da fala, então a duração é derivada do tamanho do texto.
 * Superestimar só atrasa um aviso secundário; subestimar corta o conteúdo.
 */
export function estimateAnnouncementReadingMs(message: string): number {
  return Math.max(MIN_READING_MS, message.length * MS_PER_CHARACTER);
}

/**
 * Um aviso adiado descreve o estado do momento em que foi produzido. Passado
 * esse tempo ele deixa de valer a pena: numa resposta longa, ou numa sequência
 * de respostas, a leitura protegida pode se estender por minutos.
 */
const MAX_DEFERRED_WAIT_MS = 30_000;

let readingProtectedUntil = 0;
/**
 * Só o último anúncio adiado é guardado: são avisos de estado transitório
 * ("carregando" é substituído por "carregadas"), e enfileirar todos faria o
 * leitor despejar histórico velho quando a leitura terminasse.
 */
let deferredAnnouncement: {
  message: string;
  priority: AnnouncePriority;
  deferredAt: number;
} | null = null;
let deferredTimer: ReturnType<typeof setTimeout> | null = null;

function discardDeferredAnnouncement() {
  deferredAnnouncement = null;
  if (deferredTimer !== null) {
    clearTimeout(deferredTimer);
    deferredTimer = null;
  }
}

function clearArbitrationState() {
  readingProtectedUntil = 0;
  discardDeferredAnnouncement();
}

export function registerAnnouncerSink(sink: AnnounceSink) {
  announceSink = sink;
}

export function unregisterAnnouncerSink() {
  announceSink = null;
  clearArbitrationState();
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

function emit(message: string, priority: AnnouncePriority) {
  if (announceSink) {
    announceSink(message, priority);
    return;
  }

  queueMicrotask(() => {
    if (announceSink) {
      announceSink(message, priority);
    }
  });
}

function flushDeferredAnnouncement() {
  deferredTimer = null;
  const pending = deferredAnnouncement;
  deferredAnnouncement = null;
  if (!pending) return;
  if (Date.now() - pending.deferredAt > MAX_DEFERRED_WAIT_MS) return;
  emit(pending.message, pending.priority);
}

function scheduleDeferredFlush(delayMs: number) {
  if (deferredTimer !== null) {
    clearTimeout(deferredTimer);
  }
  deferredTimer = setTimeout(flushDeferredAnnouncement, Math.max(0, delayMs));
}

function shouldWaitForReading(request: VoiceAnnounceRequest, now: number): boolean {
  if (request.protectsReading) return false;
  if (NEVER_DEFERRED_EVENTS.has(request.eventType ?? 'progress')) return false;
  return now < readingProtectedUntil;
}

/**
 * Retorna se o anúncio foi aceito. Aceito não é sinônimo de já falado: um aviso
 * automático pode esperar a leitura do conteúdo terminar. Chamadores usam o
 * retorno para não repetir o mesmo aviso, nunca para saber que o leitor de
 * telas já falou.
 */
export function announceWithOrigin(request: VoiceAnnounceRequest): boolean {
  if (!shouldAnnounce(request)) return false;

  const message = originActiveResolver(request.origin)
    ? request.message
    : formatInactiveAnnouncement(request);
  const priority = request.announcePriority ?? (request.eventType === 'error' ? 'assertive' : 'polite');
  const now = Date.now();

  if (shouldWaitForReading(request, now)) {
    deferredAnnouncement = { message, priority, deferredAt: now };
    scheduleDeferredFlush(readingProtectedUntil - now);
    return true;
  }

  if (request.protectsReading) {
    // Conteúdo novo reinicia a proteção; o que estava adiado espera também esta
    // leitura, porque continua descrevendo o estado atual.
    readingProtectedUntil = now + estimateAnnouncementReadingMs(message);
    if (deferredAnnouncement) {
      scheduleDeferredFlush(estimateAnnouncementReadingMs(message));
    }
  } else {
    // Um erro ou uma ação da pessoa já substituiu a leitura que estava sendo
    // protegida e mudou o contexto. Falar depois um aviso automático de antes
    // dessa mudança seria descrever um estado que já passou.
    readingProtectedUntil = 0;
    discardDeferredAnnouncement();
  }

  emit(message, priority);
  return true;
}
