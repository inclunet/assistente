import { test, expect } from '../fixtures';

const now = new Date().toISOString();

test.describe('Abas do workspace — renderização', () => {
  test('lista de abas está visível', async ({ page, wails }) => {
    await wails.waitForApp();

    const tabList = page.locator('.ws-tabs__list');
    await expect(tabList).toBeVisible();
  });

  test('aba ativa tem classe --active', async ({ page, wails }) => {
    await wails.waitForApp();

    const activeTab = page.locator('.ws-tabs__tab--active');
    await expect(activeTab).toBeVisible();
  });

  test('aba mostra título da conversa', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'tab-1',
        items: [
          {
            id: 'tab-1',
            type: 'chat',
            conversation_id: '1',
            title: 'Minha conversa',
            position: 0,
          },
        ],
      },
    });

    await wails.waitForApp();

    const tabTitle = page.locator('.ws-tabs__tab-title');
    await expect(tabTitle).toContainText('Minha conversa');
  });
});

test.describe('Abas do workspace — múltiplas abas', () => {
  const workspaceWithTabs = {
    id: 'ws-1',
    name: 'Workspace',
    profile: '',
    created_at: now,
    last_used: now,
    tabs: {
      active: 'tab-1',
      items: [
        {
          id: 'tab-1',
          type: 'chat',
          conversation_id: '1',
          title: 'Conversa 1',
          position: 0,
        },
        {
          id: 'tab-2',
          type: 'chat',
          conversation_id: '2',
          title: 'Conversa 2',
          position: 1,
        },
        {
          id: 'tab-3',
          type: 'chat',
          conversation_id: '3',
          title: 'Conversa 3',
          position: 2,
        },
      ],
    },
  };

  test('renderiza múltiplas abas', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    const tabs = page.locator('.ws-tabs__tab');
    await expect(tabs).toHaveCount(3);
  });

  test('clicar em outra aba chama SetActiveWorkspaceTab', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.setResponse('SetActiveWorkspaceTab', undefined);
    await wails.waitForApp();

    // Clica na segunda aba
    const secondTab = page.locator('.ws-tabs__tab').nth(1);
    await secondTab.click();

    const log = await wails.getCallLog();
    const setCalls = log.filter(c => c.fn === 'SetActiveWorkspaceTab');
    expect(setCalls.length).toBeGreaterThanOrEqual(1);
  });

  test('botão de fechar aba está visível quando há múltiplas abas', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    // Hover na aba para mostrar botão de fechar
    const firstTab = page.locator('.ws-tabs__tab-wrapper').first();
    await firstTab.hover();

    const closeBtn = firstTab.locator('.ws-tabs__tab-close');
    await expect(closeBtn).toBeVisible();
  });

  test('abas têm role=tab para acessibilidade', async ({ page, wails }) => {
    await wails.setResponse('GetActiveWorkspace', workspaceWithTabs);
    await wails.waitForApp();

    const tabButtons = page.locator('button[role="tab"]');
    const count = await tabButtons.count();
    expect(count).toBeGreaterThanOrEqual(3);
  });
});
