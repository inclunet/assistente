import { test, expect } from '../fixtures';

const now = new Date().toISOString();
const conversationId = '1';
const initialUserMessageId = '01968740-1234-7000-8000-000000000002';
const initialAssistantMessageId = '01968740-1234-7000-8000-000000000003';
const firstFailedUserMessageId = '01968740-1234-7000-8000-000000000004';
const firstFailedAssistantMessageId = '01968740-1234-7000-8000-000000000005';
const secondAttemptUserMessageId = '01968740-1234-7000-8000-000000000006';
const secondAttemptAssistantMessageId = '01968740-1234-7000-8000-000000000007';
const retryAssistantMessageId = '01968740-1234-7000-8000-000000000008';

const baseConversation = {
  id: conversationId,
  title: 'Test Conversation',
  created_at: now,
  updated_at: now,
  messages: [],
  message_count: 1,
};

const userMessage = {
  message: {
    id: initialUserMessageId,
    conversationId,
    role: 'user',
    content: 'Olá!',
    createdAt: now,
  },
  children: [],
};

test.describe('Chat — streaming multi-segmento', () => {
  test('exibe segmentos text → tool → text em sequência', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', initialAssistantMessageId);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Pesquise sobre IA');
    await textarea.press('Enter');

    // Inicia streaming (conteúdo vazio para criar o container do assistente)
    await wails.emit('chat:stream', {
      conversationId,
      messageId: initialAssistantMessageId,
      content: '',
      done: false,
    });

    // Tool call dentro do stream (conversationId obrigatório para filtro backend-driven)
    await wails.emit('chat:tool_start', {
      conversationId,
      name: 'search_web',
      callId: 'tc-seg-1',
      args: '{"query":"inteligência artificial"}',
    });

    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible({ timeout: 5_000 });

    await wails.emit('chat:tool_end', {
      conversationId,
      callId: 'tc-seg-1',
      name: 'search_web',
      status: 'success',
      summary: 'Encontrados 5 resultados',
    });

    // Após tool_end, a seção deve continuar visível (streaming ainda ativo)
    await expect(toolSection).toBeVisible({ timeout: 3_000 });

    // Texto final pós-tool
    await wails.emit('chat:stream', {
      conversationId,
      messageId: initialAssistantMessageId,
      content: 'Baseado na pesquisa, a IA é uma tecnologia que...',
      done: true,
    });

    await wails.emit('chat:done', {});

    // Após chat:done, o input deve estar habilitado
    await page.waitForFunction(() => {
      const ta = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement;
      return ta && !ta.disabled;
    }, { timeout: 5_000 });
  });

  test('chat:done reabilita o input para nova mensagem', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', initialAssistantMessageId);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.message-node', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Teste');
    await textarea.press('Enter');

    // Simula stream do backend: resposta do assistente
    await wails.emit('chat:stream', {
      conversationId,
      messageId: initialAssistantMessageId,
      content: 'Resposta completa.',
      done: true,
    });
    await wails.emit('chat:done', {});

    // Aguarda o React processar (isLoading → false)
    await page.waitForFunction(() => {
      const ta = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement;
      return ta && !ta.disabled;
    }, { timeout: 5_000 });

    // Agora deve poder digitar de novo
    await textarea.fill('Segunda mensagem');
    await expect(textarea).toHaveValue('Segunda mensagem');
  });
});

