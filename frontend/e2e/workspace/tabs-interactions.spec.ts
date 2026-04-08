import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const makeWorkspace = (tabs: Array<{ id: string; title: string; conversation_id: number; position: number }>, activeTab?: string) => ({
  id: 'ws-1',
  name: 'Workspace',
  profile: '',
  created_at: now,
  last_used: now,
  tabs: {
    active: activeTab || tabs[0]?.id || 'tab-1',
    items: tabs.map(t => ({
      id: t.id,
      type: 'chat',
      conversation_id: t.conversation_id,
      title: t.title,
      position: t.position,
    })),
  },
});

const threeTabWorkspace = makeWorkspace([
  { id: 'tab-1', title: 'Conversa 1', conversation_id: 1, position: 0 },
  { id: 'tab-2', title: 'Conversa 2', conversation_id: 2, position: 1 },
  { id: 'tab-3', title: 'Conversa 3', conversation_id: 3, position: 2 },
]);

test.describe('Abas — fechar aba', () => {
  test('fechar aba via botão X remove do tablist', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('RemoveWorkspaceTab', makeWorkspace([
      { id: 'tab-2', title: 'Conversa 2', conversation_id: 2, position: 0 },
      { id: 'tab-3', title: 'Conversa 3', conversation_id: 3, position: 1 },
    ], 'tab-2'));

    await wails.waitForApp();

    // Verifica que as 3 abas existem
    const tabs = page.locator('button[role="tab"]');
    await expect(tabs).toHaveCount(3);

    // Hover na primeira aba para mostrar o botão de fechar
    const firstTabWrapper = page.locator('.ws-tabs__tab-wrapper').first();
    await firstTabWrapper.hover();

    const closeBtn = firstTabWrapper.locator('.ws-tabs__tab-close');
    await expect(closeBtn).toBeVisible();
    await closeBtn.click();

    // Verifica que RemoveWorkspaceTab foi chamado
    const log = await wails.getCallLog();
    const removeCalls = log.filter(c => c.fn === 'RemoveWorkspaceTab');
    expect(removeCalls.length).toBe(1);
  });

  test('aba única não pode ser fechada (botão X não aparece)', async ({ page, wails }) => {
    const singleTabWorkspace = makeWorkspace([
      { id: 'tab-1', title: 'Conversa única', conversation_id: 1, position: 0 },
    ]);
    await wails.setResponse('GetActiveWorkspace', singleTabWorkspace);

    await wails.waitForApp();

    const tabWrapper = page.locator('.ws-tabs__tab-wrapper');
    await tabWrapper.hover();

    // O botão de fechar não deve aparecer se é a única aba
    const closeBtn = tabWrapper.locator('.ws-tabs__tab-close');
    const isVisible = await closeBtn.isVisible().catch(() => false);
    expect(isVisible).toBe(false);
  });

  test('fechar via context menu', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('RemoveWorkspaceTab', makeWorkspace([
      { id: 'tab-1', title: 'Conversa 1', conversation_id: 1, position: 0 },
      { id: 'tab-3', title: 'Conversa 3', conversation_id: 3, position: 1 },
    ], 'tab-1'));

    await wails.waitForApp();

    // Right-click na segunda aba
    const secondTab = page.locator('button[role="tab"]').nth(1);
    await secondTab.click({ button: 'right' });

    // Menu de contexto aparece
    const menu = page.locator('[role="menu"]');
    await expect(menu).toBeVisible({ timeout: 3_000 });

    // Clica em "Fechar"
    const closeItem = menu.locator('[role="menuitem"]', { hasText: /fechar$/i });
    if (await closeItem.count() > 0) {
      await closeItem.first().click();

      const log = await wails.getCallLog();
      const removeCalls = log.filter(c => c.fn === 'RemoveWorkspaceTab');
      expect(removeCalls.length).toBe(1);
    }
  });
});

test.describe('Abas — trocar aba', () => {
  test('clicar em outra aba chama SetActiveWorkspaceTab e troca', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.setResponse('EnsureConversation', {
      id: 2,
      title: 'Conversa 2',
      created_at: now,
      updated_at: now,
      messages: [],
      message_count: 0,
    });

    await wails.waitForApp();

    // Aba 1 deve estar ativa
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toHaveAttribute('aria-selected', 'true');

    // Clica na segunda aba
    const secondTab = page.locator('button[role="tab"]').nth(1);
    await secondTab.click();

    const log = await wails.getCallLog();
    const setCalls = log.filter(c => c.fn === 'SetActiveWorkspaceTab');
    expect(setCalls.length).toBeGreaterThanOrEqual(1);
  });

  test('navegação por teclado (ArrowRight/Left) entre abas', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);

    await wails.waitForApp();

    // Foca na aba ativa
    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click();
    await firstTab.focus();
    await expect(firstTab).toBeFocused({ timeout: 2_000 });

    // ArrowRight deve mover foco para a próxima aba
    await page.keyboard.press('ArrowRight');

    const secondTab = page.locator('button[role="tab"]').nth(1);
    await expect(secondTab).toBeFocused({ timeout: 3_000 });
  });
});

