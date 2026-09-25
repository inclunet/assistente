import { useRef } from 'react';
import { render, cleanup } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { capturePageMutationTarget, usePageMutationCommands } from './commandPageMutation';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from './modalRegistry';

const auth = useAuthStore.getState(), workspace = useWorkspaceStore.getState();
function Surface({ modal = false, commandId = 'tasklists.update', pathname = '/tasklists', tabId }: { modal?: boolean; commandId?: 'tasklists.update' | 'tasklists.clear' | 'profiles.create' | 'profiles.update' | 'profiles.duplicate' | 'profiles.delete' | 'profiles.activate'; pathname?: string; tabId?: string }) {
  const root = useRef<HTMLDivElement>(null);
  usePageMutationCommands({ root, pathname, tabId, allowedCommands: [commandId],
    canStart: () => true, prepare: () => ({ isCurrent: () => true, readRequest: async () => ({ targetId: 'a', expectedFingerprint: 'v1', title: 'a', description: '' }) }),
  });
  return <div ref={root} className={modal ? 'modal-overlay' : undefined} data-modal-id={modal ? 'form' : undefined}>source</div>;
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'user', sessionId: 'session', role: 'user' } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'ws', activeTabId: 'tab', tabs: [] } });
});
afterEach(() => { cleanup(); unregisterOpenModal('form'); unregisterOpenModal('other'); useAuthStore.setState(auth); useWorkspaceStore.setState(workspace); vi.restoreAllMocks(); });
describe('page mutation source identity', () => {
  it('captures only the mounted route and rejects ambiguous instances', () => {
    render(<Surface />);
    expect(capturePageMutationTarget(() => '/elsewhere', 'tasklists.update')).toBeUndefined();
    const target = capturePageMutationTarget(() => '/tasklists', 'tasklists.update'); expect(target?.canCommit()).toBe(true); target?.dispose();
    render(<Surface />); expect(capturePageMutationTarget(() => '/tasklists', 'tasklists.update')).toBeUndefined();
  });
  it('never reacquires authority after auth ABA', () => {
    render(<Surface />); const target = capturePageMutationTarget(() => '/tasklists', 'tasklists.update')!;
    const owner = useAuthStore.getState().user;
    useAuthStore.setState({ user: { userId: 'other', sessionId: 'other', role: 'user' } });
    useAuthStore.setState({ user: owner }); expect(target.canCommit()).toBe(false); target.dispose();
  });
  it('requires exact topmost form modal, not an unrelated overlay', () => {
    render(<Surface modal />); registerOpenModal('form');
    const target = capturePageMutationTarget(() => '/tasklists', 'tasklists.update')!; expect(target.canCommit()).toBe(true);
    registerOpenModal('other'); expect(target.canCommit()).toBe(false);
    expect(capturePageMutationTarget(() => '/tasklists', 'tasklists.update')).toBeUndefined(); target.dispose();
  });
  it('revokes a captured source when unmounted', () => {
    const view = render(<Surface />); const target = capturePageMutationTarget(() => '/tasklists', 'tasklists.update')!;
    view.unmount(); expect(target.canCommit()).toBe(false); target.dispose();
  });
  it.each(['profiles.create', 'profiles.update', 'profiles.duplicate', 'profiles.delete', 'profiles.activate'] as const)('captures profile mutation %s with the same owner/workspace/route lease', commandId => {
    render(<Surface commandId={commandId} pathname="/profiles" />);
    const target = capturePageMutationTarget(() => '/profiles', commandId);
    expect(target?.canCommit()).toBe(true);
    target?.dispose();
  });
  it('keeps the profile delete decision lease exact and cancels a stale target', () => {
    render(<Surface commandId="profiles.delete" pathname="/profiles" modal />); registerOpenModal('form');
    const target = capturePageMutationTarget(() => '/profiles', 'profiles.delete')!;
    expect(target.canCommit()).toBe(true);
    useWorkspaceStore.setState({ workspace: { id: 'other-workspace', name: 'other', activeTabId: 'tab', tabs: [] } });
    expect(target.canCommit()).toBe(false);
    target.dispose();
  });
  it('requires the captured task-list tab to remain active, including ABA', () => {
    render(<Surface commandId="tasklists.clear" pathname="/" tabId="tab" />);
    const target = capturePageMutationTarget(() => '/', 'tasklists.clear')!;
    useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'ws', activeTabId: 'other', tabs: [] } });
    expect(target.canCommit()).toBe(false);
    useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'ws', activeTabId: 'tab', tabs: [] } });
    expect(target.canCommit()).toBe(false);
    target.dispose();
  });
});
