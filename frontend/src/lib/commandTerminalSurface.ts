import type { WorkspaceData, WorkspaceTab } from '../store/workspaceStore';
import type { SessionInfo } from '../store/terminalStore';
import type { SurfaceContext } from './chatSurface';

export interface TerminalSurfaceReaderState {
  readonly workspace: WorkspaceData | null | undefined;
  readonly tab: WorkspaceTab | null | undefined;
  readonly isActive: boolean;
  readonly currentSessionId: string | null | undefined;
  readonly session: SessionInfo | null | undefined;
}

/** Lê somente fatos pequenos e autoritativos da aba terminal atual. */
export function readTerminalSurfaceContext(
  state: TerminalSurfaceReaderState,
): SurfaceContext | null {
  const { workspace, tab, currentSessionId, session } = state;
  if (!state.isActive || !workspace || !tab || tab.type !== 'terminal') return null;
  if (workspace.activeTabId !== tab.id) return null;
  if (!currentSessionId || tab.state?.sessionId !== currentSessionId) return null;
  if (!session || session.id !== currentSessionId) return null;

  const sessionState = session.state || '';
  const shell = session.shell || '';

  return {
    surfaceType: 'terminal',
    surfaceId: tab.id,
    title: session.name,
    mode: sessionState || undefined,
    metadata: {
      sessionId: currentSessionId,
      state: session.state,
      shell: session.shell,
    },
    snapshotVersion: JSON.stringify([currentSessionId, sessionState, shell, session.name]),
  };
}
