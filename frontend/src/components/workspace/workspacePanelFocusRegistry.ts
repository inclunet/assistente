type WorkspacePanelFocusHandler = () => boolean;

const handlers = new Map<string, WorkspacePanelFocusHandler>();

export function registerWorkspacePanelFocus(
  tabId: string,
  handler: WorkspacePanelFocusHandler,
): () => void {
  handlers.set(tabId, handler);
  return () => {
    if (handlers.get(tabId) === handler) {
      handlers.delete(tabId);
    }
  };
}

export function requestWorkspacePanelFocus(tabId: string): boolean {
  return handlers.get(tabId)?.() ?? false;
}
