import { useCallback, useId, useLayoutEffect, useRef, type RefObject } from 'react';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { getModalRegistrySnapshot } from './modalRegistry';
import { ReadFocusContext } from './commandContextProviders';
import type { CommandContextualBackendPort } from './commandContextualBackendExecution';
import type { UICommandStatus } from './commandUIExecution';
import type { profiles } from '@wailsjs/go/models';
import { useCommandContextScope } from './commandContextReact';

export const PAGE_MUTATION_IDS = [
  'tasklists.create', 'tasklists.update', 'tasklists.duplicate', 'tasklists.clear', 'tasklists.delete',
  'profiles.create', 'profiles.update', 'profiles.duplicate', 'profiles.delete', 'profiles.activate',
] as const;
export type PageMutationID = typeof PAGE_MUTATION_IDS[number];
export const PAGE_MUTATION_EVENT = 'commands:page-mutation';
export function isPageMutationCommand(id: string): id is PageMutationID { return (PAGE_MUTATION_IDS as readonly string[]).includes(id); }
export interface PageMutationRequest {
  readonly targetId: string;
  readonly expectedFingerprint: string;
  readonly title: string;
  readonly description: string;
  readonly profile?: profiles.Profile;
}
export interface PageMutationResult { readonly id: string; readonly title: string }
export interface PageMutationOutcome { status: UICommandStatus; result?: PageMutationResult }
export interface PageMutationPort extends CommandContextualBackendPort {
  preparePageMutationCommand(ticket: string, request: PageMutationRequest): Promise<void>;
  getPageMutationCommandResult(ticket: string): Promise<PageMutationResult>;
}
interface Prepared { readRequest(): Promise<PageMutationRequest>; isCurrent(): boolean; canPresent?(): boolean; succeeded?(result: PageMutationResult): void | Promise<void> }
interface Options {
  root: RefObject<HTMLElement>; pathname: string; tabId?: string; allowedCommands: readonly PageMutationID[];
  canStart(id: PageMutationID): boolean;
  prepare(id: PageMutationID): Prepared | undefined;
  subscribe?(changed: () => void): () => void;
}
interface Source { id: string; options(): Options; changed: Set<() => void> }
const sources = new Set<Source>();
export interface PageMutationTarget {
  readonly commandId: PageMutationID; readonly instanceId: string;
  isCurrent(): boolean; canCommit(): boolean;
  readRequest(): Promise<PageMutationRequest>;
  startDecision(): void; finishDecision(): boolean;
  succeeded(result: PageMutationResult): Promise<void>; dispose(): void;
}
export interface PageMutationEventDetail {
  commandId: PageMutationID; instanceId: string;
  resolve(result: PageMutationOutcome): void;
}
function scope(source: Source) {
  const options = source.options(); const root = options.root.current;
  const auth = useAuthStore.getState(); const ws = useWorkspaceStore.getState().workspace;
  if (!root?.isConnected || !auth.isAuthenticated || !auth.user || !ws || options.tabId && ws.activeTabId !== options.tabId) return;
  return JSON.stringify([auth.user.userId, auth.user.sessionId, ws.id, ws.activeTabId, options.pathname]);
}
function modalOwner(source: Source) { return source.options().root.current?.closest('[data-modal-id]')?.getAttribute('data-modal-id') ?? null; }
export function capturePageMutationTarget(readPath: () => string, id: string, instanceId?: string): PageMutationTarget | undefined {
  if (!isPageMutationCommand(id) || !document.hasFocus() || ReadFocusContext().composition === 'active') return;
  const modal = getModalRegistrySnapshot();
  const matches = [...sources].filter(source => {
    const options = source.options(); const root = options.root.current;
    return (!instanceId || source.id === instanceId) && options.pathname === readPath() && options.allowedCommands.includes(id) &&
      scope(source) && modalOwner(source) === modal.topID && root && !root.closest('[hidden],[inert],[aria-hidden="true"]') && options.canStart(id);
  });
  if (matches.length !== 1) return;
  const source = matches[0], capturedScope = scope(source), capturedRoot = source.options().root.current, prepared = source.options().prepare(id);
  if (!prepared) return;
  const pathname = readPath(); let generation = modal.generation;
  let disposed = false, invalid = false, decisionPending = false;
  const cleanups: (() => void)[] = [];
  const dispose = () => { if (disposed) return; disposed = true; cleanups.splice(0).forEach(cleanup => cleanup()); };
  const isCurrent = () => {
    if (disposed || invalid) return false;
    const current = getModalRegistrySnapshot();
    const decisionStack = decisionPending && modal.ids.every((id, i) => current.ids[i] === id) &&
      (current.ids.length === modal.ids.length || current.ids.length === modal.ids.length + 1 && current.dialogCommandScope?.kind === 'decision');
    const valid = sources.has(source) && source.options().root.current === capturedRoot && capturedScope === scope(source) && pathname === readPath() && prepared.isCurrent() &&
      (decisionStack || current.generation === generation);
    if (!valid) invalid = true;
    return valid;
  };
  const changed = () => { isCurrent(); }; const invalidate = () => { invalid = true; };
  source.changed.add(changed);
  cleanups.push(() => source.changed.delete(changed), useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed));
  const unsubscribe = source.options().subscribe?.(changed); if (unsubscribe) cleanups.push(unsubscribe);
  window.addEventListener('blur', invalidate); cleanups.push(() => window.removeEventListener('blur', invalidate));
  const target: PageMutationTarget = {
    commandId: id, instanceId: source.id, isCurrent, dispose,
    canCommit: () => isCurrent() && document.hasFocus() && ReadFocusContext().composition !== 'active' && modalOwner(source) === getModalRegistrySnapshot().topID,
    readRequest: prepared.readRequest,
    startDecision: () => { decisionPending = id === 'tasklists.delete' || id === 'tasklists.clear' || id === 'profiles.delete'; },
    finishDecision: () => {
      const current = getModalRegistrySnapshot();
      if (current.ids.length !== modal.ids.length || !current.ids.every((id, i) => id === modal.ids[i])) invalid = true;
      generation = current.generation; decisionPending = false; return isCurrent();
    },
    succeeded: async result => { if (sources.has(source) && capturedScope === scope(source) && pathname === readPath() && (prepared.canPresent?.() ?? prepared.isCurrent())) await prepared.succeeded?.(result); },
  };
  if (!target.canCommit()) { dispose(); return; }
  return target;
}
export function usePageMutationCommands(options: Options) {
  const contextScope = useCommandContextScope();
  const id = useId(); const ref = useRef(options); ref.current = options;
  const source = useRef<Source>({ id, options: () => ref.current, changed: new Set() });
  const provider = useRef<{ root: HTMLElement; rootRef: RefObject<HTMLElement>; scope: typeof contextScope; type: string; dispose(): void }>();
  // Standalone pages own this provider; a workspace tab already has its own
  // canonical surface registration, which must never be shadowed here.
  useLayoutEffect(() => {
    const root = options.root.current;
    const domain = options.allowedCommands.length > 0 && options.allowedCommands.every(command => command.startsWith('tasklists.'))
      ? 'tasklists' : options.allowedCommands.length > 0 && options.allowedCommands.every(command => command.startsWith('profiles.')) ? 'profiles' : undefined;
    const surfaceType = !options.tabId && domain && options.pathname === `/${domain}` ? domain : undefined;
    const previous = provider.current;
    if (previous && previous.root === root && previous.rootRef === options.root && previous.scope === contextScope && previous.type === surfaceType && !root?.closest('[data-modal-id]')) return;
    previous?.dispose(); provider.current = undefined;
    if (!contextScope || !root || !surfaceType || root.closest('[data-modal-id]')) return;
    const rootRef = options.root;
    const dispose = contextScope.registerSurface(id, rootRef, () => ({ surfaceId: id, surfaceType, snapshotVersion: `page-mutation:${id}` }));
    provider.current = { root, rootRef, scope: contextScope, type: surfaceType, dispose };
  });
  useLayoutEffect(() => () => { provider.current?.dispose(); provider.current = undefined; }, []);
  useLayoutEffect(() => { const current = source.current; sources.add(current); return () => { sources.delete(current); current.changed.forEach(fn => fn()); }; }, []);
  useLayoutEffect(() => { source.current.changed.forEach(fn => fn()); });
  const request = useCallback((commandId: PageMutationID): Promise<PageMutationOutcome> => new Promise(resolve => {
    const event = new CustomEvent<PageMutationEventDetail>(PAGE_MUTATION_EVENT, { detail: { commandId, instanceId: id, resolve }, cancelable: true });
    window.dispatchEvent(event); if (!event.defaultPrevented) resolve({ status: 'failed' });
  }), [id]);
  return { request };
}
export async function executePageMutation(port: PageMutationPort, target: PageMutationTarget): Promise<PageMutationOutcome> {
  let ticket: string | undefined, handoff: string | undefined, invocation: string | undefined;
  let submitted = false, taking = false;
  try {
    if (!target.canCommit()) return { status: 'cancelled' };
    const request = await target.readRequest();
    if (!target.canCommit()) return { status: 'cancelled' };
    const reservation = await port.beginUICommand(target.commandId);
    ticket = reservation?.ticket; invocation = reservation?.invocationId;
    if (!ticket || !invocation || reservation.commandId !== target.commandId || !target.canCommit()) return { status: 'cancelled' };
    target.startDecision();
    await port.preparePageMutationCommand(ticket, request);
    if (!target.isCurrent()) return { status: 'cancelled' };
    taking = true;
    const taken = await port.takeUICommand(ticket); handoff = taken?.handoffId;
    if (!target.finishDecision() || !handoff || taken.ticket !== ticket || taken.invocationId !== invocation || taken.commandId !== target.commandId || !target.canCommit()) return { status: 'cancelled' };
    submitted = true;
    await port.commitBackendCommand(ticket, handoff);
    const completion = await port.getUICommandResult(ticket);
    if (completion.invocationId !== invocation) return { status: 'outcome_unknown' };
    if (completion.status !== 'succeeded') return { status: completion.status };
    const result = await port.getPageMutationCommandResult(ticket);
    try { await target.succeeded(result); } catch { /* persisted success; presentation must not retry */ }
    return { status: 'succeeded', result };
  } catch {
    if (submitted) return { status: 'outcome_unknown' };
    if (taking && ticket) {
      try { const result = await port.getUICommandResult(ticket); if (result.invocationId === invocation && ['cancelled', 'denied', 'cancelled_stale', 'rejected_stale', 'timed_out'].includes(result.status)) return { status: result.status }; } catch { /* no effect submitted */ }
    }
    return { status: 'failed' };
  } finally {
    if (!submitted && ticket) { try { if (handoff) await port.completeUICommand(ticket, handoff, 'cancelled'); else await port.cancelUICommand(ticket); } catch { /* expiry owns cleanup */ } }
    target.dispose();
  }
}
