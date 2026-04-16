import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const baseConversation = {
  id: 1,
  title: 'Test Conversation',
  created_at: now,
  updated_at: now,
  messages: [],
  message_count: 1,
};

const userMessage = {
  message: {
    id: 1,
    conversationId: 1,
    role: 'user',
    content: 'Olá!',
    createdAt: now,
  },
  children: [],
};

test.describe('Chat — streaming multi-segmento', () => {
  test('exibe segmentos text → tool → text em sequência', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', 2);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.chat-message', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Pesquise sobre IA');
    await textarea.press('Enter');

    // Inicia streaming (conteúdo vazio para criar o container do assistente)
    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
      content: '',
      done: false,
    });

    // Tool call dentro do stream (conversationId obrigatório para filtro backend-driven)
    await wails.emit('chat:tool_start', {
      conversationId: 1,
      name: 'search_web',
      callId: 'tc-seg-1',
      args: '{"query":"inteligência artificial"}',
    });

    const toolSection = page.locator('.tool-calls-section');
    await expect(toolSection).toBeVisible({ timeout: 5_000 });

    await wails.emit('chat:tool_end', {
      conversationId: 1,
      callId: 'tc-seg-1',
      name: 'search_web',
      status: 'success',
      summary: 'Encontrados 5 resultados',
    });

    // Após tool_end, a seção deve continuar visível (streaming ainda ativo)
    await expect(toolSection).toBeVisible({ timeout: 3_000 });

    // Texto final pós-tool
    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
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
    await wails.setResponse('SendMessage', 2);
    await wails.setResponse('EnsureConversation', baseConversation);

    await wails.waitForApp();
    await page.waitForSelector('.message-node', { timeout: 5_000 });

    const textarea = page.locator('.chat-input__textarea');
    await textarea.fill('Teste');
    await textarea.press('Enter');

    // Simula stream do backend: resposta do assistente
    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
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
});

test.describe('Chat — thinking/reasoning', () => {
  test('exibe seção de raciocínio durante streaming', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', [userMessage]);
    await wails.setResponse('SendMessage', 2);
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
      conversationId: 1,
      started: true,
      content: 'Analisando a pergunta...',
    });

    // A seção de raciocínio deve aparecer
    const reasoning = page.locator('.reasoning-section');
    await expect(reasoning).toBeVisible({ timeout: 5_000 });

    // Finaliza thinking e inicia resposta
    await wails.emit('chat:thinking', {
      conversationId: 1,
      done: true,
      content: 'Analisando a pergunta... Considerando diferentes perspectivas.',
    });

    await wails.emit('chat:stream', {
      conversationId: 1,
      messageId: 2,
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
          id: 2,
          conversationId: 1,
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
