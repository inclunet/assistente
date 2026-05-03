import { ChatSessionView } from './ChatSessionView';
import type { MediaFile } from '../../services/mediaService';
import type { ChatSurfaceOrigin, ChatSurfaceType } from '../../services/chatSessionRegistry';

export interface ChatPanelSendContext {
  conversationId: string | null;
  origin: ChatSurfaceOrigin;
}

export type ChatPanelSendHandler = (
  content: string,
  mediaFiles: MediaFile[] | undefined,
  context: ChatPanelSendContext,
) => Promise<void>;

export interface ChatPanelProps {
  conversationId?: string | null;
  surfaceId?: string;
  sessionKey?: string;
  surfaceType?: ChatSurfaceType;
  onSend: ChatPanelSendHandler;
  showShortcutsHelp?: boolean;
}

export function ChatPanel({
  conversationId = null,
  surfaceId,
  sessionKey,
  surfaceType = 'page',
  onSend,
  showShortcutsHelp,
}: ChatPanelProps) {
  const variant = surfaceType === 'embedded' || surfaceType === 'modal'
    ? 'embedded'
    : 'page';

  return (
    <ChatSessionView
      variant={variant}
      surfaceType={surfaceType}
      conversationId={conversationId}
      surfaceId={surfaceId}
      sessionKey={sessionKey}
      onSend={(content, mediaFiles, origin) => onSend(content, mediaFiles, {
        conversationId: conversationId || origin.conversationId || null,
        origin,
      })}
      showShortcutsHelp={showShortcutsHelp}
    />
  );
}
