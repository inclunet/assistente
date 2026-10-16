/** Runtime transport. Generated bindings remain owned by the official generator. */
export interface ChatEditorPlan {
  readonly tabId: string;
  readonly draftId?: string;
  readonly filePath?: string;
}
interface ChatEditorAPI {
  PrepareChatEditorCommand(ticket: string, messageId: string, originalContent: string, targetDocumentId: string): Promise<ChatEditorPlan>;
  OpenChatEditorCommand(ticket: string, handoffId: string, title: string): Promise<ChatEditorPlan>;
  ValidateChatEditorCommand(ticket: string, handoffId: string): Promise<void>;
}
function api(): ChatEditorAPI {
  const host = (window as Window & { go?: { app?: { App?: Partial<ChatEditorAPI> } } }).go?.app?.App;
  if (!host || typeof host.PrepareChatEditorCommand !== 'function' || typeof host.OpenChatEditorCommand !== 'function' ||
      typeof host.ValidateChatEditorCommand !== 'function') throw new Error('Chat editor command API is unavailable');
  return host as ChatEditorAPI;
}
function plan(value: ChatEditorPlan): ChatEditorPlan {
  if (!value || typeof value.tabId !== 'string' || !value.tabId.trim()) throw new Error('Invalid editor destination');
  return Object.freeze({ tabId: value.tabId, draftId: value.draftId, filePath: value.filePath });
}
export async function prepareChatEditorCommand(ticket: string, messageId: string, originalContent: string, targetDocumentId: string) {
  return plan(await api().PrepareChatEditorCommand(ticket, messageId, originalContent, targetDocumentId));
}
export async function openChatEditorCommand(ticket: string, handoffId: string, title: string) {
  return plan(await api().OpenChatEditorCommand(ticket, handoffId, title));
}
export async function validateChatEditorCommand(ticket: string, handoffId: string) {
  await api().ValidateChatEditorCommand(ticket, handoffId);
}
