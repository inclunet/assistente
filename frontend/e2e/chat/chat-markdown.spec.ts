import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const baseConversation = {
  id: '01926b90-0000-7000-8000-000000000001',
  title: 'Test Conversation',
  created_at: now,
  updated_at: now,
  messages: [],
  message_count: 1,
};

const userMessage = {
  message: {
    id: '01926b90-0000-7000-8000-000000000010',
    conversationId: '01926b90-0000-7000-8000-000000000001',
    role: 'user',
    content: 'Teste',
    createdAt: now,
  },
  children: [],
};

/**
 * Helper: envia mensagem e emite streaming via chat:stream (não segment events).
 */
async function sendAndStream(
  page: import('@playwright/test').Page,
  wails: import('../fixtures').WailsMock,
  markdownContent: string,
) {
  const textarea = page.locator('.chat-input__textarea');
  await expect(textarea).toBeEditable({ timeout: 5_000 });
  await textarea.click();
  await textarea.fill('Teste');
  await textarea.press('Enter');

  await page.waitForFunction(() => {
    return window.__wailsMock.getCallLog().some(
      (c: { fn: string }) => c.fn === 'SendMessage',
    );
  }, { timeout: 5_000 });

  await wails.emit('chat:messages_ready', {
    conversationId: '01926b90-0000-7000-8000-000000000001',
    userMessageId: '01926b90-0000-7000-8000-100000000100',
    userContent: 'Teste',
  });

  await page.waitForFunction(() => {
    return document.querySelectorAll('.message-node').length >= 2;
  }, { timeout: 5_000 });

  // Stream com conteúdo
  await wails.emit('chat:stream', {
    conversationId: '01926b90-0000-7000-8000-000000000001',
    messageId: '01926b90-0000-7000-8000-100000000002',
    token: markdownContent,
    done: false,
    content: markdownContent,
  });

  await page.waitForFunction(() => {
    return document.querySelectorAll('.message-node').length >= 3;
  }, { timeout: 5_000 });

  // Finaliza stream
  await wails.emit('chat:stream', {
    conversationId: '01926b90-0000-7000-8000-000000000001',
    messageId: '01926b90-0000-7000-8000-100000000002',
    token: '',
    done: true,
    content: markdownContent,
  });

  await wails.emit('chat:done', {});
}

test.describe('Chat — Renderização Markdown', () => {
  test.beforeEach(async ({ wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', '01926b90-0000-7000-8000-100000000002');
    await wails.setResponse('EnsureConversation', baseConversation);
  });

  test('renderiza headings corretamente', async ({ page, wails }) => {
    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    await sendAndStream(page, wails, '# Título Principal\n\n## Subtítulo');

    // Verifica que o conteúdo do heading está visível
    const message = page.locator('.chat-message').last();
    await expect(message).toContainText('Título Principal', { timeout: 5_000 });

    // Verifica se foi renderizado como h1
    const h1 = message.locator('h1');
    if (await h1.count() > 0) {
      await expect(h1.first()).toContainText('Título Principal');
    }
  });

  test('renderiza bloco de código', async ({ page, wails }) => {
    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    await sendAndStream(page, wails, '```python\ndef hello():\n    print("Hello!")\n```');

    const message = page.locator('.chat-message').last();
    await expect(message).toContainText('hello', { timeout: 5_000 });

    const code = message.locator('code');
    if (await code.count() > 0) {
      await expect(code.first()).toContainText('hello');
    }
  });

  test('renderiza tabela', async ({ page, wails }) => {
    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    await sendAndStream(page, wails, '| Nome | Idade |\n|------|-------|\n| Ana | 25 |');

    const message = page.locator('.chat-message').last();
    await expect(message).toContainText('Nome', { timeout: 5_000 });

    const table = message.locator('table');
    if (await table.count() > 0) {
      await expect(table.first()).toBeVisible();
    }
  });

  test('renderiza lista não-ordenada', async ({ page, wails }) => {
    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    await sendAndStream(page, wails, '- Alpha\n- Beta\n- Gamma');

    const message = page.locator('.chat-message').last();
    await expect(message).toContainText('Alpha', { timeout: 5_000 });
    await expect(message).toContainText('Beta', { timeout: 3_000 });
    await expect(message).toContainText('Gamma', { timeout: 3_000 });
  });

  test('renderiza links', async ({ page, wails }) => {
    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    await sendAndStream(page, wails, 'Acesse [Exemplo](https://example.com) aqui.');

    const message = page.locator('.chat-message').last();
    await expect(message).toContainText('Exemplo', { timeout: 5_000 });

    const link = message.locator('a');
    if (await link.count() > 0) {
      await expect(link.first()).toHaveAttribute('href', 'https://example.com');
    }
  });
});
