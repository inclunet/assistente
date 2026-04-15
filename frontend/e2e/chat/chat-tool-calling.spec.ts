import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const toolCallsJson = JSON.stringify([
  {
    id: 'tc-1',
    type: 'function',
    function: {
      name: 'search_web',
      arguments: '{"query":"clima hoje"}',
    },
    result: '{"results":["Ensolarado, 25°C"]}',
  },
]);

const messagesWithToolCalls = [
  {
    message: {
      id: 1,
      conversationId: 1,
      role: 'user',
      content: 'Como está o clima?',
      createdAt: now,
    },
    children: [],
  },
  {
    message: {
      id: 2,
      conversationId: 1,
      role: 'assistant',
      content: 'O clima hoje está ensolarado, 25°C.',
      createdAt: now,
      toolCalls: toolCallsJson,
    },
    children: [],
  },
];

test.describe('Chat — tool calls (histórico)', () => {
  test('exibe a seção de tool calls em mensagem com toolCalls', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible();
  });

  test('header da seção de tool calls mostra nome da ferramenta', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    const title = page.locator('.tool-calls-section__title');
    await expect(title).toContainText('search_web');
  });

  test('seção de tool calls é expansível via clique', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    const header = page.locator('.tool-calls-section__header');
    await expect(header).toHaveAttribute('aria-expanded', 'false');

    await header.click();
    await expect(header).toHaveAttribute('aria-expanded', 'true');

    const content = page.locator('.tool-calls-section__content');
    await expect(content).toBeVisible();
  });

  test('seção expandida mostra argumentos da ferramenta', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    // Expande
    await page.locator('.tool-calls-section__header').click();

    const args = page.locator('.tool-calls-section__args');
    await expect(args).toBeVisible();
    await expect(args).toContainText('clima hoje');
  });

  test('seção expandida mostra resultado da ferramenta', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    await page.locator('.tool-calls-section__header').click();

    const result = page.locator('.tool-calls-section__result-content');
    await expect(result).toBeVisible();
    await expect(result).toContainText('Ensolarado');
  });

  test('seção de tool calls tem aria-expanded e role=region corretos', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesWithToolCalls);
    await wails.waitForApp();

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    const header = page.locator('.tool-calls-section__header');
    await expect(header).toHaveAttribute('aria-expanded');

    // Expande e verifica region
    await header.click();
    const region = page.locator('.tool-calls-section__content[role="region"]');
    await expect(region).toBeVisible();
  });
});

test.describe('Chat — tool calls (streaming)', () => {
  test('exibe tool call em execução durante streaming', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [
      {
        message: {
          id: 1,
          conversationId: 1,
          role: 'user',
          content: 'Pesquise algo',
          createdAt: now,
        },
        children: [],
      },
    ]);
    await wails.setResponse('SendMessage', 2);

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    // Envia mensagem para iniciar o fluxo
    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Pesquise o clima');
    await textarea.press('Enter');

    // Backend confirma a mensagem do usuário
    await wails.emit('chat:messages_ready', {
      conversationId: 1,
      userMessageId: 100,
      userContent: 'Pesquise o clima',
    });

    // Simula início de streaming
    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
      content: '',
      done: false,
    });

    // Simula início de tool call
    await wails.emit('chat:tool_start', {
      conversationId: 1,
      name: 'search_web',
      callId: 'tc-live-1',
      args: '{"query":"clima"}',
    });

    // Aguarda a seção de tool calls aparecer
    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible({ timeout: 5_000 });
  });

  test('tool call muda de running para done durante streaming', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [
      {
        message: {
          id: 1,
          conversationId: 1,
          role: 'user',
          content: 'Pesquise algo',
          createdAt: now,
        },
        children: [],
      },
    ]);
    await wails.setResponse('SendMessage', 2);

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Pesquise o clima');
    await textarea.press('Enter');

    // Backend confirma a mensagem do usuário
    await wails.emit('chat:messages_ready', {
      conversationId: 1,
      userMessageId: 100,
      userContent: 'Pesquise o clima',
    });

    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
      content: '',
      done: false,
    });

    await wails.emit('chat:tool_start', {
      conversationId: 1,
      name: 'search_web',
      callId: 'tc-live-2',
      args: '{"query":"clima"}',
    });

    await page.waitForSelector('.tool-calls-section', { timeout: 5_000 });

    // Verifica que a seção mostra estado de running
    const runningSection = page.locator('.tool-calls-section--running');
    await expect(runningSection).toBeVisible({ timeout: 3_000 });

    // Finaliza o tool call (muda de running para done)
    await wails.emit('chat:tool_end', {
      conversationId: 1,
      callId: 'tc-live-2',
      name: 'search_web',
      status: 'success',
      summary: 'Encontrado: Ensolarado',
    });

    // Após tool_end, a seção deve ainda estar visível (tool call done, streaming continua)
    await expect(page.locator('.tool-calls-section')).toBeVisible({ timeout: 3_000 });
  });
});
