import { describe, expect, it } from 'vitest';
import type { WorkspaceData, WorkspaceTab } from '../store/workspaceStore';
import type { SessionInfo } from '../store/terminalStore';
import { readTerminalSurfaceContext, type TerminalSurfaceReaderState } from './commandTerminalSurface';

const tab: WorkspaceTab = {
  id: 'tab-terminal', type: 'terminal', title: 'Terminal', position: 0,
  state: { sessionId: 'session-1' },
};
const workspace: WorkspaceData = {
  id: 'workspace-1', name: 'Workspace', activeTabId: tab.id, tabs: [tab], profile: 'default',
};
const session = {
  id: 'session-1', name: 'Shell', cwd: '/tmp', state: 'running', shell: 'sh',
  createdAt: '2026-01-01T00:00:00Z', lastUsed: '2026-01-01T00:00:00Z',
} as SessionInfo;

function state(overrides: Partial<TerminalSurfaceReaderState> = {}): TerminalSurfaceReaderState {
  return { workspace, tab, isActive: true, currentSessionId: session.id, session, ...overrides };
}

describe('command terminal surface reader', () => {
  it('retorna null quando a fonte autoritativa está ausente ou foi retargeted', () => {
    expect(readTerminalSurfaceContext(state({ workspace: null }))).toBeNull();
    expect(readTerminalSurfaceContext(state({ session: null }))).toBeNull();
    expect(readTerminalSurfaceContext(state({ currentSessionId: 'session-2' }))).toBeNull();
    expect(readTerminalSurfaceContext(state({
      tab: { ...tab, state: { sessionId: 'session-2' } },
    }))).toBeNull();
    expect(readTerminalSurfaceContext(state({ isActive: false }))).toBeNull();
  });

  it('detecta troca de aba e de sessão sem depender de notify', () => {
    const first = readTerminalSurfaceContext(state());
    expect(first?.surfaceId).toBe(tab.id);
    expect(first?.metadata).toEqual({ sessionId: 'session-1', state: 'running', shell: 'sh' });

    const retargeted = readTerminalSurfaceContext(state({
      workspace: { ...workspace, activeTabId: 'other-tab' },
    }));
    expect(retargeted).toBeNull();

    const changed = readTerminalSurfaceContext(state({
      session: { ...session, state: 'exited' },
    }));
    expect(changed).not.toBeNull();
    expect(changed?.snapshotVersion).not.toBe(first?.snapshotVersion);
  });

  it('publica apenas identidade e estado, sem histórico ou output', () => {
    const result = readTerminalSurfaceContext(state());
    expect(result).toMatchObject({ surfaceType: 'terminal', surfaceId: tab.id });
    expect(JSON.stringify(result)).not.toContain('output');
    expect(JSON.stringify(result)).not.toContain('history');
    expect(JSON.stringify(result)).not.toContain('cwd');
    expect(result?.selection).toBeUndefined();
    expect(result?.content).toBeUndefined();
    expect(result?.focus).toBeUndefined();
  });
});
