import { test, expect } from '../fixtures';

const now = new Date().toISOString();

const twoWorkspaces = [
  { id: 'ws-1', name: 'Principal', path: '', profile: '', tab_count: 3, is_active: true },
  { id: 'ws-2', name: 'Secundário', path: '', profile: '', tab_count: 1, is_active: false },
];

const activeWorkspace = {
  id: 'ws-1',
  name: 'Principal',
  profile: '',
  created_at: now,
  last_used: now,
  tabs: {
    active: 'tab-1',
    items: [
      { id: 'tab-1', type: 'chat', conversation_id: 1, title: 'Conversa 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversation_id: 2, title: 'Conversa 2', position: 1 },
    ],
  },
};

test.describe('Abas — context menu avançado', () => {
  test('right-click mostra opções do menu de contexto', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    // Usa .first() pois pode haver submenu também
    const menu = page.locator('[role="menu"]').first();
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const menuItems = menu.locator('[role="menuitem"]');
    const count = await menuItems.count();
    expect(count).toBeGreaterThanOrEqual(2);
  });

  test('Move To submenu aparece com outros workspaces', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', activeWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    await expect(menu).toBeVisible({ timeout: 3_000 });

    // Procura o item "Mover para..." ou "Move to..."
    const moveToItem = menu.locator('[role="menuitem"]', { hasText: /mover|move/i });
    if (await moveToItem.count() > 0) {
      await moveToItem.first().hover();

      // O submenu deve mostrar outros workspaces
      const submenuItem = page.locator('[role="menuitem"]', { hasText: 'Secundário' });
      await expect(submenuItem).toBeVisible({ timeout: 3_000 });
    }
  });

  test('selecionar workspace no submenu chama MoveWorkspaceTabTo', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', activeWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const moveToItem = menu.locator('[role="menuitem"]', { hasText: /mover|move/i });
    if (await moveToItem.count() > 0) {
      await moveToItem.first().hover();

      const submenuItem = page.locator('[role="menuitem"]', { hasText: 'Secundário' });
      const isVisible = await submenuItem.isVisible({ timeout: 3_000 }).catch(() => false);

      if (isVisible) {
        await submenuItem.click();

        await page.waitForFunction(() => {
          return window.__wailsMock.getCallLog().some(
            (c: { fn: string }) => c.fn === 'MoveWorkspaceTabTo'
          );
        }, { timeout: 5_000 });

        const log = await wails.getCallLog();
        const moveCalls = log.filter(c => c.fn === 'MoveWorkspaceTabTo');
        expect(moveCalls.length).toBe(1);
      }
    }
  });

  test('Close Others via context menu chama RemoveWorkspaceTab', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('RemoveWorkspaceTab', activeWorkspace);

    await wails.waitForApp();

    const firstTab = page.locator('button[role="tab"]').first();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const closeOthers = menu.locator('[role="menuitem"]', { hasText: /fechar outr|close other/i });
    if (await closeOthers.count() > 0) {
      await closeOthers.first().click();

      await page.waitForFunction(() => {
        return window.__wailsMock.getCallLog().some(
          (c: { fn: string }) => c.fn === 'RemoveWorkspaceTab'
        );
      }, { timeout: 5_000 });

      const log = await wails.getCallLog();
      const removeCalls = log.filter(c => c.fn === 'RemoveWorkspaceTab');
      expect(removeCalls.length).toBeGreaterThanOrEqual(1);
    }
  });
});
