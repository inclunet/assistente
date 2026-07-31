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
 * Um aviso transitório descreve o estado do momento em que foi produzido.
 * Passado esse tempo ele deixa de valer a pena: numa resposta longa, ou numa
 * sequência de respostas, a leitura protegida pode se estender por minutos.
 */
const MAX_TRANSIENT_WAIT_MS = 30_000;

/** Teto de segurança para a fila não crescer sem limite. */
const MAX_DEFERRED_QUEUE = 5;

interface DeferredAnnouncement {
  message: string;
  priority: AnnouncePriority;
  deferredAt: number;
  /**
   * Conclusão de resposta é evento, não estado: continua verdadeira depois e o
   * AEP-0058 exige que aba inativa possa anunciá-la. Não pode ser descartada
   * junto com os avisos de estado transitório.
   */
  durable: boolean;
}

let readingProtectedUntil = 0;
let deferredQueue: DeferredAnnouncement[] = [];
let deferredTimer: ReturnType<typeof setTimeout> | null = null;

function cancelDeferredTimer() {
  if (deferredTimer !== null) {
    clearTimeout(deferredTimer);
    deferredTimer = null;
  }
}

/**
 * Avisos de estado transitório saem da fila assim que a conversa anda: falá-los
 * depois descreveria um instante que já passou. Os duráveis permanecem.
 */
function discardTransientAnnouncements() {
  deferredQueue = deferredQueue.filter((item) => item.durable);
  if (deferredQueue.length === 0) {
    cancelDeferredTimer();
  }
}

function clearArbitrationState() {
  readingProtectedUntil = 0;
  deferredQueue = [];
  cancelDeferredTimer();
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

function flushNextDeferredAnnouncement() {
  deferredTimer = null;
  const now = Date.now();
  const next = deferredQueue.find(
    (item) => item.durable || now - item.deferredAt <= MAX_TRANSIENT_WAIT_MS,
  );
  deferredQueue = next ? deferredQueue.slice(deferredQueue.indexOf(next) + 1) : [];
  if (!next) return;

  emit(next.message, next.priority);
  const readingMs = estimateAnnouncementReadingMs(next.message);
  if (next.durable) {
    // Este aviso esperou justamente porque não pode se perder; deixá-lo ser
    // substituído no meio o perderia do mesmo jeito. Um aviso de estado, ao
    // contrário, deve ceder lugar a um estado mais recente: é o mais novo que
    // descreve a situação atual.
    readingProtectedUntil = Date.now() + readingMs;
  }
  if (deferredQueue.length > 0) {
    // Um por vez: falar dois seguidos faria o segundo substituir o primeiro.
    scheduleDeferredFlush(readingMs);
  }
}

function scheduleDeferredFlush(delayMs: number) {
  cancelDeferredTimer();
  deferredTimer = setTimeout(flushNextDeferredAnnouncement, Math.max(0, delayMs));
}

function enqueueDeferredAnnouncement(item: DeferredAnnouncement) {
  if (!item.durable) {
    // Entre avisos de estado só o mais recente descreve a situação atual.
    deferredQueue = deferredQueue.filter((queued) => queued.durable);
  }
  deferredQueue = [...deferredQueue, item].slice(-MAX_DEFERRED_QUEUE);
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
    enqueueDeferredAnnouncement({
      message,
      priority,
      deferredAt: now,
      durable: request.eventType === 'completion',
    });
    scheduleDeferredFlush(readingProtectedUntil - now);
    return true;
  }

  // Conteúdo novo reinicia a proteção; erro e ação da pessoa a encerram, porque
  // já substituíram a leitura que estava sendo protegida.
  readingProtectedUntil = request.protectsReading
    ? now + estimateAnnouncementReadingMs(message)
    : 0;
  discardTransientAnnouncements();
  if (deferredQueue.length > 0) {
    scheduleDeferredFlush(Math.max(readingProtectedUntil - now, estimateAnnouncementReadingMs(message)));
  }

  emit(message, priority);
  return true;
}