test.describe('Chat — erro no envio', () => {
  test('exibe mensagem de erro quando SendMessage falha', async ({ page, wails }) => {
    await wails.setResponse('EnsureConversation', baseConversation);
    await wails.waitForApp();

    // Configura SendMessage para rejeitar
    await wails.setError('SendMessage', 'Network error');

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Mensagem que vai falhar');
    await expect.poll(async () => textarea.inputValue()).toBe('Mensagem que vai falhar');
    await textarea.press('Enter');
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'SendMessage',
      );
    }, { timeout: 5_000 });

    const errorMessage = page.locator('[role="alert"], .chat-message').filter({ hasText: /Network error/ });
    await expect(errorMessage).toBeVisible({ timeout: 5_000 });
  });

  test('input é reabilitado após erro de envio', async ({ page, wails }) => {
    await wails.setResponse('EnsureConversation', baseConversation);
    await wails.waitForApp();

    await wails.setError('SendMessage', 'Timeout');

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Mensagem com erro');
    await expect.poll(async () => textarea.inputValue()).toBe('Mensagem com erro');
    await textarea.press('Enter');
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'SendMessage',
      );
    }, { timeout: 5_000 });

    // Aguarda o erro aparecer
    await page.locator('[role="alert"], .chat-message').filter({ hasText: /Timeout/ }).waitFor({ timeout: 5_000 });

    // Input deve ser reabilitado (isLoading → false)
    await page.waitForFunction(() => {
      const ta = document.querySelector('.chat-input__textarea') as HTMLTextAreaElement;
      return ta && !ta.disabled;
    }, { timeout: 5_000 });

    // Pode digitar uma nova mensagem
    await textarea.fill('Nova tentativa');
    await expect(textarea).toHaveValue('Nova tentativa');
  });

  test('mensagem de erro contém detalhes do erro original', async ({ page, wails }) => {
    await wails.setResponse('EnsureConversation', baseConversation);
    await wails.waitForApp();

    await wails.setError('SendMessage', 'Connection refused: server unavailable');

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Teste');
    await expect.poll(async () => textarea.inputValue()).toBe('Teste');
    await textarea.press('Enter');
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'SendMessage',
      );
    }, { timeout: 5_000 });

    // A mensagem de erro deve conter o texto do erro
    const errorMessage = page.locator('[role="alert"], .chat-message').filter({ hasText: 'Connection refused' });
    await expect(errorMessage).toBeVisible({ timeout: 5_000 });
  });

  test('erro no stream seguido de novo envio não gera duplicate key no histórico', async ({ page, wails }) => {
    const consoleWarnings: string[] = [];
    page.on('console', (msg) => {
      const text = msg.text();
      if (text.includes('Encountered two children with the same key')) {
        consoleWarnings.push(text);
      }
    });

    await wails.setResponse('EnsureConversation', baseConversation);
    await wails.setResponse('SendMessage', initialAssistantMessageId);
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });

    await textarea.click();
    await textarea.fill('primeira falha');
    await textarea.press('Enter');
    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().filter((c) => c.fn === 'SendMessage').length >= 1,
      undefined,
      { timeout: 5_000 },
    );

    await wails.emit('chat:messages_ready', {
      conversationId,
      userMessageId: firstFailedUserMessageId,
      userContent: 'primeira falha',
    });
    await wails.emit('chat:stream', {
      conversationId,
      messageId: firstFailedAssistantMessageId,
      content: 'resposta parcial',
      done: false,
    });
    await wails.emit('chat:stream', {
      conversationId,
      error: '500 Internal Server Error',
    });
    await expect(page.locator('.chat-message').filter({ hasText: /500 Internal Server Error/ })).toBeVisible({ timeout: 5_000 });

    await textarea.fill('segunda tentativa');
    await textarea.press('Enter');
    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().filter((c) => c.fn === 'SendMessage').length >= 2,
      undefined,
      { timeout: 5_000 },
    );

    await wails.emit('chat:messages_ready', {
      conversationId,
      userMessageId: secondAttemptUserMessageId,
      userContent: 'segunda tentativa',
    });
    await wails.emit('chat:stream', {
      conversationId,
      messageId: secondAttemptAssistantMessageId,
      content: 'resposta final',
      done: true,
    });
    await wails.emit('chat:done', {
      conversationId,
      assistantMessageId: secondAttemptAssistantMessageId,
      hadToolCalls: false,
    });

    await expect(page.locator('.chat-message').filter({ hasText: 'resposta final' })).toBeVisible({ timeout: 5_000 });
    expect(consoleWarnings).toEqual([]);
  });

  test('reenviar mensagem existente após erro usa RetryMessage sem duplicar mensagem do usuário', async ({ page, wails }) => {
    await wails.setResponse('EnsureConversation', baseConversation);
    await wails.setResponse('SendMessage', initialAssistantMessageId);
    await wails.setResponse('RetryMessage', retryAssistantMessageId);
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('mensagem original');
    await textarea.press('Enter');

    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().filter((c) => c.fn === 'SendMessage').length === 1,
      undefined,
      { timeout: 5_000 },
    );

    await wails.emit('chat:messages_ready', {
      conversationId,
      userMessageId: firstFailedUserMessageId,
      userContent: 'mensagem original',
    });
    await wails.emit('chat:stream', {
      conversationId,
      messageId: firstFailedAssistantMessageId,
      content: 'resposta parcial',
      done: false,
    });
    await wails.emit('chat:stream', {
      conversationId,
      error: '500 Internal Server Error',
    });

    const firstUserMessage = page.locator('.message-node[data-level="0"]').filter({ hasText: 'mensagem original' }).first();
    await expect(firstUserMessage).toBeVisible({ timeout: 5_000 });
    await firstUserMessage.focus();
    await firstUserMessage.press('Shift+F10');

    const contextMenu = page.locator('[role="menu"]');
    await expect(contextMenu).toBeVisible({ timeout: 7_000 });
    await contextMenu.locator('[role="menuitem"]', { hasText: /Reenviar mensagem/i }).click();

    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().filter((c) => c.fn === 'RetryMessage').length === 1,
      undefined,
      { timeout: 5_000 },
    );

    await wails.emit('chat:stream', {
      conversationId,
      messageId: retryAssistantMessageId,
      content: 'resposta após retry',
      done: true,
    });
    await wails.emit('chat:done', {
      conversationId,
      assistantMessageId: retryAssistantMessageId,
      hadToolCalls: false,
    });

    await expect(page.locator('.chat-message').filter({ hasText: 'resposta após retry' })).toBeVisible({ timeout: 5_000 });

    const callLog = await wails.getCallLog();
    expect(callLog.filter((c) => c.fn === 'SendMessage')).toHaveLength(1);
    expect(callLog.filter((c) => c.fn === 'RetryMessage')).toHaveLength(1);
    await expect(page.locator('.message-node[data-level="0"]').filter({ hasText: 'mensagem original' })).toHaveCount(1);
  });
});

