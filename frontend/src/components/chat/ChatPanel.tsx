import { ChatSessionView } from './ChatSessionView';
import { useMemo } from 'react';
import {
  createChatSurfaceIdentity,
  type ChatSurfaceIdentity,
  type ChatSurfaceType,
} from '../../services/chatSessionRegistry';
import type {
  ChatSurfaceSendContext,
  ChatSurfaceSendHandler,
} from './ChatSurfaceController';
import { useWorkspacePanel } from '../workspace/WorkspacePanelContext';

export type ChatPanelSendContext = ChatSurfaceSendContext;
export type ChatPanelSendHandler = ChatSurfaceSendHandler;

export interface ChatPanelProps {
  surface?: ChatSurfaceIdentity;
  conversationId?: string | null;
  surfaceId?: string;
  sessionKey?: string;
  surfaceType?: ChatSurfaceType;
  onSend: ChatPanelSendHandler;
  showShortcutsHelp?: boolean;
}

export function ChatPanel({
  surface,
  conversationId = null,
  surfaceId,
  sessionKey,
  surfaceType = 'page',
  onSend,
  showShortcutsHelp,
}: ChatPanelProps) {
  const { tab } = useWorkspacePanel();
  const surfaceIdentity = useMemo(() => surface ?? createChatSurfaceIdentity({
    conversationId: conversationId || null,
    sessionKey,
    surfaceId,
    surfaceType,
    tabId: tab.id,
  }), [conversationId, sessionKey, surface, surfaceId, surfaceType, tab.id]);
  const variant = surfaceIdentity.surfaceType === 'embedded' || surfaceIdentity.surfaceType === 'modal'
    ? 'embedded'
    : 'page';

  return (
    <ChatSessionView
      variant={variant}
      surface={surfaceIdentity}
      onSend={(content, mediaFiles, origin) => onSend(content, mediaFiles, {
        conversationId: surfaceIdentity.conversationId || origin.conversationId || null,
        origin,
      })}
      showShortcutsHelp={showShortcutsHelp}
    />
  );
}
