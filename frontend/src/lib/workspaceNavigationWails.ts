interface WorkspaceNavigationAPI {
  SetActiveWorkspaceTabForWorkspace(workspaceID: string, tabID: string): Promise<unknown>;
}

type WorkspaceNavigationWindow = Window & {
  go?: {
    wailsapi?: {
      Workspace?: Partial<WorkspaceNavigationAPI>;
    };
  };
};

export interface WorkspaceNavigationWailsOptions {
  readonly target?: WorkspaceNavigationWindow;
}

function workspaceNavigationAPI(target: WorkspaceNavigationWindow): WorkspaceNavigationAPI {
  const api = target.go?.wailsapi?.Workspace;
  if (typeof api?.SetActiveWorkspaceTabForWorkspace !== 'function') {
    throw new Error('Scoped workspace navigation Wails API is not available');
  }
  return api as WorkspaceNavigationAPI;
}

export async function setActiveWorkspaceTabForWorkspace(
  workspaceID: string,
  tabID: string,
  options: WorkspaceNavigationWailsOptions = {},
): Promise<unknown> {
  const target = options.target ?? (window as WorkspaceNavigationWindow);
  const api = workspaceNavigationAPI(target);
  try {
    // The store only submits after bootstrap has observed the bridge. Keeping
    // this call synchronous avoids a wait window in which session/workspace
    // identity could change after the store's guards ran.
    return Promise.resolve(api.SetActiveWorkspaceTabForWorkspace(workspaceID, tabID));
  } catch (error) {
    return Promise.reject(error);
  }
}
