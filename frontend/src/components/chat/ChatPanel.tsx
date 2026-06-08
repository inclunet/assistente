import { ChatSessionView } from './ChatSessionView';
import type { ChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import type {
  ChatSurfaceSendContext,
  ChatSurfaceSendHandler,
} from './ChatSurfaceController';

export type ChatPanelSendContext = ChatSurfaceSendContext;
export type ChatPanelSendHandler = ChatSurfaceSendHandler;

export interface ChatPanelProps {
  surface: ChatSurfaceIdentity;
  onSend: ChatPanelSendHandler;
  showShortcutsHelp?: boolean;
}

export function ChatPanel({
  surface,
  onSend,
  showShortcutsHelp,
}: ChatPanelProps) {
  const variant = surface.surfaceType === 'embedded' || surface.surfaceType === 'modal'
    ? 'embedded'
    : 'page';

  return (
    <ChatSessionView
      variant={variant}
      surface={surface}
      onSend={(content, mediaFiles, origin) => onSend(content, mediaFiles, {
        conversationId: surface.conversationId || origin.conversationId || null,
        origin,
      })}
      showShortcutsHelp={showShortcutsHelp}
    />
  );
}
