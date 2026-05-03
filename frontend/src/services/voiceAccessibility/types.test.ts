import { describe, expect, it } from 'vitest';
import type { WorkspaceData, WorkspaceTab } from '../../store/workspaceStore';
import {
  buildVoiceAccessibilityOriginFromTab,
  getVoiceAccessibilityOriginLabel,
  getVoiceAccessibilityPriority,
  isVoiceAccessibilityOriginActive,
} from './types';

const chatTab: WorkspaceTab = {
  id: 'tab-chat',
  type: 'chat',
  title: 'Chat Principal',
  position: 0,
  conversationId: 'conv-1',
  profileOverride: { slug: 'perfil-tab' },
};

const workspace: WorkspaceData = {
  id: 'workspace-1',
  name: 'Workspace',
  profile: 'perfil-workspace',
  tabs: [chatTab],
  activeTabId: 'tab-chat',
};

describe('voiceAccessibility types', () => {
  it('cria origem a partir de uma aba do workspace', () => {
    expect(buildVoiceAccessibilityOriginFromTab(chatTab, workspace)).toEqual({
      tabId: 'tab-chat',
      surfaceId: 'tab-chat',
      conversationId: 'conv-1',
      surfaceType: 'chat',
      profileSlug: 'perfil-tab',
      title: 'Chat Principal',
    });
  });

  it('detecta origem ativa e inativa pelo workspace', () => {
    expect(isVoiceAccessibilityOriginActive({ tabId: 'tab-chat' }, workspace)).toBe(true);
    expect(isVoiceAccessibilityOriginActive({ tabId: 'tab-other' }, workspace)).toBe(false);
  });

  it('considera origem externa e origem sem tab como ativa', () => {
    expect(isVoiceAccessibilityOriginActive({ isExternal: true, tabId: 'tab-other' }, workspace)).toBe(true);
    expect(isVoiceAccessibilityOriginActive({ conversationId: 'conv-1' }, workspace)).toBe(true);
  });

  it('resolve label e prioridade', () => {
    expect(getVoiceAccessibilityOriginLabel({ title: 'Editor' })).toBe('Editor');
    expect(getVoiceAccessibilityOriginLabel({ tabId: 'tab-1' })).toBe('tab-1');
    expect(getVoiceAccessibilityPriority(true, true)).toBe('manual-active');
    expect(getVoiceAccessibilityPriority(false)).toBe('automatic-inactive');
  });
});
