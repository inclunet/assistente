import { test, expect } from '../fixtures';

const now = new Date().toISOString();
const editorKeyboardMap = {
  generation: 'editor-rendered-keyboard-map',
  validUntil: Date.now() + 30 * 60 * 1000,
  ownerId: 'user-e2e',
  sessionId: 'session-e2e',
  workspaceId: 'ws-1',
  bindings: [
    { shortcut: { version: 1, code: 'Tab', modifiers: ['Control'] }, commandId: 'workspace.tab.next', handler: 'local_ui' },
    { shortcut: { version: 1, code: 'Tab', modifiers: ['Control', 'Shift'] }, commandId: 'workspace.tab.previous', handler: 'local_ui' },
    { shortcut: { version: 1, code: 'PageDown', modifiers: ['Control'] }, commandId: 'workspace.tab.next', handler: 'local_ui' },
    { shortcut: { version: 1, code: 'PageUp', modifiers: ['Control'] }, commandId: 'workspace.tab.previous', handler: 'local_ui' },
    { shortcut: { version: 1, code: 'F6', modifiers: [] }, commandId: 'navigation.landmark.next', handler: 'local_ui' },
  ],
  contextualBindings: ['Digit1', 'Digit2', 'Digit3'].map((code, index) => {
    const shortcut = { version: 1, code, modifiers: ['Alt'] };
    const commandId = ['editor.mode.markdown', 'editor.mode.rich', 'editor.mode.view'][index];
    return {
      shortcut,
      bySurface: { editor: { shortcut, commandId, handler: 'contextual' } },
      fallback: null,
    };
  }),
  localPaletteCommands: ['workspace.tab.next', 'workspace.tab.previous', 'navigation.landmark.next'],
};