test.describe('Chat — thinking/reasoning', () => {
  test('exibe seção de raciocínio durante streaming', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', initialAssistantMessageId);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.message-node', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeEditable({ timeout: 5_000 });
    await textarea.click();
    await textarea.fill('Pense sobre isso');
    await expect.poll(async () => textarea.inputValue(), { timeout: 5_000 }).toBe('Pense sobre isso');
    await textarea.press('Enter');

    await page.waitForFunction(
      () => window.__wailsMock.getCallLog().some((c) => c.fn === 'SendMessage'),
      undefined,
      { timeout: 5_000 },
    );

    // Simula início de thinking (conversationId obrigatório)
    await wails.emit('chat:thinking', {
      conversationId,
      started: true,
      content: 'Analisando a pergunta...',
    });

    // A seção de raciocínio deve aparecer
    const reasoning = page.locator('.reasoning-section');
    await expect(reasoning).toBeVisible({ timeout: 5_000 });

    // Finaliza thinking e inicia resposta
    await wails.emit('chat:thinking', {
      conversationId,
      done: true,
      content: 'Analisando a pergunta... Considerando diferentes perspectivas.',
    });

    await wails.emit('chat:stream', {
      conversationId,
      messageId: initialAssistantMessageId,
      content: 'Após analisar, a resposta é...',
      done: true,
    });

    await wails.emit('chat:done', {});

    // A seção de raciocínio deve continuar visível (colapsável)
    await expect(reasoning).toBeVisible();
  });

  test('seção de raciocínio é togglável via clique no header', async ({ page, wails }) => {
    // Configura mensagem com reasoning já existente
    await wails.setResponse('GetMessages', [
      userMessage,
      {
        message: {
          id: initialAssistantMessageId,
          conversationId,
          role: 'assistant',
          content: 'Resposta final',
          createdAt: now,
          reasoning: 'Pensei sobre X e Y e Z',
        },
        children: [],
      },
    ]);

    await wails.waitForApp();
    await page.waitForSelector('.reasoning-section', { timeout: 5_000 });

    const header = page.locator('.reasoning-section__header');
    await expect(header).toBeVisible();

    // Clica para expandir/colapsar
    const initialExpanded = await header.getAttribute('aria-expanded');
    await header.click();

    const newExpanded = await header.getAttribute('aria-expanded');
    expect(newExpanded).not.toBe(initialExpanded);
  });
});

test.describe('Chat — Ctrl+L limpar conversa', () => {
  test('Ctrl+L chama ClearConversation', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('ClearConversation', undefined);
    await wails.setResponse('ClearMessages', undefined);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.message-node', { timeout: 5_000 });

    // Pressiona Ctrl+L
    await page.keyboard.press('Control+l');

    // Verifica que a função de clear foi chamada
    const log = await wails.getCallLog();
    const clearCalls = log.filter(c =>
      c.fn === 'ClearConversation' || c.fn === 'ClearMessages',
    );
    expect(clearCalls.length).toBeGreaterThanOrEqual(1);
  });
});
