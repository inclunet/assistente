import { test, expect } from '../fixtures';

/**
 * Testes de navegação por landmarks usando F6/Shift+F6.
 *
 * O app segue o padrão VS Code: F6 avança entre landmarks,
 * Shift+F6 retorna, Escape volta à área padrão (contentArea → textarea).
 *
 * Landmarks da rota workspace (em ordem):
 *   topbar → workspaceToolbar → workspaceTabs → contentToolbar → contentArea
 */

function messagesFixture() {
  const now = new Date().toISOString();
  return [
    {
      message: { id: '01926b90-0000-7000-8000-100000000001', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'user', content: 'Olá', createdAt: now },
      children: [],
      childCount: 0,
    },
    {
      message: { id: '01926b90-0000-7000-8000-100000000002', conversationId: '01926b90-0000-7000-8000-000000000001', role: 'assistant', content: 'Oi!', createdAt: now },
      children: [],
      childCount: 0,
    },
  ];
}

function activeChatTextarea(page: import('@playwright/test').Page) {
  return page.locator('.ws-content__panel[data-active="true"] .chat-input__textarea');
}

test.describe('Landmark navigation — F6 / Shift+F6', () => {
  test('página foca no textarea (área padrão) ao carregar', async ({ page, wails }) => {
    await wails.waitForApp();

    // contentArea é a landmark padrão → foca no textarea do chat
    const textarea = activeChatTextarea(page);
    await expect(textarea).toBeFocused({ timeout: 5_000 });
  });

  test('F6 avança pelas landmarks e cicla de volta ao contentArea', async ({ page, wails }) => {
    await wails.setResponse('GetMessages', messagesFixture());
    await wails.waitForApp();

    // Garante que o foco inicial é no textarea (contentArea)
    const textarea = activeChatTextarea(page);
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // Coleta quais landmarks recebem foco ao pressionar F6 repetidamente.
    // O app define 5 landmarks (topbar, workspaceToolbar, workspaceTabs,
    // contentToolbar, contentArea) mas pode pular as indisponíveis.
    const visited: string[] = [];

    for (let i = 0; i < 6; i++) {
      await page.keyboard.press('F6');
      const region = await page.evaluate(() => {
        const ae = document.activeElement;
        if (ae?.closest('.topbar')) return 'topbar';
        if (ae?.closest('.workspace-toolbar')) return 'workspaceToolbar';
        if (ae?.closest('.ws-tabs')) return 'workspaceTabs';
        if (ae?.closest('.ws-content-toolbar')) return 'contentToolbar';
        if (ae?.closest('.ws-content-area')) return 'contentArea';
        return 'unknown';
      });
      visited.push(region);
      // Se voltou ao contentArea, completou o ciclo
      if (region === 'contentArea') break;
    }

    // Deve ter visitado pelo menos 2 landmarks antes de voltar
    expect(visited.length).toBeGreaterThanOrEqual(2);

    // Deve incluir workspaceToolbar e workspaceTabs (sempre presentes)
    expect(visited).toContain('workspaceToolbar');
    expect(visited).toContain('workspaceTabs');

    // Deve terminar no contentArea (ciclo completo)
    expect(visited[visited.length - 1]).toBe('contentArea');
  });

  test('Shift+F6 navega na direção reversa', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = activeChatTextarea(page);
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // Shift+F6 deve ir para a landmark anterior (contentToolbar ou workspaceTabs)
    await page.keyboard.press('Shift+F6');
    const isBeforeContent = await page.evaluate(() => {
      const ae = document.activeElement;
      return (
        !!ae?.closest('.ws-content-toolbar') ||
        !!ae?.closest('.ws-tabs') ||
        !!ae?.closest('.workspace-toolbar') ||
        !!ae?.closest('.topbar')
      );
    });
    expect(isBeforeContent).toBe(true);
  });

  test('Escape retorna à área padrão (textarea do chat)', async ({ page, wails }) => {
    await wails.waitForApp();

    const textarea = activeChatTextarea(page);
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // Navega para outra landmark
    await page.keyboard.press('F6');
    await expect(textarea).not.toBeFocused();

    // Escape deve voltar para o textarea (default area)
    await page.keyboard.press('Escape');
    await expect(textarea).toBeFocused({ timeout: 3_000 });
  });

  test('trocar de aba restaura foco na área padrão', async ({ page, wails }) => {
    const now = new Date().toISOString();
    const ws = {
      id: 'ws-1',
      name: 'Workspace',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'tab-1',
        items: [
          { id: 'tab-1', type: 'chat', conversation_id: '01926b90-0000-7000-8000-000000000001', title: 'Aba 1', position: 0 },
          { id: 'tab-2', type: 'chat', conversation_id: '01926b90-0000-7000-8000-000000000002', title: 'Aba 2', position: 1 },
        ],
      },
    };
    await wails.setResponse('GetActiveWorkspace', ws);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.setResponse('EnsureConversation', {
      id: '01926b90-0000-7000-8000-000000000002', title: 'Aba 2', created_at: now, updated_at: now, messages: [], message_count: 0,
    });
    await wails.waitForApp();

    // Navega para a lista de abas via F6
    const tab2 = page.locator('.ws-tabs [role="tab"]').nth(1);
    await tab2.focus();

    // Clica na segunda aba
    await tab2.click();

    // O foco deve ir à área padrão (textarea)
    const textarea = activeChatTextarea(page);
    await expect(textarea).toBeFocused({ timeout: 5_000 });
  });
});
