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
          state: { filePath: 'C:/tmp/manual.md' },
        }],
      },
    });
    await wails.setResponse('EditorReadFile', {
      path: 'C:/tmp/manual.md',
      content: '# Manual\n\n[Link do documento](https://example.com)',
      projected: false,
      format: 'md',
      readOnly: false,
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

    await page.keyboard.press('Alt+1');
    const monacoInput = page.getByRole('textbox', { name: 'Markdown editor' });
    await expect(monacoInput).toBeVisible({ timeout: 15_000 });
    await monacoInput.focus();
    await expect(monacoInput).toBeFocused();
    await page.evaluate(() => {
      const focusTrace: Array<{ focusin: string; activeElement: string }> = [];
      const identify = (element: Element | null) => {
        if (element?.matches('[data-editor-rendered-anchor="true"]')) return 'anchor';
        if (element?.matches('[data-editor-rendered-document="true"]')) return 'document';
        if (element?.matches('[aria-label="Markdown editor"]')) return 'monaco';
        if (element?.matches('.ProseMirror')) return 'tiptap';
        return element?.tagName.toLowerCase() ?? 'none';
      };
      document.addEventListener('focusin', (event) => {
        focusTrace.push({
          focusin: identify(event.target as Element),
          activeElement: identify(document.activeElement),
        });
      });
      Object.assign(window, { __editorRenderedFocusTrace: focusTrace });
    });

    await page.keyboard.press('Alt+3');

    await expect(anchor).toHaveAttribute('role', 'group');
    await expect(anchor).toHaveAttribute('tabindex', '-1');
    await expect(document).toHaveAttribute('role', 'document');
    await expect(document).toHaveAttribute('tabindex', '0');
    await expect(document).toBeFocused();
    await expect(documentLink).toHaveAttribute('tabindex', '0');
    await expect(anchor.locator('[data-editor-rendered-document="true"]')).toHaveCount(1);
    await expect(document.locator('.editor-page__toolbar')).toHaveCount(0);
    await expect.poll(() => page.evaluate(() => (
      (window as unknown as {
        __editorRenderedFocusTrace: Array<{ focusin: string; activeElement: string }>;
      }).__editorRenderedFocusTrace
    ))).toEqual([
      { focusin: 'anchor', activeElement: 'anchor' },
      { focusin: 'document', activeElement: 'document' },
    ]);

    await page.keyboard.press('Alt+2');
    const tiptapEditor = page.locator('.ProseMirror').first();
    await expect(tiptapEditor).toBeVisible({ timeout: 15_000 });
    await tiptapEditor.focus();
    await expect(tiptapEditor).toBeFocused();
    await page.evaluate(() => {
      (window as unknown as {
        __editorRenderedFocusTrace: Array<{ focusin: string; activeElement: string }>;
      }).__editorRenderedFocusTrace.length = 0;
    });
    await page.keyboard.press('Alt+3');
    await expect(document).toBeFocused();
    await expect.poll(() => page.evaluate(() => (
      (window as unknown as {
        __editorRenderedFocusTrace: Array<{ focusin: string; activeElement: string }>;
      }).__editorRenderedFocusTrace
    ))).toEqual([
      { focusin: 'anchor', activeElement: 'anchor' },
      { focusin: 'document', activeElement: 'document' },
    ]);

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
