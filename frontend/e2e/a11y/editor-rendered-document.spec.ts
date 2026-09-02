import { test, expect } from '../fixtures';

const now = new Date().toISOString();

test.describe('Preview renderizado do editor — ilha documental', () => {
  test.beforeEach(async ({ wails }) => {
    await wails.setResponse('GetActiveWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'editor-tab',
        items: [{
          id: 'editor-tab',
          type: 'editor',
          title: 'Manual',
          position: 0,
          state: { filePath: 'C:/tmp/manual.pdf' },
        }],
      },
    });
    await wails.setResponse('EditorReadFile', {
      path: 'C:/tmp/manual.pdf',
      content: '# Manual\n\n[Link do documento](https://example.com)',
      projected: true,
      format: 'pdf',
      readOnly: true,
      warnings: [],
    });
    await wails.setResponse('EditorWatchFile', null);
    await wails.setResponse('EditorUnwatchFile', null);
    await wails.setResponse('EditorSaveState', null);
  });

  test('Alt+3 entra e retorna diretamente ao documento sem Enter adicional', async ({
    page,
    wails,
  }) => {
    await wails.waitForApp();

    const anchor = page.locator('[data-editor-rendered-anchor="true"]');
    const document = page.locator('[data-editor-rendered-document="true"]');
    const documentLink = document.getByRole('link', { name: 'Link do documento' });

    await expect(anchor).toHaveAttribute('role', 'group');
    await expect(anchor).toHaveAttribute('tabindex', '0');
    await expect(document).not.toHaveAttribute('role');
    await expect(document).not.toHaveAttribute('tabindex');
    await expect(anchor.locator('[data-editor-rendered-document="true"]')).toHaveCount(1);
    await expect(document.locator('.editor-page__toolbar')).toHaveCount(0);

    await page.keyboard.press('Alt+3');

    await expect(anchor).toHaveAttribute('tabindex', '-1');
    await expect(document).toHaveAttribute('role', 'document');
    await expect(document).toHaveAttribute('tabindex', '0');
    await expect(document).toBeFocused();
    await expect(documentLink).toHaveAttribute('tabindex', '0');

    const tabIsFree = await page.evaluate(() => window.dispatchEvent(
      new KeyboardEvent('keydown', { key: 'Tab', bubbles: true, cancelable: true }),
    ));
    const shiftTabIsFree = await page.evaluate(() => window.dispatchEvent(
      new KeyboardEvent('keydown', {
        key: 'Tab',
        shiftKey: true,
        bubbles: true,
        cancelable: true,
      }),
    ));
    expect(tabIsFree).toBe(true);
    expect(shiftTabIsFree).toBe(true);

    await documentLink.focus();
    await page.keyboard.press('Tab');
    await expect(documentLink).not.toBeFocused();
    await expect(anchor).not.toBeFocused();
    await expect(document).not.toBeFocused();
    await expect(document).toHaveAttribute('role', 'document');

    await document.focus();
    await page.keyboard.press('F6');
    await expect(document).not.toBeFocused();
    const focusLeftDocument = await document.evaluate(
      (element) => !element.contains(window.document.activeElement),
    );
    expect(focusLeftDocument).toBe(true);
    await expect(document).toHaveAttribute('role', 'document');

    await page.keyboard.press('Alt+3');
    await expect(document).toBeFocused();

    const toolbarButton = page.locator('.editor-page__toolbar button:not([disabled])').first();
    await toolbarButton.focus();
    await page.keyboard.press('Escape');
    await expect(document).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(document).toBeFocused();
    await expect(document).toHaveAttribute('role', 'document');
  });
});