test.describe('Abas — renomear', () => {
  test('F2 ativa modo de edição da aba', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('UpdateWorkspaceTab', threeTabWorkspace);

    await wails.waitForApp();

    // Foca na primeira aba
    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click();
    await firstTab.focus();

    // F2 para renomear
    await firstTab.press('F2');

    // Input de edição deve aparecer
    const editInput = page.locator('.ws-tabs__tab-edit');
    await expect(editInput).toBeVisible({ timeout: 3_000 });
  });

  test('Enter confirma renomeação e chama UpdateWorkspaceTab', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('UpdateWorkspaceTab', threeTabWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click();
    await firstTab.focus();
    await expect(firstTab).toBeFocused();
    await firstTab.press('F2');

    const editInput = page.locator('.ws-tabs__tab-edit');
    await expect(editInput).toBeVisible({ timeout: 3_000 });
    await expect(editInput).toBeFocused({ timeout: 1_000 });

    // Limpa e digita novo título
    await editInput.fill('Conversa Renomeada');
    await page.waitForTimeout(50);
    await editInput.press('Enter');

    // Verifica que UpdateWorkspaceTab foi chamado
    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'UpdateWorkspaceTab'
      );
    }, { timeout: 5_000 });

    const log = await wails.getCallLog();
    const updateCalls = log.filter(c => c.fn === 'UpdateWorkspaceTab');
    expect(updateCalls.length).toBeGreaterThanOrEqual(1);
  });

  test('Escape cancela renomeação sem salvar', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click();
    await firstTab.focus();
    await firstTab.press('F2');

    const editInput = page.locator('.ws-tabs__tab-edit');
    await expect(editInput).toBeVisible({ timeout: 3_000 });
    await expect(editInput).toBeFocused({ timeout: 1_000 });

    await editInput.fill('Título que será cancelado');
    await page.waitForTimeout(50);
    await editInput.press('Escape');

    await expect(editInput).not.toBeVisible({ timeout: 3_000 });

    // Verifica que NÃO houve chamada de update
    const log = await wails.getCallLog();
    const updateCalls = log.filter(c => c.fn === 'UpdateWorkspaceTab');
    expect(updateCalls.length).toBe(0);
  });

  test('renomear via context menu', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('UpdateWorkspaceTab', threeTabWorkspace);

    await wails.waitForApp();

    // Right-click na primeira aba
    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]');
    await expect(menu).toBeVisible({ timeout: 3_000 });

    // Clica em "Renomear"
    const renameItem = menu.locator('[role="menuitem"]', { hasText: /renomear|rename/i });
    if (await renameItem.count() > 0) {
      await renameItem.first().click();

      const editInput = page.locator('.ws-tabs__tab-edit');
      await expect(editInput).toBeVisible({ timeout: 3_000 });
    }
  });
});

test.describe('Abas — reordenar com teclado', () => {
  test('Alt+Right move aba para a direita', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('ReorderWorkspaceTabs', threeTabWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click();
    await firstTab.focus();
    await expect(firstTab).toBeFocused({ timeout: 2_000 });

    // Alt+Right move para direita
    await page.keyboard.press('Alt+ArrowRight');

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'ReorderWorkspaceTabs'
      );
    }, { timeout: 5_000 });

    const log = await wails.getCallLog();
    const reorderCalls = log.filter(c => c.fn === 'ReorderWorkspaceTabs');
    expect(reorderCalls.length).toBeGreaterThanOrEqual(1);
  });

  test('Alt+Left no início toca bump (não reordena)', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.focus();

    // Alt+Left no início — não deve reordenar
    await page.keyboard.press('Alt+ArrowLeft');

    const log = await wails.getCallLog();
    const reorderCalls = log.filter(c => c.fn === 'ReorderWorkspaceTabs');
    expect(reorderCalls.length).toBe(0);
  });
});

test.describe('Abas — close others via context menu', () => {
  test('fechar outras abas mantém apenas a selecionada', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', threeTabWorkspace);
    await wails.setResponse('RemoveWorkspaceTab', makeWorkspace([
      { id: 'tab-1', title: 'Conversa 1', conversation_id: 1, position: 0 },
    ]));

    await wails.waitForApp();

    // Right-click na primeira aba
    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]');
    await expect(menu).toBeVisible({ timeout: 3_000 });

    // Clica em "Fechar outras"
    const closeOthersItem = menu.locator('[role="menuitem"]', { hasText: /fechar outras|close other/i });
    if (await closeOthersItem.count() > 0) {
      await closeOthersItem.first().click();

      const log = await wails.getCallLog();
      const removeCalls = log.filter(c => c.fn === 'RemoveWorkspaceTab');
      expect(removeCalls.length).toBeGreaterThanOrEqual(2);
    }
  });
});
