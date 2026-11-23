import { useLayoutEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver(panelsVisible: boolean) {
  useLayoutEffect(() => {
    const unregister = registerVoiceAccessibilityActiveResolver(
      origin => isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace, panelsVisible),
      tabId => useWorkspaceStore.getState().workspace?.tabs.find(tab => tab.id === tabId && tab.type === 'terminal')?.title,
    );
    cancelInactiveSTTSession();
    return unregister;
  }, [panelsVisible]);

  useLayoutEffect(() => {
    return useWorkspaceStore.subscribe(
      (state, previousState) => {
        // A aba selecionada também pode trocar de sessão sem mudar de ID.
        if (state.workspace !== previousState.workspace) {
          cancelInactiveSTTSession();
        }
      },
    );
  }, []);
}
