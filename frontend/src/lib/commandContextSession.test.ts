import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  registerSurfaceContext,
  type SurfaceContext,
} from './commandContextProviders';

const stores = vi.hoisted(() => ({
  auth: {
    isAuthenticated: true,
    user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } as {
      userId: string;
      sessionId: string;
      role: string;
    } | null,
  },
  workspace: { workspace: { id: 'workspace-a', name: 'A', tabs: [], activeTabId: null } },
}));

vi.mock('../store/authStore', () => ({
  useAuthStore: { getState: () => stores.auth },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: { getState: () => stores.workspace },
}));

import { createTrustedCommandContextSession } from './commandContextSession';

afterEach(() => {
  stores.auth.isAuthenticated = true;
  stores.auth.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  stores.workspace.workspace = { id: 'workspace-a', name: 'A', tabs: [], activeTabId: null };
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

function context(surfaceId: string, title: string): SurfaceContext {
  return {
    surfaceType: 'editor',
    surfaceId,
    snapshotVersion: 'snapshot-1',
    title,
  };
}

describe('trusted command context session', () => {
  it('binds registration to the actual user, session, and active workspace', () => {
    const session = createTrustedCommandContextSession();
    let reads = 0;
    const cleanup = session.registerSurfaceContext('surface-1', () => {
      reads += 1;
      return context('surface-1', 'user-a');
    });

    expect(session.readSurfaceContext('surface-1')?.title).toBe('user-a');
    expect(reads).toBe(1);
    expect(session.readOwnedCommandContextFrame('surface-1')).toMatchObject({
      owner: {
        userId: 'user-a',
        sessionId: 'session-a',
        workspaceId: 'workspace-a',
      },
      frame: { surface: { surfaceId: 'surface-1' } },
    });
    // Duas capturas explícitas: a segunda também reconsulta a fonte.
    expect(reads).toBe(2);

    stores.auth.isAuthenticated = false;
    stores.auth.user = null;
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    expect(session.readOwnedCommandContextFrame('surface-1')).toBeUndefined();
    expect(reads).toBe(2);

    stores.auth.isAuthenticated = true;
    stores.auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    expect(reads).toBe(2);

    stores.auth.user = { userId: 'user-a', sessionId: 'session-a-2', role: 'user' };
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    stores.auth.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
    stores.workspace.workspace = { id: 'workspace-b', name: 'B', tabs: [], activeTabId: null };
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    expect(reads).toBe(2);

    cleanup();
  });

  it('allows a new owner to register the same surface id without an old lease removing it', () => {
    const session = createTrustedCommandContextSession();
    const oldCleanup = session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'old-user'),
    );

    stores.auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
    const newCleanup = session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'new-user'),
    );
    oldCleanup();
    expect(session.readSurfaceContext('surface-reused')?.title).toBe('new-user');
    newCleanup();
    expect(session.readSurfaceContext('surface-reused')).toBeUndefined();
  });

  it('does not use a neutral registration as a scoped authority after logout', () => {
    const neutralCleanup = registerSurfaceContext('surface-1', () => context('surface-1', 'raw-old'));
    const session = createTrustedCommandContextSession();
    stores.auth.isAuthenticated = false;
    stores.auth.user = null;

    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    expect(session.readCommandContextFrame('surface-1').surface).toBeNull();
    neutralCleanup();
  });

  it('rechecks ownership after a re-entrant logout from the getter', () => {
    const session = createTrustedCommandContextSession();
    const cleanup = session.registerSurfaceContext('surface-reentrant', () => {
      stores.auth.isAuthenticated = false;
      stores.auth.user = null;
      return context('surface-reentrant', 'must-not-escape');
    });

    expect(session.readSurfaceContextDetailed('surface-reentrant')).toEqual({
      status: 'missing',
      context: null,
    });
    cleanup();
  });

  it('does not return an owned frame when the getter switches owner', () => {
    const session = createTrustedCommandContextSession();
    const cleanup = session.registerSurfaceContext('surface-switch', () => {
      stores.auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
      stores.workspace.workspace = { id: 'workspace-b', name: 'B', tabs: [], activeTabId: null };
      return context('surface-switch', 'must-not-escape');
    });

    expect(session.readOwnedCommandContextFrame('surface-switch')).toBeUndefined();
    cleanup();
  });
});
