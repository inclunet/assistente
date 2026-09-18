import { test, expect } from '../fixtures';

test('anuncia ferramenta pendente antes do fim e depois sua conclusão', async ({ page, wails }) => {
  const conversationId = '01926b90-0000-7000-8000-000000000001';
  const userMessageId = '01968740-1234-7000-8000-000000000002';
  const assistantMessageId = '01968740-1234-7000-8000-000000000003';
  const createdAt = new Date().toISOString();
  await wails.setResponse('GetMessages', [{
    message: { id: userMessageId, conversationId, role: 'user', content: 'Olá', createdAt },
    children: [],
  }]);
  await wails.setResponse('SendMessage', assistantMessageId);
  await wails.setResponse('EnsureConversation', { id: conversationId, title: 'Progresso acessível', created_at: createdAt, updated_at: createdAt, messages: [], message_count: 1 });
  await wails.waitForApp();
  const input = page.locator('.chat-input__textarea');
  await expect(input).toBeEditable();
  await input.fill('Consulte a fonte');
  await input.press('Enter');
  await page.waitForFunction(() => window.__wailsMock.getCallLog().some((call: { fn: string }) => call.fn === 'SendMessage'));
  const identity = { conversationId, turnId: userMessageId, assistantMessageId, callId: 'pending-call', name: 'consulta_demora' };
  await wails.emit('chat:tool_start', identity);
  const status = page.locator('.sr-announcer [role="status"]');
  await expect(status).toContainText('consulta_demora');
  await expect(page.locator('.tool-calls-section--running')).toContainText(/Ferramentas em execução|Tools running|Herramientas en ejecución/);
  await expect(input).toBeFocused();
  const runningAnnouncement = await status.textContent();

  await wails.emit('chat:tool_end', { ...identity, status: 'ok', summary: 'Consulta concluída' });
  await expect(status).toContainText('consulta_demora');
  await expect(status).not.toHaveText(runningAnnouncement ?? '');
  await expect(input).toBeFocused();
  await expect(page.locator('.sr-announcer')).toHaveCount(1);
});
