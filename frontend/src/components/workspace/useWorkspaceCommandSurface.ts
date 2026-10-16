import { useEffect, useLayoutEffect, useRef } from 'react';
import { useCommandContextScope } from '../../lib/commandContextReact';
import type { SurfaceContextGetter } from '../../lib/commandContextProviders';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useOptionalWorkspacePanel } from './WorkspacePanelContext';

/** Registers the rendered panel, never synthesizes a surface from activeTab. */
export function useWorkspaceCommandSurface(
  surfaceType: string,
  getter: SurfaceContextGetter,
  subscribe?: (invalidate: () => void) => () => void,
): void {
  const scope = useCommandContextScope();
  const panel = useOptionalWorkspacePanel();
  const getterRef = useRef(getter);
  useLayoutEffect(() => { getterRef.current = getter; });
  const tabId = panel?.tab.id;
  const root = panel?.rootRef;
  const active = panel?.isActive === true;
  // The panel root ref belongs to an ancestor: its ref can be attached after
  // this child's layout effects when a cached/lazy panel mounts. Register in
  // the passive effect, once all committed DOM refs are available. Until then
  // no lease exists; readers fail closed rather than binding a null root.
  useEffect(() => {
    if (!scope || !root || !tabId || !active) return;
    const read: SurfaceContextGetter = () => {
      const workspace = useWorkspaceStore.getState().workspace;
      const currentTab = workspace?.tabs.find((tab) => tab.id === tabId);
      if (workspace?.activeTabId !== tabId || currentTab?.type !== surfaceType) return null;
      const context = getterRef.current();
      if (!context || context.surfaceId !== tabId || context.surfaceType !== surfaceType) return null;
      return context;
    };
    let mounted = true;
    let unregister = scope.registerSurface(tabId, root, read);
    const unsubscribe = subscribe?.(() => {
      if (!mounted) return;
      // A source notification retires the preparation even after A→B→A.
      // It never substitutes for the synchronous source read at commit.
      unregister();
      unregister = scope.registerSurface(tabId, root, read);
    });
    return () => { mounted = false; unsubscribe?.(); unregister(); };
  }, [scope, root, tabId, active, surfaceType, subscribe]);
}
