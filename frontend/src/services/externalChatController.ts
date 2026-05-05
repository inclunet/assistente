import {
  startChatEventController,
  type ChatEventControllerAdapter,
} from './chatEventController';
import {
  createChatSurfaceIdentity,
  createChatSurfaceOrigin,
  type ChatSurfaceOrigin,
} from './chatSessionRegistry';

export interface ExternalChatIncomingData {
  channel: string;
  from: string;
  fromId?: string;
  text: string;
  conversationId: string;
  newConversation?: boolean;
}

export interface ExternalChatControllerAdapter {
  hasConversationSession: (conversationId: string) => boolean;
  loadConversationSession: (conversationId: string) => Promise<void>;
  enqueueExternalTurn: (
    conversationId: string,
    sessionKey: string,
    task: () => Promise<void>,
  ) => Promise<void>;
  chatEventAdapter: ChatEventControllerAdapter;
}

function buildExternalSurfaceOrigin(data: ExternalChatIncomingData): ChatSurfaceOrigin {
  const participant = data.fromId || data.from || 'unknown';
  const identity = createChatSurfaceIdentity({
    conversationId: data.conversationId,
    surfaceId: `external:${data.channel}:${participant}`,
    surfaceType: 'external',
  });
  return createChatSurfaceOrigin(identity);
}

export async function handleExternalChatIncoming(
  data: ExternalChatIncomingData,
  adapter: ExternalChatControllerAdapter,
) {
  const { channel, from, text, conversationId } = data;
  if (!conversationId) return;
  const origin = buildExternalSurfaceOrigin(data);

  if (!adapter.hasConversationSession(conversationId)) {
    await adapter.loadConversationSession(conversationId);
  }

  await adapter.enqueueExternalTurn(conversationId, origin.sessionKey, async () => {
    const controller = startChatEventController({
      conversationId,
      external: { channel, from, text },
      origin,
      adapter: adapter.chatEventAdapter,
    });
    await controller.done;
  });
}
