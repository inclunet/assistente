import type { Page } from '@playwright/test';
import type { WailsMock } from '../fixtures';

export const chatClearCommand = 'chat.conversation.clear';
export const chatClearTicket = 'e2e-chat-clear-ticket';
export const chatClearInvocation = '01926b90-0000-7000-8000-000000000101';
export const chatClearHandoff = 'e2e-chat-clear-handoff';
export const chatClearShortcut = { version: 1, code: 'KeyL', modifiers: ['Control'] };
export const chatClearMapGeneration = 'e2e-chat-clear-map-v1';

/** Install map and protocol responses before app startup so the keyboard map is loaded. */
export async function configureChatClearCommand(wails: WailsMock): Promise<void> {
  await wails.setResponse('GetLocalCommandKeyboardMap', {
    generation: chatClearMapGeneration,
    ownerId: 'user-e2e',
    sessionId: 'session-e2e',
    workspaceId: 'ws-1',
    bindings: [
      { shortcut: { version: 1, code: 'KeyM', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' },
      { shortcut: chatClearShortcut, commandId: chatClearCommand, handler: 'contextual' },
    ],
    localPaletteCommands: [],
  });
  await wails.setResponse('BeginLocalCommandUIKey', {
    ticket: chatClearTicket,
    invocationId: chatClearInvocation,
    commandId: chatClearCommand,
  });
  await wails.setResponse('TakeUICommand', {
    ticket: chatClearTicket,
    handoffId: chatClearHandoff,
    invocationId: chatClearInvocation,
    commandId: chatClearCommand,
  });
  await wails.setResponse('GetUICommandResult', {
    invocationId: chatClearInvocation,
    commandId: chatClearCommand,
    status: 'succeeded',
  });
}

/** Install after app startup; the real Ctrl+L invocation commits and emits the clear event. */
export async function installChatClearCommitCallback(page: Page, conversationId: string): Promise<void> {
  await page.evaluate(({ ticket, handoff, conversationId: targetConversationId }) => {
    window.__wailsMock.setResponse('CommitWorkspaceTabCommand', (receivedTicket: string, receivedHandoff: string) => {
      if (receivedTicket !== ticket || receivedHandoff !== handoff) {
        throw new Error('chat-clear-invalid-handoff');
      }
      window.__wailsMock.emit('conversation:cleared', { conversation_id: targetConversationId });
    });
  }, { ticket: chatClearTicket, handoff: chatClearHandoff, conversationId });
}

export async function waitForChatClearCommit(page: Page): Promise<void> {
  await page.waitForFunction(({ ticket, handoff }) => window.__wailsMock.getCallLog().some(call =>
    call.fn === 'CommitWorkspaceTabCommand' && call.args[0] === ticket && call.args[1] === handoff,
  ), { ticket: chatClearTicket, handoff: chatClearHandoff }, { timeout: 5_000 });
  await page.waitForFunction(({ ticket }) => window.__wailsMock.getCallLog().some(call =>
    call.fn === 'GetUICommandResult' && call.args[0] === ticket,
  ), { ticket: chatClearTicket }, { timeout: 5_000 });
}