test.describe('Preview renderizado do editor — ilha documental', () => {
  test.beforeEach(async ({ wails }) => {
    await wails.setResponse('GetLocalCommandKeyboardMap', editorKeyboardMap);
    await wails.setResponse('GetActiveWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      snapshot_epoch: 'editor-rendered-epoch',
      snapshot_sequence: '1',
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
    await wails.setResponse('UpdateWorkspaceTab', null);
  });

  test('Alt+3 entra e retorna diretamente ao documento sem Enter adicional', async ({
    page,
    wails,
  }) => {
    await wails.waitForApp();
    await page.evaluate(() => {
      const commandByCode: Record<string, string> = {
        Digit1: 'editor.mode.markdown',
        Digit2: 'editor.mode.rich',
        Digit3: 'editor.mode.view',
      };
      let next = 0;
      const reservations = new Map<string, {
        invocationId: string;
        commandId: string;
        handoffId: string;
        taken: boolean;
        committed: boolean;
      }>();
      window.__wailsMock.setResponse('BeginContextualLocalCommandUIKey', (
        _generation: string,
        shortcut: { code: string },
        repeat: boolean,
        context: { surfaceId: string; surfaceType: string },
      ) => {
        const commandId = commandByCode[shortcut?.code];
        if (repeat || !commandId || context?.surfaceId !== 'editor-tab' || context.surfaceType !== 'editor') return null;
        next++;
        const ticket = `editor-mode-ticket-${next}`;
        const reservation = {
          ticket,
          invocationId: `01926b90-0000-7000-8000-${String(next).padStart(12, '0')}`,
          commandId,
          handoffId: `${ticket}-handoff`,
          taken: false,
          committed: false,
        };
        reservations.set(ticket, reservation);
        return { ticket: reservation.ticket, invocationId: reservation.invocationId, commandId: reservation.commandId };
      });
      window.__wailsMock.setResponse('TakeUICommand', (ticket: string) => {
        const reservation = reservations.get(ticket);
        if (!reservation || reservation.taken) return null;
        reservation.taken = true;
        return {
          ticket,
          invocationId: reservation.invocationId,
          commandId: reservation.commandId,
          handoffId: reservation.handoffId,
        };
      });
      window.__wailsMock.setResponse('CommitWorkspaceTabCommand', (ticket: string, handoffId: string) => {
        const reservation = reservations.get(ticket);
        if (!reservation || !reservation.taken || reservation.committed || reservation.handoffId !== handoffId) {
          throw new Error('editor-mode-invalid-handoff');
        }
        reservation.committed = true;
      });
      window.__wailsMock.setResponse('GetUICommandResult', (ticket: string) => {
        const reservation = reservations.get(ticket);
        return reservation?.committed
          ? { invocationId: reservation.invocationId, status: 'succeeded' }
          : null;
      });
    });

    const anchor = page.locator('[data-editor-rendered-anchor="true"]');
    const document = page.locator('[data-editor-rendered-document="true"]');
    const documentLink = document.getByRole('link', { name: 'Link do documento' });
    const readCommandEffectCounts = async () => {
      const calls = await wails.getCallLog();
      const effectMethods = [
        'BeginContextualLocalCommandUIKey',
        'TakeUICommand',
        'CommitWorkspaceTabCommand',
        'GetUICommandResult',
        'CompleteUICommand',
      ];
      return effectMethods.map(method => calls.filter(({ fn }) => fn === method).length);
    };
    await expect(page.getByRole('toolbar', { name: 'Editor toolbar' })).toBeVisible({
      timeout: 30_000,
    });
    const monacoInput = page.getByRole('textbox', { name: 'Markdown editor' });
    await expect(monacoInput).toBeVisible();
    const beforeSameModeShortcut = await readCommandEffectCounts();
    await page.keyboard.press('Alt+1');
    expect(await readCommandEffectCounts()).toEqual(beforeSameModeShortcut);
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

    const countsBeforeViewRefocus = await readCommandEffectCounts();
    await page.keyboard.press('Alt+3');
    await expect(document).toBeFocused();
    expect(await readCommandEffectCounts()).toEqual(countsBeforeViewRefocus);

    const toolbarButton = page.locator('.editor-page__toolbar button:not([disabled])').first();
    await toolbarButton.focus();
    await page.keyboard.press('Escape');
    await expect(document).toBeFocused();

    await page.keyboard.press('Escape');
    await expect(document).toBeFocused();
    await expect(document).toHaveAttribute('role', 'document');

    await expect.poll(async () => (await wails.getCallLog()).filter(
      ({ fn }) => fn === 'GetUICommandResult',
    )).toHaveLength(3);
    const commandCalls = await wails.getCallLog();
    const begins = commandCalls.filter(({ fn }) => fn === 'BeginContextualLocalCommandUIKey');
    const takes = commandCalls.filter(({ fn }) => fn === 'TakeUICommand');
    const commits = commandCalls.filter(({ fn }) => fn === 'CommitWorkspaceTabCommand');
    const results = commandCalls.filter(({ fn }) => fn === 'GetUICommandResult');
    expect(begins.map(({ args }) => (args[1] as { code: string }).code)).toEqual([
      'Digit3', 'Digit2', 'Digit3',
    ]);
    for (const { args } of begins) {
      expect(args[0]).toBe(editorKeyboardMap.generation);
      expect(args[2]).toBe(false);
      expect(args[3]).toEqual({ surfaceId: 'editor-tab', surfaceType: 'editor', appPage: 'workspace' });
      expect(args[1]).toMatchObject({ version: 1, modifiers: ['Alt'] });
    }
    expect(takes).toHaveLength(3);
    expect(commits).toHaveLength(3);
    expect(results).toHaveLength(3);
    expect(commandCalls.filter(({ fn }) => fn === 'CompleteUICommand')).toHaveLength(0);
    const tickets = begins.map((_, index) => `editor-mode-ticket-${index + 1}`);
    expect(takes.map(({ args }) => args)).toEqual(tickets.map((ticket) => [ticket]));
    expect(commits.map(({ args }) => args)).toEqual(tickets.map((ticket) => [ticket, `${ticket}-handoff`]));
    expect(results.map(({ args }) => args)).toEqual(tickets.map((ticket) => [ticket]));
  });

  test('troca de aba restaura o foco conforme o displayMode persistido', async ({
    page,
    wails,
  }) => {
    await wails.setResponse('GetActiveWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      snapshot_epoch: 'editor-tabs-epoch',
      snapshot_sequence: '1',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'editor-tab-a',
        items: [
          {
            id: 'editor-tab-a',
            type: 'editor',
            title: 'Visualização',
            position: 0,
            state: {
              filePath: 'C:/tmp/manual-a.md',
              displayMode: 'view',
            },
          },
          {
            id: 'editor-tab-b',
            type: 'editor',
            title: 'Código',
            position: 1,
            state: {
              filePath: 'C:/tmp/manual-b.md',
              displayMode: 'markdown',
            },
          },
        ],
      },
    });
    await wails.setResponse('SetActiveWorkspaceTabForWorkspace', {
      id: 'ws-1',
      name: 'Workspace',
      snapshot_epoch: 'editor-tabs-epoch',
      snapshot_sequence: '2',
      profile: '',
      created_at: now,
      last_used: now,
      tabs: {
        active: 'editor-tab-b',
        items: [
          { id: 'editor-tab-a', type: 'editor', title: 'Visualização', position: 0, state: { filePath: 'C:/tmp/manual-a.md', displayMode: 'view' } },
          { id: 'editor-tab-b', type: 'editor', title: 'Código', position: 1, state: { filePath: 'C:/tmp/manual-b.md', displayMode: 'markdown' } },
        ],
      },
    });
    await wails.waitForApp();

    const activePanel = () => page.locator('.ws-content__panel[data-active="true"]');
    const renderedDocument = activePanel().locator('[data-editor-rendered-document="true"]');
    await expect(renderedDocument).toBeVisible({ timeout: 15_000 });
    const renderedAnchor = activePanel().locator('[data-editor-rendered-anchor="true"]');
    await renderedAnchor.focus();
    await expect(renderedAnchor).toBeFocused();

    await page.keyboard.press('Control+Tab');
    const monacoInput = activePanel().getByRole('textbox', { name: 'Markdown editor' });
    await expect(monacoInput).toBeVisible({ timeout: 15_000 });
    await expect(monacoInput).toBeFocused();

    await wails.setResponse('SetActiveWorkspaceTabForWorkspace', {
      id: 'ws-1', name: 'Workspace', snapshot_epoch: 'editor-tabs-epoch', snapshot_sequence: '3', profile: '',
      created_at: now, last_used: now,
      tabs: { active: 'editor-tab-a', items: [
        { id: 'editor-tab-a', type: 'editor', title: 'Visualização', position: 0, state: { filePath: 'C:/tmp/manual-a.md', displayMode: 'view' } },
        { id: 'editor-tab-b', type: 'editor', title: 'Código', position: 1, state: { filePath: 'C:/tmp/manual-b.md', displayMode: 'markdown' } },
      ] },
    });
    await page.keyboard.press('Control+Shift+Tab');
    await expect(renderedDocument).toHaveAttribute('role', 'document');
    await expect(renderedDocument).toBeFocused();

    await wails.setResponse('SetActiveWorkspaceTabForWorkspace', {
      id: 'ws-1', name: 'Workspace', snapshot_epoch: 'editor-tabs-epoch', snapshot_sequence: '4', profile: '',
      created_at: now, last_used: now,
      tabs: { active: 'editor-tab-b', items: [
        { id: 'editor-tab-a', type: 'editor', title: 'Visualização', position: 0, state: { filePath: 'C:/tmp/manual-a.md', displayMode: 'view' } },
        { id: 'editor-tab-b', type: 'editor', title: 'Código', position: 1, state: { filePath: 'C:/tmp/manual-b.md', displayMode: 'markdown' } },
      ] },
    });
    await page.keyboard.press('Control+PageDown');
    await expect(monacoInput).toBeFocused();
    await wails.setResponse('SetActiveWorkspaceTabForWorkspace', {
      id: 'ws-1', name: 'Workspace', snapshot_epoch: 'editor-tabs-epoch', snapshot_sequence: '5', profile: '',
      created_at: now, last_used: now,
      tabs: { active: 'editor-tab-a', items: [
        { id: 'editor-tab-a', type: 'editor', title: 'Visualização', position: 0, state: { filePath: 'C:/tmp/manual-a.md', displayMode: 'view' } },
        { id: 'editor-tab-b', type: 'editor', title: 'Código', position: 1, state: { filePath: 'C:/tmp/manual-b.md', displayMode: 'markdown' } },
      ] },
    });
    await page.keyboard.press('Control+PageUp');
    await expect(renderedDocument).toBeFocused();
  });
});
