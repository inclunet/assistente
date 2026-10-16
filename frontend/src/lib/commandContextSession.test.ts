import { afterEach, describe, expect, it, vi } from 'vitest';
import {
  registerSurfaceContext,
  type SurfaceContext,
  type SurfaceContextCleanup,
} from './commandContextProviders';

const stores = vi.hoisted(() => ({
  listeners: new Set<() => void>(),
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
  useAuthStore: {
    getState: () => stores.auth,
    subscribe: (listener: () => void) => {
      stores.listeners.add(listener);
      return () => stores.listeners.delete(listener);
    },
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: {
    getState: () => stores.workspace,
    subscribe: (listener: () => void) => {
      stores.listeners.add(listener);
      return () => stores.listeners.delete(listener);
    },
  },
}));

import { createTrustedCommandContextSession } from './commandContextSession';
import { ReadProfileContext } from './commandContextProviders';

afterEach(() => {
  stores.listeners.clear();
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
  it('captures only a known routed app.page in its trusted context frame', () => {
    const session = createTrustedCommandContextSession(() => 'settings');
    expect(session.readCommandContextFrame().appPage).toBe('settings');
    session.dispose();
    const unknown = createTrustedCommandContextSession(() => { throw new Error('route-unavailable'); });
    expect(unknown.readCommandContextFrame().appPage).toBeNull();
    unknown.dispose();
  });

  it('não ressuscita uma lease após logout e retorno ao mesmo owner sem leitura intermediária', () => {
    const session = createTrustedCommandContextSession();
    const getter = vi.fn(() => context('surface-aba', 'old'));
    session.registerSurfaceContext('surface-aba', getter);
    stores.auth.isAuthenticated = false;
    stores.listeners.forEach((listener) => listener());
    stores.auth.isAuthenticated = true;
    stores.listeners.forEach((listener) => listener());
    expect(session.readSurfaceContext('surface-aba')).toBeUndefined();
    expect(getter).not.toHaveBeenCalled();
    session.registerSurfaceContext('surface-aba', () => context('surface-aba', 'new'));
    expect(session.readSurfaceContext('surface-aba')?.title).toBe('new');
    session.dispose();
    expect(stores.listeners.size).toBe(0);
  });

  it('recusa getter reentrante que troca workspace e volta ao anterior', () => {
    const session = createTrustedCommandContextSession();
    session.registerSurfaceContext('surface-aba', () => {
      stores.workspace.workspace.id = 'workspace-b';
      stores.listeners.forEach((listener) => listener());
      stores.workspace.workspace.id = 'workspace-a';
      stores.listeners.forEach((listener) => listener());
      return context('surface-aba', 'stale');
    });
    expect(session.readOwnedCommandContextFrame('surface-aba')).toBeUndefined();
    expect(session.readSurfaceContext('surface-aba')).toBeUndefined();
    session.dispose();
  });

  it('relê owner sem depender da notificação e preserva leases em updates do mesmo owner', () => {
    const session = createTrustedCommandContextSession();
    session.registerSurfaceContext('surface-1', () => context('surface-1', 'valid'));
    stores.workspace.workspace.name = 'renamed';
    stores.listeners.forEach((listener) => listener());
    expect(session.readSurfaceContext('surface-1')?.title).toBe('valid');
    stores.workspace.workspace.id = 'workspace-b';
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    stores.workspace.workspace.id = 'workspace-a';
    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    session.dispose();
  });

  it('monta o perfil efetivo a partir da aba/workspace canônicos', () => {
    stores.workspace.workspace = {
      id: 'workspace-a',
      name: 'A',
      activeTabId: 'tab-a',
      profile: 'workspace-profile',
      tabs: [
        {
          id: 'tab-a',
          type: 'editor',
          title: 'Editor',
          position: 0,
          profileOverride: { slug: 'tab-profile' },
        },
      ],
    } as never;
    const profile = ReadProfileContext();
    expect(profile?.slug).toBe('tab-profile');
    expect(profile?.snapshotVersion).toContain('profile:workspace-a:');

    stores.workspace.workspace = {
      id: 'workspace-a',
      name: 'A',
      activeTabId: null,
      profile: 'workspace-profile',
      tabs: [],
    } as never;
    expect(ReadProfileContext()?.slug).toBe('workspace-profile');
  });

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
    session.dispose();
  });

  it('allows a new owner to register the same surface id without an old lease removing it', () => {
    const session = createTrustedCommandContextSession();
    const oldCleanup = session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'old-user')
    );

    stores.auth.user = { userId: 'user-b', sessionId: 'session-b', role: 'user' };
    const newCleanup = session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'new-user')
    );
    oldCleanup();
    expect(session.readSurfaceContext('surface-reused')?.title).toBe('new-user');
    newCleanup();
    expect(session.readSurfaceContext('surface-reused')).toBeUndefined();
    session.dispose();
  });

  it('recusa unregister/register da mesma superfície mesmo com frame idêntico', () => {
    const session = createTrustedCommandContextSession();
    const oldCleanup = session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'same-payload')
    );
    const captured = session.readOwnedCommandContextFrame('surface-reused');
    expect(captured).toBeDefined();

    oldCleanup();
    session.registerSurfaceContext('surface-reused', () =>
      context('surface-reused', 'same-payload')
    );
    const replacement = session.readOwnedCommandContextFrame('surface-reused');
    expect(replacement?.frame.surface).toEqual(captured?.frame.surface);
    expect(replacement?.frame.modal).toEqual(captured?.frame.modal);
    expect(replacement?.frame.focus).toEqual(captured?.frame.focus);
    expect(replacement?.surfaceLease).not.toBe(captured?.surfaceLease);
    session.dispose();
  });

  it('falha fechado quando o getter substitui o registro pelo mesmo id e payload', () => {
    const session = createTrustedCommandContextSession();
    let replacementCleanup: SurfaceContextCleanup | undefined;
    session.registerSurfaceContext('surface-replaced', () => {
      replacementCleanup = session.registerSurfaceContext('surface-replaced', () =>
        context('surface-replaced', 'same-payload')
      );
      return context('surface-replaced', 'same-payload');
    });
    expect(session.readOwnedCommandContextFrame('surface-replaced')).toBeUndefined();
    replacementCleanup?.();
    session.dispose();
  });

  it('does not use a neutral registration as a scoped authority after logout', () => {
    const neutralCleanup = registerSurfaceContext('surface-1', () =>
      context('surface-1', 'raw-old')
    );
    const session = createTrustedCommandContextSession();
    stores.auth.isAuthenticated = false;
    stores.auth.user = null;

    expect(session.readSurfaceContext('surface-1')).toBeUndefined();
    expect(session.readCommandContextFrame('surface-1').surface).toBeNull();
    neutralCleanup();
    session.dispose();
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
    session.dispose();
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
    session.dispose();
  });
});
