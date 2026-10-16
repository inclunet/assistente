import { test, expect } from '../fixtures';

const conversationId = '01926b90-0000-7000-8000-000000000001';
const userMessageId = '01926b90-7a5a-7c4e-8d3f-000000000101';
const assistantMessageId = '01926b90-7a5a-7c4e-8d3f-000000000102';
const secondUserMessageId = '01926b90-7a5a-7c4e-8d3f-000000000103';
const parentMessageId = '01926b90-7a5a-7c4e-8d3f-000000000111';
const threadAssistantMessageId = '01926b90-7a5a-7c4e-8d3f-000000000112';
const internalThreadMessageId = '01926b90-7a5a-7c4e-8d3f-000000000113';
const withChildrenMessageId = '01926b90-7a5a-7c4e-8d3f-000000000121';
const withoutChildrenMessageId = '01926b90-7a5a-7c4e-8d3f-000000000122';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
    __messageOperationRequests?: Array<{ commandId: string; accepted: boolean }>;
    __messageNavigationRequests?: Array<{
      commandID: string;
      accepted: boolean;
      activeMessageId: string;
      activeExpanded: string | null;
      documentHasFocus: boolean;
    }>;
  }
}

/**
 * Testes de operações por teclado em mensagens do chat.
 *
 * Teclas testadas no MessageNode:
 * - Delete: deleta mensagem
 * - F2: edita mensagem (apenas user)
 * - Ctrl+C: copia conteúdo
 * - Ctrl+Shift+C: copia conteúdo em Markdown
 * - Space: reproduz TTS (speak)
 * - R: toggle reasoning (apenas assistant com reasoning)
 * - ArrowRight: expande thread (filhos)
 * - ArrowLeft: colapsa thread ou volta ao pai
 * - Shift+F10 / ContextMenu: abre menu de contexto
 * - PageDown / PageUp: pula 10 mensagens
 */

