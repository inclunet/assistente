import { test, expect } from '../fixtures';

const conversationId = '01926b90-0000-7000-8000-000000000001';
const messageId = '01926b90-0000-7000-8000-100000000001';

function pinnedMessage(pinned: boolean) {
  return {
    id: messageId,
    conversationId,
    role: 'assistant',
    content: 'Resposta importante para fixar',
    pinned,
    createdAt: new Date().toISOString(),
    timestamp: Date.now(),
    isStreaming: false,
    internal: false,
  };
}

test.describe('Chat — fixação de mensagens', () => {
  test('fixa pelo menu de teclado e lista na conversa', async ({ page, wails }) => {
    await wails.setResponse('GetConversationInfo', {
      id: conversationId,
      title: 'Conversa',
      created_at: new Date().toISOString(),
      updated_at: new Date().toISOString(),
    });
    await wails.setResponse('GetConversationMessageWindow', {
      scope: 'conversation',
      conversationId,
      nodes: [{ message: pinnedMessage(false), children: [], childCount: 0, originalIndex: 0 }],
      totalCount: 1,
      startIndex: 0,
      endIndex: 0,
      hasBefore: false,
      hasAfter: false,
    });
    await wails.setResponse('ToggleMessagePin', pinnedMessage(true));
    await wails.setResponse('GetPinnedMessages', [pinnedMessage(true)]);
    await wails.waitForApp();

    const message = page.locator(`[data-message-id="${messageId}"]`);
    await expect(message).toBeVisible();
    await message.focus();
    await page.keyboard.press('Shift+F10');
    await page.getByRole('menuitem', { name: /Fixar mensagem|Pin message/i }).click();

    await wails.emit('message:pin_changed', { conversationId, messageId, pinned: true });
    await expect(message.getByText(/Mensagem fixada|Pinned message/i)).toBeVisible();

    await page.getByRole('button', { name: /Mensagens fixadas|Pinned messages/i }).click();
    await expect(page.getByRole('dialog')).toContainText('Resposta importante para fixar');
  });
});
