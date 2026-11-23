import { useLayoutEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver(panelsVisible: boolean) {
  useLayoutEffect(() => {
    const unregister = registerVoiceAccessibilityActiveResolver((origin) => (
      isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace, panelsVisible)
    ));
    cancelInactiveSTTSession();
    return unregister;
  }, [panelsVisible]);

  useLayoutEffect(() => {
    return useWorkspaceStore.subscribe(
      (state, previousState) => {
        if (state.workspace !== previousState.workspace) {
          cancelInactiveSTTSession();
        }
      },
    );
  }, []);
}
