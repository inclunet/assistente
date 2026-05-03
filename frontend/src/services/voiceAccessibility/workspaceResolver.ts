import { useEffect } from 'react';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { isVoiceAccessibilityOriginActive } from './types';
import { registerVoiceAccessibilityActiveResolver } from './announcerBroker';

export function useVoiceAccessibilityWorkspaceResolver() {
  useEffect(() => registerVoiceAccessibilityActiveResolver((origin) => (
    isVoiceAccessibilityOriginActive(origin, useWorkspaceStore.getState().workspace)
  )), []);
}
