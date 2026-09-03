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
