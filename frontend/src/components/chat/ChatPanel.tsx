import { ChatSessionView } from './ChatSessionView';
import type { ChatToolbarConversationChangeHandler } from './ChatToolbar';
import type { ChatSurfaceIdentity } from '../../services/chatSessionRegistry';
import type {
  ChatSurfaceSendContext,
  ChatSurfaceSendHandler,
} from './ChatSurfaceController';
import { useWorkspaceStore } from '../../store/workspaceStore';

export type ChatPanelSendContext = ChatSurfaceSendContext;
export type ChatPanelSendHandler = ChatSurfaceSendHandler;

/**
 * Solicitação de troca de conversa originada no HistoryPicker do toolbar. A
 * superfície de chat é "controlada": ela não decide o que trocar a conversa
 * significa — apenas notifica o dono (página, modal, etc.), que reage atualizando
 * a identidade da superfície que passa para baixo (ex.: `tab.conversation_id` na
 * página; `setBoundConversation` no modal embutido).
 *
 * Alias do tipo do `ChatToolbar` (origem do contrato) para não driftar.
 */
export type ChatPanelConversationChangeHandler = ChatToolbarConversationChangeHandler;

export interface ChatPanelProps {
  surface: ChatSurfaceIdentity;
  onSend: ChatPanelSendHandler;
  onRequestConversationChange?: ChatPanelConversationChangeHandler;
  showShortcutsHelp?: boolean;
  profileSlug?: string;
}

export function ChatPanel({
  surface,
  onSend,
  onRequestConversationChange,
  showShortcutsHelp,
  profileSlug,
}: ChatPanelProps) {
  const workspaceTabs = useWorkspaceStore((s) => s.workspace?.tabs);
  const workspaceProfile = useWorkspaceStore((s) => s.workspace?.profile);
  const variant = surface.surfaceType === 'embedded' || surface.surfaceType === 'modal'
    ? 'embedded'
    : 'page';
  const surfaceTab = surface.tabId
    ? workspaceTabs?.find((tab) => tab.id === surface.tabId)
    : undefined;
  const effectiveProfileSlug = profileSlug
    || (surfaceTab?.profileOverride?.slug as string | undefined)
    || workspaceProfile
    || undefined;

  return (
    <ChatSessionView
      variant={variant}
      surface={surface}
      onSend={(content, mediaFiles, origin) => onSend(content, mediaFiles, {
        conversationId: surface.conversationId || origin.conversationId || null,
        origin,
      })}
      onRequestConversationChange={onRequestConversationChange}
      showShortcutsHelp={showShortcutsHelp}
      profileSlug={effectiveProfileSlug}
    />
  );
}
