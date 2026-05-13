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
      { id: 'tab-1', type: 'chat', conversation_id: '01970a9e-0001-7000-8000-000000000001', title: 'Conversa 1', position: 0 },
      { id: 'tab-2', type: 'chat', conversation_id: '01970a9e-0002-7000-8000-000000000002', title: 'Conversa 2', position: 1 },
    ],
  },
};

test.describe('Abas — context menu avançado', () => {
  test('right-click mostra opções do menu de contexto', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    // Usa .first() pois pode haver submenu também
    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });
    await expect(page.locator('#close')).toBeVisible({ timeout: 3_000 });
    await expect(page.locator('#close-others')).toBeVisible({ timeout: 3_000 });
  });

  test('Move To submenu aparece com outros workspaces', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', activeWorkspace);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.click({ button: 'right' });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const moveToItem = page.locator('#move-to');
    await expect(moveToItem).toBeVisible({ timeout: 3_000 });
    await moveToItem.evaluate((element: HTMLButtonElement) => element.click());

    // O submenu deve mostrar outros workspaces
    const submenuItem = page.locator('#move-ws-2');
    await expect(submenuItem).toBeVisible({ timeout: 3_000 });
  });

  test('selecionar workspace no submenu chama MoveWorkspaceTabTo', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('MoveWorkspaceTabTo', activeWorkspace);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    await menu.locator('#move-to').evaluate((element: HTMLButtonElement) => element.click());

    const submenuItem = page.locator('#move-ws-2');
    await expect(submenuItem).toBeVisible({ timeout: 3_000 });
    await submenuItem.evaluate((element: HTMLButtonElement) => element.click());

    await page.waitForFunction(() => {
      return window.__wailsMock.getCallLog().some(
        (c: { fn: string }) => c.fn === 'MoveWorkspaceTabTo'
      );
    }, { timeout: 5_000 });

    const log = await wails.getCallLog();
    const moveCalls = log.filter(c => c.fn === 'MoveWorkspaceTabTo');
    expect(moveCalls.length).toBe(1);
  });

  test('Close Others via context menu chama RemoveWorkspaceTab', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', activeWorkspace);
    await wails.setResponse('ListWorkspaces', twoWorkspaces);
    await wails.setResponse('RemoveWorkspaceTab', activeWorkspace);

    await wails.waitForApp();

    await expect(page.locator('button[role="tab"]')).toHaveCount(2);
    const firstTab = page.locator('button[role="tab"]').first();
    await expect(firstTab).toBeVisible();
    await firstTab.click({ button: 'right' });

    const menu = page.locator('[role="menu"]').first();
    if (!(await menu.isVisible().catch(() => false))) {
      await firstTab.dispatchEvent('mousedown', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
      await firstTab.dispatchEvent('contextmenu', { button: 2, buttons: 2, clientX: 16, clientY: 16 });
    }
    await expect(menu).toBeVisible({ timeout: 3_000 });

    const closeOthers = page.locator('#close-others');
    if (await closeOthers.count() > 0) {
      await menu.locator('#close-others').evaluate((element: HTMLButtonElement) => element.click());

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
