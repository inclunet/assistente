import { useCallback, useId, useLayoutEffect, useRef, type RefObject } from 'react';
import { useAuthStore } from '../store/authStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { ReadFocusContext } from './commandContextProviders';
import { getModalRegistrySnapshot } from './modalRegistry';

export const PAGE_PRESENTATION_COMMAND_IDS = [
  'tasklists.create.open', 'tasklists.edit.open', 'tasklists.search.focus',
  'tasklist.task.create.open',
  'profiles.create.open', 'profiles.edit.open', 'profiles.search.focus',
  'terminal.sessions.open', 'terminal.focus.input', 'terminal.focus.history',
] as const;
export type PagePresentationCommandID = typeof PAGE_PRESENTATION_COMMAND_IDS[number];
export const PAGE_PRESENTATION_COMMAND_EVENT = 'commands:page-presentation';
export function isPagePresentationCommand(id: string): id is PagePresentationCommandID {
  return (PAGE_PRESENTATION_COMMAND_IDS as readonly string[]).includes(id);
}
export interface PagePresentationOptions {
  root: RefObject<HTMLElement>;
  pathname: string;
  tabId?: string;
  allowedCommands: readonly PagePresentationCommandID[];
  readTarget(): unknown;
  isCurrent(): boolean;
  canOpen(id: PagePresentationCommandID): boolean;
  open(id: PagePresentationCommandID): boolean;
  subscribe?(changed: () => void): () => void;
}
interface Source { instanceId: string; root: HTMLElement; options(): PagePresentationOptions; listeners: Set<() => void> }
const sources = new Set<Source>();
export interface PagePresentationTarget {
  isCurrent(): boolean;
  canOpen(id: string): boolean;
  open(id: string): boolean;
  dispose(): void;
}
function identity(source: Source): string | undefined {
  const options = source.options();
  const auth = useAuthStore.getState();
  const ws = useWorkspaceStore.getState().workspace;
  if (!auth.isAuthenticated || !auth.user || !ws || !options.isCurrent() ||
      !source.root.isConnected || source.root.closest('[hidden],[inert],[aria-hidden="true"]') ||
      getModalRegistrySnapshot().topID || (options.tabId && ws.activeTabId !== options.tabId)) return;
  for (let element: HTMLElement | null = source.root; element; element = element.parentElement) {
    const style = getComputedStyle(element);
    if (style.display === 'none' || style.visibility === 'hidden' || style.visibility === 'collapse') return;
  }
  return JSON.stringify([auth.user.userId, auth.user.sessionId, ws.id, ws.activeTabId, options.pathname, options.tabId]);
}
export function capturePagePresentationTarget(readPathname: () => string, commandID: string, instanceId?: string): PagePresentationTarget | undefined {
  try { return captureTarget(readPathname, commandID, instanceId); } catch { return undefined; }
}
function captureTarget(readPathname: () => string, commandID: string, instanceId?: string): PagePresentationTarget | undefined {
  if (!isPagePresentationCommand(commandID) || !document.hasFocus() || ReadFocusContext().composition === 'active') return;
  const path = readPathname();
  const origin = document.activeElement;
  if (!instanceId && origin?.closest('[role="menu"],[role="listbox"],[role="combobox"]:not([aria-expanded="false"])')) return;
  const candidates = [...sources].filter(source => {
    const options = source.options();
    return options.pathname === path && options.allowedCommands.includes(commandID) && identity(source) &&
      (!instanceId || source.instanceId === instanceId) && options.canOpen(commandID);
  });
  if (candidates.length !== 1) return;
  const source = candidates[0];
  const capturedIdentity = identity(source);
  if (!capturedIdentity) return;
  const target = source.options().readTarget();
  const modalGeneration = getModalRegistrySnapshot().generation;
  let disposed = false;
  const cleanups: (() => void)[] = [];
  const dispose = () => { if (disposed) return; disposed = true; cleanups.splice(0).forEach(cleanup => cleanup()); };
  const isCurrent = () => {
    if (disposed) return false;
    try {
      const valid = sources.has(source) && identity(source) === capturedIdentity && source.options().readTarget() === target &&
        path === readPathname() && getModalRegistrySnapshot().generation === modalGeneration &&
        document.hasFocus() && ReadFocusContext().composition !== 'active';
      if (!valid) dispose();
      return valid;
    } catch { dispose(); return false; }
  };
  const changed = () => { isCurrent(); };
  source.listeners.add(changed);
  cleanups.push(() => source.listeners.delete(changed), useAuthStore.subscribe(changed), useWorkspaceStore.subscribe(changed));
  try {
    const off = source.options().subscribe?.(changed);
    if (off) { if (disposed) off(); else cleanups.push(off); }
  } catch { dispose(); return; }
  if (disposed) return;
  window.addEventListener('blur', dispose);
  document.addEventListener('compositionstart', dispose, true);
  cleanups.push(() => window.removeEventListener('blur', dispose), () => document.removeEventListener('compositionstart', dispose, true));
  const canOpen = (id: string) => {
    try { return id === commandID && isCurrent() && source.options().canOpen(commandID); }
    catch { dispose(); return false; }
  };
  return { isCurrent, canOpen, dispose, open(id) {
    if (!canOpen(id)) return false;
    try { return source.options().open(commandID) === true; } catch { return false; } finally { dispose(); }
  } };
}

export function usePagePresentationCommands(options: PagePresentationOptions): { request: (id: PagePresentationCommandID) => boolean } {
  const instanceId = useId();
  const latest = useRef(options);
  latest.current = options;
  const registration = useRef<Source>();
  useLayoutEffect(() => {
    if (registration.current?.root !== options.root.current) {
      if (registration.current) {
        sources.delete(registration.current);
        [...registration.current.listeners].forEach(changed => changed());
      }
      registration.current = options.root.current ? {
        instanceId, root: options.root.current, options: () => latest.current, listeners: new Set(),
      } : undefined;
      if (registration.current) sources.add(registration.current);
    }
    [...(registration.current?.listeners ?? [])].forEach(changed => changed());
  });
  useLayoutEffect(() => () => {
    if (registration.current) {
      sources.delete(registration.current);
      [...registration.current.listeners].forEach(changed => changed());
      registration.current = undefined;
    }
  }, []);
  const request = useCallback((commandID: PagePresentationCommandID) => {
    const event = new CustomEvent(PAGE_PRESENTATION_COMMAND_EVENT, { detail: { commandID, instanceId }, cancelable: true });
    window.dispatchEvent(event);
    return event.defaultPrevented;
  }, [instanceId]);
  return { request };
}
