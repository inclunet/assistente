import { useEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver() {
  useEffect(() => {
    const unregisterResolver = registerVoiceAccessibilityActiveResolver((origin) => (
      isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace)
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
  }, []);
}
