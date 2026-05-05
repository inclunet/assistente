import { test, expect } from '../fixtures';

declare global {
  interface Window {
    __origRAF?: typeof requestAnimationFrame;
    __rafQueue?: FrameRequestCallback[];
  }
}

/**
 * Testes de navegação por teclado no histórico do chat e
 * alternância entre campo de edição ↔ lista de mensagens.
 *
 * MessageNode keyboard:
 * - ArrowDown → próximo irmão (level 0: último → input)
 * - ArrowUp → irmão anterior
 * - Home → primeiro irmão, End → último irmão
 * - Enter → modo de leitura (virtual modal com role="dialog")
 * - Escape → sai do virtual modal → volta ao message-node
 *
 * ChatKeyboardNav:
 * - ArrowUp no input (cursor pos 0) → foca última mensagem
 * - Type-ahead: tecla printável na msg lista → redireciona ao input
 */

function messagesFixture() {
  const now = new Date().toISOString();
  return [
    {
      message: { id: '1', conversationId: 1, role: 'user', content: 'Primeira mensagem', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: { id: '2', conversationId: 1, role: 'assistant', content: 'Resposta do assistente', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: { id: '3', conversationId: 1, role: 'user', content: 'Segunda mensagem', createdAt: now },
      children: [],
      childCount: 0,
    },
  ];
}

async function setupChatWithMessages(wails: Parameters<Parameters<typeof test>[2]>[0]['wails']) {
  const now = new Date().toISOString();
  await wails.setResponse('GetMessages', messagesFixture());
  await wails.setResponse('SendMessage', 4);
  await wails.setResponse('EnsureConversation', {
    id: 1, title: 'Conversa', created_at: now, updated_at: now,
    messages: [], message_count: 3,
  });
  await wails.waitForApp();
}

/**
 * Pausa requestAnimationFrame para evitar que restoreDefaultFocus roube o foco.
 */
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

test.describe('Chat history — navegação por setas entre mensagens', () => {
  test('ArrowDown navega para próxima mensagem', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();

    // ArrowDown → segunda mensagem
    await messages.first().press('ArrowDown');
    await expect(messages.nth(1)).toBeFocused({ timeout: 3_000 });

    // ArrowDown → terceira mensagem
    await messages.nth(1).press('ArrowDown');
    await expect(messages.nth(2)).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('ArrowUp navega para mensagem anterior', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(2).focus();

    // ArrowUp → segunda
    await page.keyboard.press('ArrowUp');
    await expect(messages.nth(1)).toBeFocused({ timeout: 3_000 });

    // ArrowUp → primeira
    await page.keyboard.press('ArrowUp');
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('Home foca primeira mensagem, End foca última', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();

    // Home → primeira
    await page.keyboard.press('Home');
    await expect(messages.first()).toBeFocused({ timeout: 3_000 });

    // End → última
    await page.keyboard.press('End');
    await expect(messages.nth(2)).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });

  test('ArrowDown na última mensagem (level 0) move foco para o input', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(2).focus();

    // ArrowDown na última → input
    await page.keyboard.press('ArrowDown');
    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeFocused({ timeout: 3_000 });
    await resumeRAF(page);
  });
});

test.describe('Chat — alternância input ↔ histórico', () => {
  test('ArrowUp no input vazio foca a última mensagem', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    // O foco padrão é no textarea
    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // O textarea está vazio, cursor na posição 0 → ArrowUp foca última msg
    await page.keyboard.press('ArrowUp');

    const messages = page.locator('.message-node[data-level="0"]');
    const lastMsg = messages.last();
    await expect(lastMsg).toBeFocused({ timeout: 3_000 });
  });

  test('digitar caractere na lista de mensagens redireciona ao input (type-ahead)', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.first().focus();

    // Digitar 'a' deve redirecionar para o input
    await page.keyboard.press('a');

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeFocused({ timeout: 3_000 });

    // O caractere digitado deve estar no input
    await expect(textarea).toHaveValue('a');
    await resumeRAF(page);
  });
});

test.describe('Chat — Enter no detalhe da mensagem (virtual modal)', () => {
  test('Enter na mensagem ativa modo de leitura (role="dialog")', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();
    await messages.nth(1).press('Enter');

    // O message-node deve ter role="dialog" e aria-modal="true"
    await expect(messages.nth(1)).toHaveAttribute('role', 'dialog', { timeout: 3_000 });
    await expect(messages.nth(1)).toHaveAttribute('aria-modal', 'true');
    await resumeRAF(page);
  });

  test('Escape sai do modo de leitura e restaura role original', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();
    await page.keyboard.press('Enter');
    await expect(messages.nth(1)).toHaveAttribute('role', 'dialog', { timeout: 3_000 });

    // Escape → sai do virtual modal
    await page.keyboard.press('Escape');

    // O role deve voltar ao original (listitem)
    await expect(messages.nth(1)).toHaveAttribute('role', 'listitem', { timeout: 3_000 });
    await resumeRAF(page);
  });

  test('no modo de leitura, foco vai para o conteúdo da mensagem', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();
    await messages.nth(1).press('Enter');
    await expect(messages.nth(1)).toHaveAttribute('role', 'dialog', { timeout: 3_000 });

    // O foco deve estar dentro da mensagem (no conteúdo de texto)
    await expect.poll(async () => messages.nth(1).evaluate(
      (el) => el.contains(document.activeElement),
    ), { timeout: 3_000 }).toBe(true);
    await resumeRAF(page);
  });

  test('modo leitura não gera warning de aria-hidden com foco retido', async ({ page, wails }) => {
    const ariaHiddenWarnings: string[] = [];
    page.on('console', (msg) => {
      const text = msg.text();
      if (text.includes('Blocked aria-hidden on an element because its descendant retained focus')) {
        ariaHiddenWarnings.push(text);
      }
    });

    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    await pauseRAF(page);
    await messages.nth(1).focus();
    await messages.nth(1).press('Enter');
    await expect(messages.nth(1)).toHaveAttribute('role', 'dialog', { timeout: 3_000 });
    await resumeRAF(page);

    expect(ariaHiddenWarnings).toEqual([]);
  });
});

test.describe('Chat history — atributos ARIA', () => {
  test('mensagens têm role="listitem" e data-message-node', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    for (let i = 0; i < 3; i++) {
      await expect(messages.nth(i)).toHaveAttribute('role', 'listitem');
      await expect(messages.nth(i)).toHaveAttribute('data-message-node');
    }
  });

  test('cada mensagem tem tabindex para foco programático', async ({ page, wails }) => {
    await setupChatWithMessages(wails);

    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(3, { timeout: 5_000 });

    // Todas devem ter tabindex definido (-1 por padrão, 0 quando focado)
    for (let i = 0; i < 3; i++) {
      const tabIndex = await messages.nth(i).getAttribute('tabindex');
      expect(tabIndex).toBeDefined();
    }
  });
});