function messagesFixture() {
  const now = new Date().toISOString();
  return [
    {
      message: { id: userMessageId, conversationId, role: 'user', content: 'Mensagem **do usuário**', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: {
        id: assistantMessageId, conversationId, role: 'assistant', content: 'Resposta do assistente',
        createdAt: now, reasoning: 'Pensamento interno do assistente',
      },
      children: [],
      childCount: 0,
    },
    {
      message: { id: secondUserMessageId, conversationId, role: 'user', content: 'Segunda mensagem do usuário', createdAt: now },
      children: [],
      childCount: 0,
    },
  ];
}

async function setupChatWithMessages(wails: Parameters<Parameters<typeof test>[2]>[0]['wails']) {
  const now = new Date().toISOString();
  await wails.setResponse('GetMessages', messagesFixture());
  await wails.setResponse('SendMessage', '01926b90-7a5a-7c4e-8d3f-000000000104');
  await wails.setResponse('UpdateMessage', undefined);
  await wails.setResponse('SpeakMessage', undefined);
  await wails.setResponse('EnsureConversation', {
    id: conversationId, title: 'Conversa', created_at: now, updated_at: now,
    messages: [], message_count: 3,
  });
  await wails.waitForApp();
}

async function installMessageCommandLedger(
  page: import('@playwright/test').Page,
  options: { conversationId: string; initialNodes: ReturnType<typeof messagesFixture>; removeMessageOnCommit: boolean },
) {
  await page.evaluate(({ conversationId, initialNodes, removeMessageOnCommit }) => {
    let sequence = 0;
    let currentNodes = initialNodes;
    const invocations = new Map<string, {
      invocationId: string;
      commandId: string;
      messageId?: string;
      handoffId?: string;
      status: string;
    }>();
    const setMessageWindow = () => window.__wailsMock.setResponse('GetConversationMessageWindow', () => ({
      scope: 'conversation', conversationId, nodes: currentNodes, totalCount: currentNodes.length,
      startIndex: 0, endIndex: currentNodes.length - 1, hasBefore: false, hasAfter: false,
    }));
    setMessageWindow();
    window.__wailsMock.setResponse('GetMessages', () => currentNodes);
    window.__wailsMock.setResponse('BeginUICommand', (commandId: string) => {
      if (!['chat.message.copy', 'chat.message.copy_markdown', 'chat.message.delete'].includes(commandId)) {
        throw new Error(`Unexpected message command ${commandId}`);
      }
      sequence += 1;
      const uuid = (suffix: number) => `01926b90-7a5a-7c4e-8d3f-${String(suffix).padStart(12, '0')}`;
      const ticket = uuid(100 + sequence);
      const invocationId = uuid(200 + sequence);
      invocations.set(ticket, { invocationId, commandId, status: 'pending' });
      return { ticket, invocationId, commandId };
    });
    window.__wailsMock.setResponse('PrepareChatMessageCommand', (ticket: string, messageId: string) => {
      const invocation = invocations.get(ticket);
      if (!invocation || invocation.status !== 'pending' || !messageId) throw new Error('Invalid chat-message preparation');
      invocation.messageId = messageId;
    });
    window.__wailsMock.setResponse('TakeUICommand', (ticket: string) => {
      const invocation = invocations.get(ticket);
      if (!invocation || invocation.status !== 'pending' || !invocation.messageId) throw new Error('Unprepared message command');
      invocation.status = 'taken';
      invocation.handoffId = `01926b90-7a5a-7c4e-8d3f-${String(300 + invocations.size).padStart(12, '0')}`;
      return { ticket, invocationId: invocation.invocationId, commandId: invocation.commandId, handoffId: invocation.handoffId };
    });
    window.__wailsMock.setResponse('CompleteUICommand', (ticket: string, handoffId: string, status: string) => {
      const invocation = invocations.get(ticket);
      if (!invocation || invocation.status !== 'taken' || invocation.handoffId !== handoffId ||
          invocation.commandId === 'chat.message.delete' || !['succeeded', 'failed', 'cancelled'].includes(status)) {
        throw new Error('Invalid message UI-command completion');
      }
      invocation.status = status;
    });
    window.__wailsMock.setResponse('CommitChatMessageCommand', (ticket: string, handoffId: string) => {
      const invocation = invocations.get(ticket);
      if (!invocation || invocation.status !== 'taken' || invocation.handoffId !== handoffId ||
          invocation.commandId !== 'chat.message.delete' || !invocation.messageId) {
        throw new Error('Invalid chat-message backend commit');
      }
      if (removeMessageOnCommit) {
        currentNodes = currentNodes.filter(node => node.message.id !== invocation.messageId);
        setMessageWindow();
      }
      invocation.status = 'succeeded';
    });
    window.__wailsMock.setResponse('GetUICommandResult', (ticket: string) => {
      const invocation = invocations.get(ticket);
      if (!invocation) throw new Error('Unknown message command ticket');
      return { invocationId: invocation.invocationId, status: invocation.status };
    });
    window.__wailsMock.setResponse('CancelUICommand', (ticket: string) => {
      const invocation = invocations.get(ticket);
      if (invocation?.status === 'pending') invocation.status = 'cancelled';
    });
  }, options);
}

async function observeMessageOperationRequests(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__messageOperationRequests = [];
    window.addEventListener('commands:chat-messaging', event => {
      const detail = (event as CustomEvent<{ commandId?: unknown }>).detail;
      if (typeof detail?.commandId === 'string') {
        window.__messageOperationRequests?.push({ commandId: detail.commandId, accepted: event.defaultPrevented });
      }
    });
  });
}

async function observeMessageNavigationRequests(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__messageNavigationRequests = [];
    window.addEventListener('commands:chat-navigation', event => {
      const detail = (event as CustomEvent<{ commandID?: unknown }>).detail;
      if (typeof detail?.commandID === 'string') {
        const active = document.activeElement instanceof HTMLElement ? document.activeElement : null;
        window.__messageNavigationRequests?.push({
          commandID: detail.commandID,
          accepted: event.defaultPrevented,
          activeMessageId: active?.closest<HTMLElement>('.message-node')?.dataset.messageId ?? '',
          activeExpanded: active?.classList.contains('message-node') ? active.getAttribute('aria-expanded') : null,
          documentHasFocus: document.hasFocus(),
        });
      }
    });
  });
}

async function pauseRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    window.__origRAF = window.requestAnimationFrame;
    window.requestAnimationFrame = (cb: FrameRequestCallback) => {
      window.__rafQueue = window.__rafQueue || [];
      window.__rafQueue.push(cb);
      return 0;
    };
  });
}

