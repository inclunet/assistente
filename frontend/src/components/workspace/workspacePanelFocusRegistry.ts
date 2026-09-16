type WorkspacePanelFocusHandler = () => boolean;

const handlers = new Map<string, WorkspacePanelFocusHandler>();
const pendingRequests = new Set<string>();

export function registerWorkspacePanelFocus(
  tabId: string,
  handler: WorkspacePanelFocusHandler,
): () => void {
  handlers.set(tabId, handler);
  if (pendingRequests.delete(tabId)) {
    window.requestAnimationFrame(() => {
      if (handlers.get(tabId) === handler) {
        handler();
      }
    });
  }
  return () => {
    if (handlers.get(tabId) === handler) {
      handlers.delete(tabId);
    }
  };
}

export function requestWorkspacePanelFocus(tabId: string): boolean {
  return handlers.get(tabId)?.() ?? false;
}

export function hasWorkspacePanelFocusHandler(tabId: string): boolean {
  return handlers.has(tabId);
}

export function queueWorkspacePanelFocus(tabId: string): void {
  pendingRequests.add(tabId);
}

export function cancelWorkspacePanelFocus(tabId: string): void {
  pendingRequests.delete(tabId);
}

export function pruneWorkspacePanelFocus(validTabIds: ReadonlySet<string>): void {
  for (const tabId of pendingRequests) {
    if (!validTabIds.has(tabId)) {
      pendingRequests.delete(tabId);
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
