import type { Message } from './chatMessageTree';
import { isBackendId } from './idUtils';

export function canContinueChatMessage(message: Message | undefined): message is Message {
  const item = message?.timelineItem;
  return !!message && !!item && item.scopeParentId === null
    && item.durability === 'persisted'
    && (item.state === 'failed' || item.state === 'interrupted') && !!item.metadata.terminalReason
    && isBackendId(item.representativeMessageId) && isBackendId(item.turnId)
    && message.role === 'assistant' && !message.isStreaming && !!message.content.trim();
}
