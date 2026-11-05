import type { RecoveryItemIdentity } from '../services/timelineRecoveryItem';
/** Runtime adapter, kept outside generated bindings. Regenerate the official
 * Wails bindings before release; absence of either endpoint fails closed. */
interface ChatMessageCommandWindow extends Window {
  go?: { app?: { App?: {
    PrepareChatTimelineCopyCommand?: (ticket: string, windowId: string, identity: RecoveryItemIdentity) => Promise<void>;
    PrepareChatMessageCommand?: (ticket: string, messageID: string) => Promise<void>;
    PrepareChatMessageEditCommand?: (ticket: string, messageID: string, originalContent: string, content: string) => Promise<void>;
    CommitChatMessageCommand?: (ticket: string, handoffID: string) => Promise<void>;
  } } };
}
export async function prepareChatMessageCommand(ticket: string, messageID: string): Promise<void> {
  const api = (window as ChatMessageCommandWindow).go?.app?.App;
  if (typeof api?.PrepareChatMessageCommand !== 'function') throw new Error('Chat message command API is unavailable');
  await api.PrepareChatMessageCommand(ticket, messageID);
}
export async function commitChatMessageCommand(ticket: string, handoffID: string): Promise<void> {
  const api = (window as ChatMessageCommandWindow).go?.app?.App;
  if (typeof api?.CommitChatMessageCommand !== 'function') throw new Error('Chat message command API is unavailable');
  await api.CommitChatMessageCommand(ticket, handoffID);
}
export async function prepareChatMessageEditCommand(ticket: string, messageID: string, originalContent: string, content: string): Promise<void> {
  const api = (window as ChatMessageCommandWindow).go?.app?.App;
  if (typeof api?.PrepareChatMessageEditCommand !== 'function') throw new Error('Chat message edit command API is unavailable');
  await api.PrepareChatMessageEditCommand(ticket, messageID, originalContent, content);
}

export async function prepareChatTimelineCopyCommand(ticket: string, target: { windowId: string; identity: RecoveryItemIdentity }): Promise<void> {
  const api = (window as ChatMessageCommandWindow).go?.app?.App;
  if (typeof api?.PrepareChatTimelineCopyCommand !== 'function') throw new Error('Timeline copy command API is unavailable');
  await api.PrepareChatTimelineCopyCommand(ticket, target.windowId, target.identity);
}
