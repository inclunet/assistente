export type WorkspacePanelFocusHandler = () => boolean;

const handlers = new Map<string, WorkspacePanelFocusHandler>();
const immediateHandlers = new Map<string, WorkspacePanelFocusHandler>();
const immediateReadiness = new Map<string, () => boolean>();
const registrationTokens = new Map<string, object>();
interface PendingFocusRequest {
  readonly guard?: () => boolean;
  readonly onApplied?: () => void;
  readonly onRejected?: () => void;
  readonly onCancelled?: () => void;
  readonly requireImmediate?: boolean;
}

const pendingRequests = new Map<string, PendingFocusRequest>();
const scheduledRequests = new Map<string, { frame: number; request: PendingFocusRequest }>();

function deliverFocusRequest(tabId: string, handler: WorkspacePanelFocusHandler, request: PendingFocusRequest): void {
  if (request.guard && !request.guard()) {
    request.onRejected?.();
    return;
  }
  const immediate = immediateHandlers.get(tabId);
  const focused = request.requireImmediate
    ? Boolean(immediate && canFocusWorkspacePanelImmediately(tabId) && immediate())
    : handler();
  if (focused) request.onApplied?.();
  else request.onRejected?.();
}

export function registerWorkspacePanelFocus(
  tabId: string,
  handler: WorkspacePanelFocusHandler,
  immediateHandler?: WorkspacePanelFocusHandler,
  canFocus?: () => boolean,
): () => void {
  const token = {};
  registrationTokens.set(tabId, token);
  handlers.set(tabId, handler);
  const immediateCapability = immediateHandler ? () => immediateHandler() : undefined;
  if (immediateCapability) immediateHandlers.set(tabId, immediateCapability);
  else immediateHandlers.delete(tabId);
  if (immediateCapability && canFocus) immediateReadiness.set(tabId, canFocus);
  else immediateReadiness.delete(tabId);
  const pendingRequest = pendingRequests.get(tabId);
  if (pendingRequests.delete(tabId) && pendingRequest) {
    scheduleFocusRequest(tabId, handler, pendingRequest);
  }
  return () => {
    if (registrationTokens.get(tabId) === token) {
      registrationTokens.delete(tabId);
      handlers.delete(tabId);
      immediateHandlers.delete(tabId);
      immediateReadiness.delete(tabId);
    }
  };
}

export function requestWorkspacePanelFocus(tabId: string): boolean {
  const handler = handlers.get(tabId);
  if (!handler) return false;
  return handler();
}

/** Retorna somente a capability registrada como foco síncrono. */
export function getWorkspacePanelImmediateFocusHandler(
  tabId: string,
): WorkspacePanelFocusHandler | undefined {
  return immediateHandlers.get(tabId);
}

export function canFocusWorkspacePanelImmediately(tabId: string): boolean {
  const immediateHandler = immediateHandlers.get(tabId);
  if (!immediateHandler) return false;
  return immediateReadiness.get(tabId)?.() ?? true;
}

export function hasWorkspacePanelFocusHandler(tabId: string): boolean {
  return handlers.has(tabId);
}

export function queueWorkspacePanelFocus(
  tabId: string,
  guard?: () => boolean,
  onApplied?: () => void,
  onRejected?: () => void,
  onCancelled?: () => void,
  requireImmediate = false,
): void {
  cancelWorkspacePanelFocus(tabId);
  const request = { guard, onApplied, onRejected, onCancelled, requireImmediate };
  const handler = handlers.get(tabId);
  if (handler) {
    scheduleFocusRequest(tabId, handler, request);
    return;
  }
  pendingRequests.set(tabId, request);
}

export function cancelWorkspacePanelFocus(tabId: string): void {
  const pending = pendingRequests.get(tabId);
  pendingRequests.delete(tabId);
  pending?.onCancelled?.();
  const scheduled = scheduledRequests.get(tabId);
  if (scheduled !== undefined) {
    window.cancelAnimationFrame(scheduled.frame);
    scheduledRequests.delete(tabId);
    scheduled.request.onCancelled?.();
  }
}

function scheduleFocusRequest(tabId: string, handler: WorkspacePanelFocusHandler, request: PendingFocusRequest): void {
  const previous = scheduledRequests.get(tabId);
  if (previous) {
    window.cancelAnimationFrame(previous.frame);
    previous.request.onCancelled?.();
  }
  const frame = window.requestAnimationFrame(() => {
    const current = scheduledRequests.get(tabId);
    if (!current || current.request !== request) return;
    scheduledRequests.delete(tabId);
    if (handlers.get(tabId) !== handler) {
      request.onCancelled?.();
      return;
    }
    deliverFocusRequest(tabId, handler, request);
  });
  scheduledRequests.set(tabId, { frame, request });
}

export function pruneWorkspacePanelFocus(validTabIds: ReadonlySet<string>): void {
  const tabIds = new Set([...pendingRequests.keys(), ...scheduledRequests.keys()]);
  for (const tabId of tabIds) {
    if (!validTabIds.has(tabId)) {
      cancelWorkspacePanelFocus(tabId);
    }
  }
}

/**
 * Decide para onde mandar o foco ao ativar/fechar uma aba, unificando o contrato
 * usado na troca por atalho, na navegação por número e ao fechar aba.
 *
 * Todo painel de workspace (chat, terminal, editor, tasklist) registra um
 * handler de foco próprio, então a decisão é única e agnóstica de tipo:
 *
 * 1. Se o painel já registrou o handler, invoca-o.
 * 2. Senão (painel ainda lazy/não montado), enfileira o pedido para ser refeito
 *    assim que o painel montar e registrar o handler.
 */
export function routeWorkspacePanelFocus(tabId: string): void {
  if (hasWorkspacePanelFocusHandler(tabId)) {
    requestWorkspacePanelFocus(tabId);
  } else {
    queueWorkspacePanelFocus(tabId);
  }
}