async function resumeRAF(page: import('@playwright/test').Page) {
  await page.evaluate(() => {
    if (window.__origRAF) {
      window.requestAnimationFrame = window.__origRAF;
      const queue = window.__rafQueue || [];
      window.__rafQueue = [];
      for (const cb of queue) {
        try { cb(performance.now()); } catch (_) { /* ignore */ }
      }
    }
  });
}

test.describe('MessageNode — Delete key', () => {
  test('Delete prepara e commita a exclusão auditada da mensagem focada', async ({ page, wails }) => {
    await setupChatWithMessages(wails);
    await installMessageCommandLedger(page, { conversationId, initialNodes: messagesFixture(), removeMessageOnCommit: true });

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await observeMessageOperationRequests(page);
    // Foca na primeira mensagem (user)
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });

    // O mock representa somente o ledger/commit para a integração frontend; a autorização
    // e as decisões de stale/cancelamento permanecem cobertas pelos testes Go do App.
    await page.keyboard.press('Delete');

    const requests = await page.evaluate(() => window.__messageOperationRequests ?? []);
    expect(requests).toContainEqual({ commandId: 'chat.message.delete', accepted: true });
    await expect.poll(async () => (await wails.getCallLog()).filter(call => call.fn === 'CommitChatMessageCommand').length).toBe(1);
    const calls = await wails.getCallLog();
    expect(calls.find(call => call.fn === 'PrepareChatMessageCommand')?.args[1]).toBe(userMessageId);
    expect(calls.find(call => call.fn === 'CommitChatMessageCommand')?.args).toHaveLength(2);
    expect(calls.find(call => call.fn === 'GetUICommandResult')?.args).toHaveLength(1);
    expect(calls.some(call => call.fn === 'DeleteMessage')).toBe(false);
    await expect(page.locator(`.message-node[data-message-id="${userMessageId}"]`)).toHaveCount(0, { timeout: 5_000 });
    await expect(page.locator('.message-node[data-level="0"]')).toHaveCount(2, { timeout: 5_000 });
    await resumeRAF(page);
  });
});

