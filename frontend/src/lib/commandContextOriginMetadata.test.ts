import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { createCommandContextScope } from './commandContextReact';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';

const cleanups: Array<() => void> = [];
beforeEach(() => {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u', sessionId: 's', role: 'user' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'doc', tabs: [{ id: 'doc', type: 'editor', title: 'Doc', position: 0 }] } });
});
afterEach(() => { cleanups.splice(0).reverse().forEach(fn => fn()); unregisterOpenModal('owned'); unregisterOpenModal('foreign'); document.getElementById('root')?.remove(); });
function fixture() {
  const app = document.createElement('div'); app.id = 'root'; document.body.append(app);
  const panel = document.createElement('div'); app.append(panel); const ref = { current: panel };
  const scope = createCommandContextScope(); cleanups.push(scope.dispose);
  const getter = vi.fn(() => ({ surfaceId: 'doc', surfaceType: 'editor', snapshotVersion: 'v1' }));
  const off = scope.registerSurface('doc', ref, getter);
  const first = scope.session.readOwnedCommandContextFrame('doc');
  const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay); cleanups.push(() => overlay.remove());
  registerOpenModal('owned');
  return { app, panel, ref, scope, getter, first, off };
}
it('metadata bypasses only app modal attributes, preserves lease, and never changes ordinary reads', () => {
  const f = fixture();
  expect(f.scope.session.readOwnedCommandContextFrame('doc')?.frame.surface).toBeNull();
  const metadata = f.scope.readOwnedCommandOriginMetadata('doc', 'owned');
  expect(metadata?.frame.surface?.snapshotVersion).toBe('v1'); expect(metadata?.surfaceLease).toBe(f.first?.surfaceLease);
  expect(f.scope.session.readOwnedCommandContextFrame('doc')?.frame.surface).toBeNull();
  unregisterOpenModal('owned');
  expect(f.scope.session.readOwnedCommandContextFrame('doc')?.surfaceLease).toBe(metadata?.surfaceLease);
  expect(f.scope.readOwnedCommandOriginMetadata('doc', 'owned')).toBeUndefined();
});
it.each(['hidden-panel', 'inert-panel', 'aria-panel', 'hidden-app', 'foreign', 'remount', 'dispose', 'owner', 'missing'])('metadata rejects %s', mode => {
  const f = fixture();
  if (mode === 'hidden-panel') f.panel.hidden = true;
  if (mode === 'inert-panel') f.panel.setAttribute('inert', '');
  if (mode === 'aria-panel') f.panel.setAttribute('aria-hidden', 'true');
  if (mode === 'hidden-app') f.app.hidden = true;
  if (mode === 'foreign') registerOpenModal('foreign');
  if (mode === 'remount') { f.ref.current = document.createElement('div'); f.app.append(f.ref.current); }
  if (mode === 'dispose') f.off();
  if (mode === 'owner') useAuthStore.setState({ user: { userId: 'u', sessionId: 'other', role: 'user' } });
  expect(f.scope.readOwnedCommandOriginMetadata(mode === 'missing' ? 'missing' : 'doc', 'owned')).toBeUndefined();
});
it('reentrant ordinary reads cannot inherit the metadata exception', () => {
  const f = fixture(); let nested: unknown;
  f.getter.mockImplementation(() => {
    nested = f.scope.session.readOwnedCommandContextFrame('doc')?.frame.surface;
    return { surfaceId: 'doc', surfaceType: 'editor', snapshotVersion: 'v1' };
  });
  expect(f.scope.readOwnedCommandOriginMetadata('doc', 'owned')?.frame.surface).toBeTruthy();
  expect(nested).toBeNull();
});
it('reentrant modal generation change rejects a captured frame', () => {
  const f = fixture();
  f.getter.mockImplementation(() => { registerOpenModal('foreign'); unregisterOpenModal('foreign'); return { surfaceId: 'doc', surfaceType: 'editor', snapshotVersion: 'v1' }; });
  expect(f.scope.readOwnedCommandOriginMetadata('doc', 'owned')).toBeUndefined();
});
