import { EventsOn } from '@wailsjs/runtime/runtime';

export const CHAT_TURN_EVENT_NAMES = [
  'chat:error',
  'chat:speak',
  'chat:messages_ready',
  'chat:stream',
  'chat:thinking',
  'chat:tool_start',
  'chat:tool_end',
  'chat:tool_failure',
  'chat:segment_done',
  'chat:done',
] as const;

export type ChatTurnEventName = (typeof CHAT_TURN_EVENT_NAMES)[number];

interface RoutedChatEvent {
  conversationId: string;
  turnId?: string;
}

export interface ChatTurnRoute {
  conversationId: string;
  getTurnId: () => string | null;
  bindTurnId: (turnId: string) => void;
  handlers: Partial<Record<ChatTurnEventName, (event: never) => void>>;
}

const routes = new Map<string, ChatTurnRoute>();
let globalUnsubscribers: Array<() => void> = [];

function dispatch(name: ChatTurnEventName, payload: unknown) {
  if (!payload || typeof payload !== 'object') return;
  const event = payload as RoutedChatEvent;
  if (!event.conversationId) return;
  const route = routes.get(event.conversationId);
  if (!route) return;

  const eventTurnId = typeof event.turnId === 'string' ? event.turnId.trim() : '';
  const activeTurnId = route.getTurnId()?.trim() ?? '';
  if (eventTurnId && activeTurnId && eventTurnId !== activeTurnId) return;
  if (eventTurnId && !activeTurnId) route.bindTurnId(eventTurnId);

  const handler = route.handlers[name] as ((event: RoutedChatEvent) => void) | undefined;
  handler?.(event);
}

function ensureGlobalListeners() {
  if (globalUnsubscribers.length > 0) return;
  globalUnsubscribers = CHAT_TURN_EVENT_NAMES.map((name) => (
    EventsOn(name, (event: unknown) => dispatch(name, event))
  ));
}

export function registerChatTurnRoute(route: ChatTurnRoute): () => void {
  ensureGlobalListeners();
  routes.set(route.conversationId, route);
  return () => {
    if (routes.get(route.conversationId) === route) {
      routes.delete(route.conversationId);
    }
  };
}

export function createChatTurnEventRouter(
  conversationId: string,
  getTurnId: () => string | null,
  bindTurnId: (turnId: string) => void,
) {
  const handlers: ChatTurnRoute['handlers'] = {};
  const unregister = registerChatTurnRoute({ conversationId, getTurnId, bindTurnId, handlers });
  return {
    on: <T,>(name: ChatTurnEventName, handler: (event: T) => void) => {
      const routedHandler = handler as (event: never) => void;
      handlers[name] = routedHandler;
      return () => {
        if (handlers[name] === routedHandler) delete handlers[name];
      };
    },
    unregister,
  };
}

export function clearChatTurnRoutes() {
  routes.clear();
}

// Usado apenas para isolar módulos em testes/HMR. O lifecycle normal mantém
// exatamente um listener Wails por nome durante toda a vida da aplicação.
export function resetChatEventHubForTests() {
  routes.clear();
  globalUnsubscribers.forEach((unsubscribe) => unsubscribe());
  globalUnsubscribers = [];
}