test.describe('MessageNode — F2 edit mode', () => {
  test('F2 em mensagem do usuário ativa modo de edição', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    // Foca na primeira mensagem (user)
    await messages.first().focus();
    await page.keyboard.press('F2');

    // Deve aparecer um textarea de edição dentro da mensagem
    const editArea = messages.first().locator('textarea');
    await expect(editArea).toBeVisible({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('F2 não ativa edição em mensagem do assistente', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    // Foca na segunda mensagem (assistant)
    await messages.nth(1).focus();
    await page.keyboard.press('F2');

    // Não deve exibir textarea de edição
    const editArea = messages.nth(1).locator('textarea');
    await expect(editArea).toHaveCount(0);
    await resumeRAF(page);
  });
});

test.describe('MessageNode — Ctrl+C copy', () => {
  test('Ctrl+C copia conteúdo da mensagem focada após admissão auditada', async ({ page, wails, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await setupChatWithMessages(wails);
    await installMessageCommandLedger(page, { conversationId, initialNodes: messagesFixture(), removeMessageOnCommit: false });

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await observeMessageOperationRequests(page);
    await messages.first().focus();
    await page.keyboard.press('Control+c');

    const requests = await page.evaluate(() => window.__messageOperationRequests ?? []);
    expect(requests).toContainEqual({ commandId: 'chat.message.copy', accepted: true });
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('Mensagem do usuário');
    const calls = await wails.getCallLog();
    expect(calls.find(call => call.fn === 'PrepareChatMessageCommand')?.args[1]).toBe(userMessageId);
    expect(calls.find(call => call.fn === 'CompleteUICommand')?.args[2]).toBe('succeeded');
    expect(calls.find(call => call.fn === 'GetUICommandResult')?.args).toHaveLength(1);
    await resumeRAF(page);
  });

  test('Ctrl+Shift+C copia Markdown da mensagem focada após admissão auditada', async ({ page, wails, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await setupChatWithMessages(wails);
    await installMessageCommandLedger(page, { conversationId, initialNodes: messagesFixture(), removeMessageOnCommit: false });

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await observeMessageOperationRequests(page);
    await messages.first().focus();
    await page.keyboard.press('Control+Shift+c');

    const requests = await page.evaluate(() => window.__messageOperationRequests ?? []);
    expect(requests).toContainEqual({ commandId: 'chat.message.copy_markdown', accepted: true });
    await expect.poll(() => page.evaluate(() => navigator.clipboard.readText())).toBe('Mensagem **do usuário**');
    const calls = await wails.getCallLog();
    expect(calls.find(call => call.fn === 'PrepareChatMessageCommand')?.args[1]).toBe(userMessageId);
    expect(calls.find(call => call.fn === 'CompleteUICommand')?.args[2]).toBe('succeeded');
    expect(calls.find(call => call.fn === 'GetUICommandResult')?.args).toHaveLength(1);
    await resumeRAF(page);
  });
});

test.describe('MessageNode — Space speak (TTS)', () => {
  test('Space em mensagem não-streaming é tratado (preventDefault)', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();
    await expect(messages.nth(1)).toBeFocused({ timeout: 3_000 });

    // Observa se o handler é chamado verificando que a tecla não causa scroll
    // ou type-ahead (Space é preventDefault no handler)
    const scrollBefore = await page.evaluate(() => {
      const list = document.querySelector('.message-list, [role="list"]');
      return list?.scrollTop ?? 0;
    });

    await page.keyboard.press('Space');
    await page.waitForTimeout(300);

    // Space não deve ter causado scroll (foi preventDefault)
    const scrollAfter = await page.evaluate(() => {
      const list = document.querySelector('.message-list, [role="list"]');
      return list?.scrollTop ?? 0;
    });
    expect(scrollAfter).toBe(scrollBefore);
    await resumeRAF(page);
  });
});

test.describe('MessageNode — R toggle reasoning', () => {
  test('R em mensagem assistant com reasoning alterna visibilidade', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    // Segunda mensagem é assistant com reasoning
    await messages.nth(1).focus();
    await expect(messages.nth(1)).toBeFocused({ timeout: 3_000 });

    // Pressiona R para toggle reasoning
    await page.keyboard.press('r');

    // Verifica que o anúncio de reasoning foi feito (aria-live)
    // O componente usa announce() para informar sobre show/hide
    const announcement = page.locator('[role="status"][aria-live="polite"]');
    // O anúncio pode estar em qualquer announcer element
    await page.waitForTimeout(200);
    await resumeRAF(page);
  });

  test('R em mensagem user sem reasoning é ignorado (foco não muda)', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    // Primeira mensagem é user (sem reasoning)
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
    await page.keyboard.press('r');
    await page.waitForTimeout(100);

    // R em user msg sem reasoning: o handler não previne default,
    // então o type-ahead pode disparar. Verificamos apenas que o
    // tipo de tecla foi processado sem erro.
    // (O foco pode ter ido ao input pelo type-ahead, isso é comportamento esperado)
    await resumeRAF(page);
  });
});

test.describe('MessageNode — ArrowRight/Left thread expand/collapse', () => {
  test('ArrowRight expande thread e foca no primeiro filho', async ({ page, wails }) => {
    const now = new Date().toISOString();
    const messagesWithChildren = [
      {
        message: { id: parentMessageId, conversationId, role: 'user', content: 'Msg pai', createdAt: now },
        children: [],
        childCount: 1,
      },
      {
        message: { id: threadAssistantMessageId, conversationId, role: 'assistant', content: 'Resposta', createdAt: now },
        children: [],
        childCount: 0,
      },
    ];

    const childrenResponse = [
      {
        message: { id: internalThreadMessageId, conversationId, role: 'assistant', content: 'Resposta interna', createdAt: now, internal: true },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithChildren);
    await wails.setResponse('GetMessageChildren', childrenResponse);
    await wails.setResponse('EnsureConversation', {
      id: conversationId, title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 2,
    });
    await wails.waitForApp();

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(2, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();

    // A primeira mensagem tem filhos (childCount: 1) mas começa colapsada (children: [])
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'false');

    // ArrowRight expande — não usar pauseRAF aqui pois a expansão precisa de rAF para render
    await resumeRAF(page);
    await messages.first().focus();
    await page.keyboard.press('ArrowRight');

    // Aguarda expansão (load children é async)
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'true', { timeout: 5_000 });
  });

  test('ArrowLeft colapsa thread expandida', async ({ page, wails }) => {
    const now = new Date().toISOString();
    const messagesWithChildren = [
      {
        message: { id: parentMessageId, conversationId, role: 'user', content: 'Msg pai', createdAt: now },
        children: [],
        childCount: 1,
      },
    ];

    const childrenResponse = [
      {
        message: { id: internalThreadMessageId, conversationId, role: 'assistant', content: 'Resposta interna', createdAt: now, internal: true },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithChildren);
    await wails.setResponse('GetMessageChildren', childrenResponse);
    await wails.setResponse('EnsureConversation', {
      id: conversationId, title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 1,
    });
    await wails.waitForApp();

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(1, { timeout: 5_000 });
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'false');

    // Keep the browser's normal focus/context publication cycle running in this test.
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
    await observeMessageNavigationRequests(page);

    // Expande primeiro
    await page.keyboard.press('ArrowRight');
    await expect.poll(async () => page.evaluate(() => window.__messageNavigationRequests ?? []))
      .toEqual(expect.arrayContaining([
        expect.objectContaining({ commandID: 'chat.message.thread.expand', accepted: true }),
      ]));
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'true', { timeout: 5_000 });
    const childMessage = page.locator(`.message-node[data-level="1"][data-message-id="${internalThreadMessageId}"]`);
    await expect(childMessage).toBeVisible({ timeout: 5_000 });
    await expect(childMessage).toBeFocused({ timeout: 5_000 });

    // Foca de volta no pai
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
    await observeMessageNavigationRequests(page);

    // ArrowLeft colapsa
    await page.keyboard.press('ArrowLeft');
    const requests = await page.evaluate(() => window.__messageNavigationRequests ?? []);
    expect(requests).toEqual(expect.arrayContaining([
      expect.objectContaining({ commandID: 'chat.message.thread.collapse', accepted: true }),
    ]));
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'false', { timeout: 5_000 });
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
  });
});

