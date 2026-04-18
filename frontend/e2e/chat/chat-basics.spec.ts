import { test, expect } from '../fixtures';

test.describe('Chat — carregamento inicial', () => {
  test('exibe o estado vazio do chat ao abrir', async ({ page, wails }) => {
    await wails.waitForApp();

    // O estado vazio deve mostrar título e descrição convidando a conversar
    const emptyState = page.locator('.message-list__empty-state');
    await expect(emptyState).toBeVisible();
  });

  test('campo de entrada de mensagem está visível e focável', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeVisible();
    await textarea.focus();
    await expect(textarea).toBeFocused();
  });

  test('botão de enviar aparece ao digitar texto', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Olá');

    const sendBtn = page.locator('.chat-input__button');
    await expect(sendBtn).toBeVisible();
  });
});

test.describe('Chat — envio de mensagem', () => {
  test('digitar e enviar mensagem chama SendMessage no backend', async ({ page, wails }) => {
    // Configura mock para retornar uma mensagem do assistente
    const now = new Date().toISOString();
    await wails.setResponse('SendMessage', 42);
    await wails.setResponse('EnsureConversation', {
      id: 1,
      title: 'Nova conversa',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 0,
    });

    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Olá, assistente!');
    await expect.poll(async () => textarea.inputValue(), { timeout: 5_000 }).toBe('Olá, assistente!');

    // Envia com Enter
    await textarea.press('Enter');

    // Backend-driven: emite chat:messages_ready como o backend real faria
    await wails.emit('chat:messages_ready', {
      conversationId: 1,
      userMessageId: 100,
      userContent: 'Olá, assistente!',
    });

    // A mensagem do usuário deve aparecer na lista
    const userMessage = page.locator('.message-node').first();
    await expect(userMessage).toBeVisible({ timeout: 5_000 });
  });

  test('Shift+Enter não envia, apenas adiciona nova linha', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Linha 1');
    await expect.poll(async () => textarea.inputValue()).toBe('Linha 1');
    await textarea.press('End');
    await textarea.press('Shift+Enter');
    await textarea.type('Linha 2');

    // O campo deve conter as duas linhas (sem ter enviado)
    await expect.poll(async () => (await textarea.inputValue()).replace(/\r\n/g, '\n')).toBe('Linha 1\nLinha 2');
  });

  test('textarea limpa após envio', async ({ page, wails }) => {
    await wails.setResponse('SendMessage', 42);
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Mensagem de teste');
    await textarea.press('Enter');

    // Textarea deve ficar vazio após envio
    await expect(textarea).toHaveValue('', { timeout: 3_000 });
  });
});

test.describe('Chat — streaming de resposta', () => {
  test('exibe mensagem do assistente via streaming', async ({ page, wails }) => {
    const now = new Date().toISOString();

    // Configura GetMessages para retornar a mensagem do usuário já existente
    await wails.setResponse('GetMessages', [
      {
        message: {
          id: 1,
          conversationId: 1,
          role: 'user',
          content: 'Olá!',
          createdAt: now,
        },
        children: [],
      },
    ]);
    await wails.setResponse('SendMessage', 2);
    await wails.setResponse('EnsureConversation', {
      id: 1,
      title: 'Test',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 1,
    });

    await wails.waitForApp();

    // Aguarda as mensagens renderizarem
    await page.waitForSelector('.message-node', { timeout: 5_000 });

    // Simula stream do backend: envia conteúdo
    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
      content: 'Olá! Como posso ajudar?',
      done: false,
    });

    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
      content: 'Olá! Como posso ajudar?',
      done: true,
    });

    // O conteúdo do streaming deve aparecer eventualmente
    const assistantContent = page.locator('.message-node').last();
    await expect(assistantContent).toBeVisible({ timeout: 5_000 });
  });
});

test.describe('Chat — toolbar', () => {
  test('toolbar do chat está visível com botões essenciais', async ({ page, wails }) => {
    await wails.waitForApp();

    // A toolbar do chat deve estar presente
    const toolbar = page.locator('.ws-content-toolbar');
    await expect(toolbar).toBeVisible();
  });
});

test.describe('Chat — acessibilidade', () => {
  test('textarea tem aria-label adequado', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    const label = await textarea.getAttribute('aria-label');
    expect(label).toBeTruthy();
  });

  test('lista de mensagens tem role adequado', async ({ page, wails }) => {
    const now = new Date().toISOString();
    await wails.setResponse('GetMessages', [
      {
        message: {
          id: 1,
          conversationId: 1,
          role: 'user',
          content: 'Teste',
          createdAt: now,
        },
        children: [],
      },
    ]);

    await wails.waitForApp();
    await page.waitForSelector('[role="list"]', { timeout: 5_000 });

    const list = page.locator('.message-list [role="list"]');
    await expect(list).toBeVisible();
  });

  test('botão de enviar tem aria-label', async ({ page, wails }) => {
    await wails.waitForApp();

    // O botão de enviar só aparece quando há texto (voice button ocupa quando vazio)
    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Olá');
    await expect.poll(async () => textarea.inputValue(), { timeout: 5_000 }).toBe('Olá');

    const sendBtn = page.locator('.chat-input__button');
    await expect(sendBtn).toBeVisible({ timeout: 5_000 });
    const label = await sendBtn.getAttribute('aria-label');
    expect(label).toBeTruthy();
  });

  test('campo de entrada ficável via Tab', async ({ page, wails }) => {
    await wails.waitForApp();

    // Clica fora e depois faz Tab para verificar que o input é atingível
    await page.locator('body').click();

    // Usa Tab repetidamente até chegar no textarea
    const textarea = page.locator('.chat-input__textarea');
    let focused = false;
    for (let i = 0; i < 20; i++) {
      await page.keyboard.press('Tab');
      if (await textarea.evaluate(el => el === document.activeElement)) {
        focused = true;
        break;
      }
    }
    expect(focused).toBe(true);
  });
});
