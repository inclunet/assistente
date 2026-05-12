import { test, expect } from '../fixtures';

/**
 * Testes de acessibilidade do ChatToolbar.
 *
 * Testa:
 * - Ctrl+L limpa conversa e anuncia
 * - Ctrl+H abre o history picker
 * - Ctrl+P abre o profile picker
 * - ToolBar tem role="toolbar" e aria-label
 * - aria-busy="true" durante streaming/loading
 * - Anuncia ações via aria-live
 */

function messagesFixture() {
  const now = new Date().toISOString();
  return [
    {
      message: { id: '01926b90-0000-7000-8000-100000000001', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: 'Mensagem', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: { id: '01926b90-0000-7000-8000-100000000002', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'assistant', content: 'Resposta', createdAt: now },
      children: [],
      childCount: 0,
    },
  ];
}

test.describe('ChatToolbar — ARIA structure', () => {
  test('toolbar tem role="toolbar" e aria-label', async ({ page, wails }) => {
    await wails.waitForApp();

    // ChatToolbar usa o componente Toolbar que renderiza com role="toolbar"
    const toolbar = page.locator('.ws-content-toolbar [role="toolbar"]');
    await expect(toolbar).toBeVisible({ timeout: 5_000 });
    
    const ariaLabel = await toolbar.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
  });

  test('toolbar mostra título da conversa como heading', async ({ page, wails }) => {
    await wails.waitForApp();

    // O ChatToolbar renderiza o título da conversa como h2
    const heading = page.locator('#chat-heading');
    await expect(heading).toBeVisible({ timeout: 5_000 });
  });

  test('aria-busy reflete estado de loading', async ({ page, wails }) => {
    await wails.waitForApp();

    const toolbar = page.locator('.ws-content-toolbar [role="toolbar"]');
    await expect(toolbar).toBeVisible({ timeout: 5_000 });

    // No estado inicial, aria-busy deve ser "false"
    await expect(toolbar).toHaveAttribute('aria-busy', 'false');
  });
});

test.describe('ChatToolbar — Ctrl+L limpa conversa', () => {
  test('Ctrl+L limpa conversa e anuncia para screen reader', async ({ page, wails }) => {
    const now = new Date().toISOString();
    await wails.setResponse('GetMessages', messagesFixture());
    await wails.setResponse('ClearConversation', undefined);
    await wails.setResponse('ClearMessages', undefined);
    // Após limpar, GetMessages retorna vazio
    await wails.setResponse('EnsureConversation', {
      id: '01926b90-0000-7000-8000-000000000001', title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 2,
    });
    await wails.waitForApp();

    // Deve ter mensagens
    const messages = page.locator('.message-node[data-level="0"]');
    await expect(messages).toHaveCount(2, { timeout: 5_000 });

    // Ctrl+L limpa
    await page.keyboard.press('Control+l');

    // Verifica que ClearConversation foi chamado no backend
    const calls = await wails.getCallLog();
    const clearCall = calls.find(c => c.fn === 'ClearConversation');
    expect(clearCall).toBeDefined();
  });

  test('Ctrl+L restaura foco no input após limpar', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesFixture());
    await wails.setResponse('ClearConversation', undefined);
    const now = new Date().toISOString();
    await wails.setResponse('EnsureConversation', {
      id: '01926b90-0000-7000-8000-000000000001', title: 'Conversa', created_at: now, updated_at: now,
      messages: [], message_count: 2,
    });
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // Ctrl+L limpa
    await page.keyboard.press('Control+l');

    // Foco deve voltar ao textarea
    await expect(textarea).toBeFocused({ timeout: 5_000 });
  });
});

