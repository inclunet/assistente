import { useLayoutEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';
import { cancelInactiveSTTSession } from './sttGate';

export function useVoiceAccessibilityWorkspaceResolver(panelsVisible: boolean) {
  useLayoutEffect(() => {
    const unregister = registerVoiceAccessibilityActiveResolver(
      origin => isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace, panelsVisible),
      origin => {
        const { tabId, terminalSessionId, surfaceType } = origin;
        if (!tabId && !terminalSessionId) return origin.title;
        if (terminalSessionId && surfaceType && surfaceType !== 'terminal') return undefined;
        const expectedType = terminalSessionId ? 'terminal' : surfaceType;
        return useWorkspaceStore.getState().workspace?.tabs.find(tab =>
          (!tabId || tab.id === tabId) && (!expectedType || tab.type === expectedType) &&
          (!terminalSessionId || tab.state?.sessionId === terminalSessionId))?.title;
      },
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
