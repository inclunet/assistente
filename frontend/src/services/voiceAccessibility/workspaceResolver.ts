import { useLayoutEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver(panelsVisible: boolean) {
  useLayoutEffect(() => {
    const unregisterResolver = registerVoiceAccessibilityActiveResolver((origin) => (
      (!origin?.terminalSessionId || panelsVisible) && isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace)
    ));
    const unsubscribeWorkspace = useWorkspaceStore.subscribe(
      (state, previousState) => {
        if (state.workspace?.activeTabId !== previousState.workspace?.activeTabId) {
          cancelInactiveSTTSession();
        }
      },
    );

    return () => {
      unsubscribeWorkspace();
      unregisterResolver();
    };
  }, [panelsVisible]);
}