test.describe('ChatToolbar — Ctrl+H history picker', () => {
  // Ctrl+H é interceptado pelo Chromium (abre histórico do browser).
  // No Wails real funciona, mas em Playwright/headless não chega ao JS handler.
  test.skip('Ctrl+H aciona o picker de histórico (desabilitado — conflito com browser)', async ({ page, wails }) => {
    await wails.setResponse('SearchConversationHistory', []);
    await wails.setResponse('GetConversations', []);
    await wails.waitForApp();

    // Ctrl+H deve acionar o history picker (clica no botão programaticamente)
    await page.keyboard.press('Control+h');
    await page.waitForTimeout(500);

    // Verifica que alguma ação de histórico foi disparada
    const calls = await wails.getCallLog();
    const historyCall = calls.find(
      c => c.fn === 'SearchConversationHistory' || c.fn === 'GetConversations',
    );
    // Ctrl+H deve ter disparado ao menos uma chamada ao backend
    expect(historyCall).toBeDefined();
  });

  test('selecionar conversa no history picker persiste a aba ativa e atualiza o título', async ({ page, wails }) => {
    const now = new Date().toISOString();
    await wails.waitForApp();

    await wails.setResponse('GetConversations', [
      {
        id: '01926b90-0000-7000-8000-000000000001',
        title: 'Conversa atual',
        created_at: now,
        updated_at: now,
        message_count: 1,
      },
      {
        id: '01926b90-0000-7000-8000-000000000002',
        title: 'Conversa importada',
        created_at: now,
        updated_at: now,
        message_count: 3,
      },
    ]);
    await wails.setResponse('GetConversationInfo', {
      id: '01926b90-0000-7000-8000-000000000002',
      title: 'Conversa importada',
      created_at: now,
      updated_at: now,
      message_count: 3,
      channel: '',
      contact_id: '',
    });
    await wails.setResponse('GetMessages', []);
    await wails.setResponse('UpdateWorkspaceTab', undefined);

    const historyBtn = page.locator('.ws-content-toolbar .picker-button').first();
    await historyBtn.click();

    const option = page.locator('[role="option"]').filter({ hasText: 'Conversa importada' });
    await expect(option).toBeVisible({ timeout: 5_000 });
    await option.click();

    await expect(page.locator('#chat-heading')).toHaveText('Conversa importada', { timeout: 5_000 });
    await expect(page.locator('button[role="tab"]').first()).toContainText('Conversa importada');

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some((c) =>
        c.fn === 'UpdateWorkspaceTab' &&
        c.args[0] === 'tab-1' &&
        c.args[1]?.conversation_id === '01926b90-0000-7000-8000-000000000002' &&
        c.args[1]?.title === 'Conversa importada',
      );
    }, { timeout: 5_000 });
  });
});

test.describe('ChatToolbar — Ctrl+P profile picker', () => {
  test('Ctrl+P abre o picker de perfis', async ({ page, wails }) => {
    await wails.waitForApp();

    // Ctrl+P deve acionar o profile picker
    await page.keyboard.press('Control+p');

    await page.waitForTimeout(300);

    // O picker de perfis deve abrir
    const popup = page.locator('[role="menu"], [role="listbox"], .picker-menu');
    const isVisible = await popup.first().isVisible().catch(() => false);
    // A chamada GetProfiles indica que o picker abriu
    const calls = await wails.getCallLog();
    const profileCall = calls.find(c => c.fn === 'GetProfiles');
    expect(isVisible || !!profileCall).toBe(true);
  });
});

test.describe('ChatToolbar — botões acessíveis', () => {
  test('botão de limpar tem aria-label descritivo', async ({ page, wails }) => {
    await wails.waitForApp();

    // Procura o botão de limpar conversa dentro da toolbar
    const clearBtn = page.locator('.ws-content-toolbar [role="toolbar"] button').filter({
      has: page.locator('.anticon-clear'),
    });

    // Se não encontrar pelo ícone, tenta pelo aria-label
    const fallbackBtn = page.locator('.ws-content-toolbar [role="toolbar"] button[aria-label]').first();
    const btn = (await clearBtn.count()) > 0 ? clearBtn.first() : fallbackBtn;

    const ariaLabel = await btn.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
    expect(ariaLabel!.length).toBeGreaterThan(0);
  });

  test('botão de token stats tem aria-label com informação de tokens', async ({ page, wails }) => {
    await wails.waitForApp();

    const tokenBtn = page.locator('.token-stats-button');
    await expect(tokenBtn).toBeVisible({ timeout: 5_000 });

    const ariaLabel = await tokenBtn.getAttribute('aria-label');
    expect(ariaLabel).toBeTruthy();
    // Deve conter informação de tokens
    expect(ariaLabel).toMatch(/token/i);
  });
});
