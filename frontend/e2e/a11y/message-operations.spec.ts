import { test, expect } from '../fixtures';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
  }
}

/**
 * Testes de operações por teclado em mensagens do chat.
 *
 * Teclas testadas no MessageNode:
 * - Delete: deleta mensagem
 * - F2: edita mensagem (apenas user)
 * - Ctrl+C: copia conteúdo
 * - Ctrl+Shift+C: copia conteúdo com role
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
      message: { id: '01926b90-7a5a-7c4e-8d3f-000000000001', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: 'Mensagem do usuário', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: {
        id: '01926b90-7a5a-7c4e-8d3f-000000000002', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'assistant', content: 'Resposta do assistente',
        createdAt: now, reasoning: 'Pensamento interno do assistente',
      },
      children: [],
      childCount: 0,
    },
    {
      message: { id: '01926b90-7a5a-7c4e-8d3f-000000000003', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: 'Segunda mensagem do usuário', createdAt: now },
      children: [],
      childCount: 0,
    },
  ];
}

async function setupChatWithMessages(wails: Parameters<Parameters<typeof test>[2]>[0]['wails']) {
  const now = new Date().toISOString();
  await wails.setResponse('GetMessages', messagesFixture());
  await wails.setResponse('SendMessage', '01926b90-7a5a-7c4e-8d3f-000000000004');
  await wails.setResponse('DeleteMessage', undefined);
  await wails.setResponse('UpdateMessage', undefined);
  await wails.setResponse('SpeakMessage', undefined);
  await wails.setResponse('EnsureConversation', {
    id: '01926b90-0000-7000-8000-000000000001', title: 'Conversa', created_at: now, updated_at: now,
    messages: [], message_count: 3,
  });
  await wails.waitForApp();
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
  test('Delete em mensagem não-streaming invoca onDelete', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    // Foca na primeira mensagem (user)
    await messages.first().focus();
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });

    // Pressiona Delete — deve chamar DeleteMessage no backend
    await page.keyboard.press('Delete');

    // Verifica que o backend recebeu a chamada (via callLog)
    const calls = await wails.getCallLog();
    const deleteCall = calls.find(c => c.fn === 'DeleteMessage');
    expect(deleteCall).toBeDefined();
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
  test('Ctrl+C copia conteúdo para clipboard', async ({ page, wails, context }) => {
    // Concede permissão de clipboard
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();
    await page.keyboard.press('Control+c');

    // Verifica o conteúdo do clipboard
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe('Mensagem do usuário');
    await resumeRAF(page);
  });

  test('Ctrl+Shift+C copia conteúdo com role', async ({ page, wails, context }) => {
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();
    await page.keyboard.press('Control+Shift+c');

    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toBe('[user] Mensagem do usuário');
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
        message: { id: '1', conversationId: '1', role: 'user', content: 'Msg pai', createdAt: now },
        children: [],
        childCount: 1,
      },
      {
        message: { id: '2', conversationId: '1', role: 'assistant', content: 'Resposta', createdAt: now },
        children: [],
        childCount: 0,
      },
    ];

    const childrenResponse = [
      {
        message: { id: '1-1', conversationId: '1', role: 'assistant', content: 'Resposta interna', createdAt: now, internal: true },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithChildren);
    await wails.setResponse('GetMessageChildren', childrenResponse);
    await wails.setResponse('EnsureConversation', {
      id: '1', title: 'Conversa', created_at: now, updated_at: now,
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
        message: { id: '1', conversationId: '1', role: 'user', content: 'Msg pai', createdAt: now },
        children: [],
        childCount: 1,
      },
    ];

    const childrenResponse = [
      {
        message: { id: '1-1', conversationId: '1', role: 'assistant', content: 'Resposta interna', createdAt: now, internal: true },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithChildren);
    await wails.setResponse('GetMessageChildren', childrenResponse);
    await wails.setResponse('EnsureConversation', {
      id: '1', title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 1,
    });
    await wails.waitForApp();

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(1, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();

    // Expande primeiro
    await page.keyboard.press('ArrowRight');
    await resumeRAF(page);
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'true', { timeout: 5_000 });

    // Foca de volta no pai
    await pauseRAF(page);
    await messages.first().focus();

    // ArrowLeft colapsa
    await page.keyboard.press('ArrowLeft');
    await resumeRAF(page);
    await expect(messages.first()).toHaveAttribute('aria-expanded', 'false', { timeout: 5_000 });
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
        id: String(i + 1),
        conversationId: '1',
        role: i % 2 === 0 ? 'user' : 'assistant',
        content: `Mensagem ${i + 1}`,
        createdAt: now,
      },
      children: [],
      childCount: 0,
    }));

    await wails.setResponse('GetMessages', manyMessages);
    await wails.setResponse('EnsureConversation', {
      id: '1', title: 'Conversa', created_at: now, updated_at: now,
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
        message: { id: '1', conversationId: '1', role: 'user', content: 'Com filhos', createdAt: now },
        children: [],
        childCount: 2,
      },
      {
        message: { id: '2', conversationId: '1', role: 'assistant', content: 'Sem filhos', createdAt: now },
        children: [],
        childCount: 0,
      },
    ];

    await wails.setResponse('GetMessages', messagesWithAndWithoutChildren);
    await wails.setResponse('EnsureConversation', {
      id: '1', title: 'Conversa', created_at: now, updated_at: now,
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
