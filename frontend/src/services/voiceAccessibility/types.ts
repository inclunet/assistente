import type { WorkspaceData, WorkspaceTab } from '../../store/workspaceStore';

export type VoiceAccessibilitySurfaceType = WorkspaceTab['type'] | 'page' | 'embedded' | 'modal' | 'external';

export type VoiceAccessibilityPriority =
  | 'manual-active'
  | 'automatic-active'
  | 'automatic-inactive'
  | 'system'
  | 'critical';

export type VoiceAccessibilityEventType =
  | 'user-action'
  | 'progress'
  | 'completion'
  | 'error'
  | 'system';

export interface VoiceAccessibilityOrigin {
  tabId?: string;
  surfaceId?: string;
  sessionKey?: string;
  conversationId?: string;
  surfaceType?: VoiceAccessibilitySurfaceType;
  profileSlug?: string | null;
  title?: string;
  isExternal?: boolean;
}

export interface VoiceAccessibilityRequestBase {
  origin?: VoiceAccessibilityOrigin;
  priority?: VoiceAccessibilityPriority;
  eventType?: VoiceAccessibilityEventType;
}

export function buildVoiceAccessibilityOriginFromTab(tab: WorkspaceTab, workspace?: WorkspaceData | null): VoiceAccessibilityOrigin {
  return {
    tabId: tab.id,
    surfaceId: tab.id,
    conversationId: tab.conversationId,
    surfaceType: tab.type,
    profileSlug: (tab.profileOverride?.slug as string | undefined) ?? workspace?.profile ?? null,
    title: tab.title,
  };
}

export function isVoiceAccessibilityOriginActive(
  origin: VoiceAccessibilityOrigin | undefined,
  workspace: WorkspaceData | null | undefined,
): boolean {
  if (!origin || origin.isExternal) return true;
  if (!origin.tabId) return true;
  return workspace?.activeTabId === origin.tabId;
}

export function getVoiceAccessibilityOriginLabel(origin: VoiceAccessibilityOrigin | undefined): string | null {
  if (!origin) return null;
  return origin.title || origin.surfaceId || origin.tabId || origin.conversationId || null;
}

export function getVoiceAccessibilityPriority(isActive: boolean, manual = false): VoiceAccessibilityPriority {
  if (manual && isActive) return 'manual-active';
  return isActive ? 'automatic-active' : 'automatic-inactive';
}
