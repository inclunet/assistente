import { useLayoutEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver(panelsVisible: boolean) {
  useLayoutEffect(() => {
    return registerVoiceAccessibilityActiveResolver((origin) => (
      (!origin?.terminalSessionId || panelsVisible) && isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace)
    ));
  }, [panelsVisible]);

  useLayoutEffect(() => {
    return useWorkspaceStore.subscribe(
      (state, previousState) => {
        if (state.workspace?.activeTabId !== previousState.workspace?.activeTabId) {
          cancelInactiveSTTSession();
        }
      },
    );
  }, []);
}
