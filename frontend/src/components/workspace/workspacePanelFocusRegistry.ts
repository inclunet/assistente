import { restoreDefaultFocus } from '../../hooks/useDefaultFocus';

type WorkspacePanelFocusHandler = () => boolean;

const handlers = new Map<string, WorkspacePanelFocusHandler>();
const pendingRequests = new Set<string>();

/**
 * Tipos de aba cujo conteúdo carrega de forma assíncrona (editor hidrata
 * Monaco/rich; tasklist carrega páginas do board). Para eles o foco não pode
 * depender do DOM estar pronto no instante da troca de aba: quando ainda não há
 * handler registrado, o pedido é enfileirado (`queueWorkspacePanelFocus`) e
 * refeito assim que o painel monta e registra seu handler.
 */
const ASYNC_PANEL_TYPES = new Set<string>(['editor', 'tasklist']);

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
 * usado na troca por atalho, na navegação por número e ao fechar aba:
 *
 * 1. Se o painel já registrou um handler de foco próprio, invoca-o.
 * 2. Senão, para painéis assíncronos (editor/tasklist), enfileira o pedido para
 *    ser refeito quando o painel montar e registrar o handler.
 * 3. Caso contrário, cai no default focus da página (chat/terminal/landmark).
 */
export function routeWorkspacePanelFocus(tabId: string, tabType?: string): void {
  if (hasWorkspacePanelFocusHandler(tabId)) {
    requestWorkspacePanelFocus(tabId);
  } else if (tabType !== undefined && ASYNC_PANEL_TYPES.has(tabType)) {
    queueWorkspacePanelFocus(tabId);
  } else {
    restoreDefaultFocus();
  }
}