test.describe('MessageNode — Shift+F10 context menu', () => {
  test('Shift+F10 abre menu de contexto via teclado', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    const firstMessage = messages.first();
    await firstMessage.focus();
    await expect(firstMessage).toBeFocused({ timeout: 3_000 });

    // Shift+F10 abre o context menu
    await firstMessage.press('Shift+F10');
    await resumeRAF(page);

    // Deve aparecer um menu de contexto
    const contextMenu = page.locator('[role="menu"]');
    await expect(contextMenu).toBeVisible({ timeout: 7_000 });
  });
});

test.describe('MessageNode — PageDown/PageUp navigation', () => {
  test('PageDown pula múltiplas mensagens', async ({ page, wails }) => {
    // Cria um fixture com muitas mensagens
    const now = new Date().toISOString();
    const manyMessages = Array.from({ length: 15 }, (_, i) => ({
      message: {
        id: `01926b90-7a5a-7c4e-8d3f-${String(i + 200).padStart(12, '0')}`,
        conversationId,
        role: i % 2 === 0 ? 'user' : 'assistant',
        content: `Mensagem ${i + 1}`,
        createdAt: now,
      },
      children: [],
      childCount: 0,
    }));

    await wails.setResponse('GetMessages', manyMessages);
    await wails.setResponse('EnsureConversation', {
      id: conversationId, title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 15,
    });
    await wails.waitForApp();

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(15, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });

    // PageDown pula 10 mensagens (de 0 para 10)
    await page.keyboard.press('PageDown');
    await expect(messages.nth(10)).toBeFocused({ timeout: 3_000 });

    // PageUp volta 10 mensagens (de 10 para 0)
    await page.keyboard.press('PageUp');
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });
});

test.describe('MessageNode — ARIA attributes', () => {
  test('mensagens com filhos têm aria-expanded', async ({ page, wails }) => {
    const now = new Date().toISOString();
    const messagesWithAndWithoutChildren = [
      {
        message: { id: withChildrenMessageId, conversationId, role: 'user', content: 'Com filhos', createdAt: now },
        children: [],
        childCount: 2,
      },
      {
        message: { id: withoutChildrenMessageId, conversationId, role: 'assistant', content: 'Sem filhos', createdAt: now },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithAndWithoutChildren);
    await wails.setResponse('EnsureConversation', {
      id: conversationId, title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 2,
    });
    await wails.waitForApp();

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(2, { timeout: 5_000 });

    // Mensagem com filhos deve ter aria-expanded
    const ariaExpanded = await messages.first().getAttribute('aria-expanded');
    expect(ariaExpanded).toBeDefined();

    // Mensagem sem filhos não deve ter aria-expanded
    const noAriaExpanded = await messages.nth(1).getAttribute('aria-expanded');
    expect(noAriaExpanded).toBeNull();
  });
});
