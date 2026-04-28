import { test, expect } from '../fixtures';

/**
 * Testes de focus trap e acessibilidade do componente Modal.
 *
 * Testa:
 * - Tab/Shift+Tab ciclam entre elementos focáveis dentro do modal
 * - Escape fecha o modal
 * - Foco é restaurado na área padrão após fechar
 * - role="dialog" e aria-labelledby presentes
 * - aria-hidden e inert no #root enquanto modal aberto
 * - Auto-focus no primeiro elemento focável ao abrir
 */

/** Dados completos para que o TokenStatsModal não crashe ao abrir */
function fullTokenStats() {
  return {
    conversationId: '01926b90-0000-7000-8000-000000000001',
    promptTokens: 500,
    completionTokens: 300,
    totalTokens: 800,
    messageCount: 5,
    mostUsedModel: 'gpt-4',
    contextUsage: 10,
    contextLimit: 128000,
    isNearLimit: false,
    isCritical: false,
    systemPromptEstimatedTokens: 100,
    summaryTokens: 0,
    messagesInContextTokens: 400,
    messagesOutOfContextTokens: 0,
    messagesInContextCount: 5,
    messagesOutOfContextCount: 0,
    toolsUsedCount: 0,
    toolBreakdown: [],
  };
}

async function openTokenModal(
  page: import('@playwright/test').Page,
  wails: Parameters<Parameters<typeof test>[2]>[0]['wails'],
) {
  await wails.setResponse('GetConversationTokenStats', fullTokenStats());
  await wails.waitForApp();

  const tokenBtn = page.locator('.token-stats-button');
  await expect(tokenBtn).toBeVisible({ timeout: 5_000 });
  await expect(tokenBtn).toBeEnabled({ timeout: 5_000 });
  await tokenBtn.focus();
  await tokenBtn.press('Enter');

  const dialog = page.locator('.modal-overlay[role="dialog"]');
  if (!(await dialog.isVisible().catch(() => false))) {
    await tokenBtn.click();
  }
  await expect(dialog).toBeVisible({ timeout: 7_000 });
  return dialog;
}

test.describe('Modal — focus trap (Tab cycling)', () => {
  test('Tab cicla entre elementos focáveis dentro do modal', async ({ page, wails }) => {
    const dialog = await openTokenModal(page, wails);

    // Modal content contém os focáveis
    const modalContent = dialog.locator('.modal-content');

    // Coleta os elementos focáveis dentro do modal-content
    const focusableCount = await modalContent.evaluate((el) => {
      const selector =
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
        'textarea:not([disabled]), [tabindex]:not([tabindex="-1"]), [contenteditable]';
      return Array.from(el.querySelectorAll(selector)).filter(
        (e) => (e as HTMLElement).offsetParent !== null,
      ).length;
    });

    // Deve ter pelo menos um elemento focável (o botão de fechar)
    expect(focusableCount).toBeGreaterThanOrEqual(1);

    // Foca no último elemento focável
    const lastFocusable = modalContent.locator(
      'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])',
    ).last();
    await lastFocusable.focus();
    await expect(lastFocusable).toBeFocused({ timeout: 2_000 });

    // Tab no último → deve voltar para o primeiro (cycling)
    await page.keyboard.press('Tab');

    // O foco deve estar no primeiro focável (focus trap)
    const activeAfterTab = await modalContent.evaluate((el) => {
      const selector =
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
        'textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
      const elements = Array.from(el.querySelectorAll(selector)).filter(
        (e) => (e as HTMLElement).offsetParent !== null,
      );
      return elements.indexOf(document.activeElement as Element);
    });
    expect(activeAfterTab).toBe(0);
  });

  test('Shift+Tab no primeiro elemento cicla para o último', async ({ page, wails }) => {
    const dialog = await openTokenModal(page, wails);

    const modalContent = dialog.locator('.modal-content');

    // Foca no primeiro elemento focável
    const firstFocusable = modalContent.locator(
      'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled])',
    ).first();
    await firstFocusable.focus();
    await expect(firstFocusable).toBeFocused({ timeout: 2_000 });

    // Shift+Tab no primeiro → deve ir para o último
    await page.keyboard.press('Shift+Tab');

    // Verifica que o foco está no último focável
    const focusIsOnLast = await modalContent.evaluate((el) => {
      const selector =
        'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), ' +
        'textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';
      const elements = Array.from(el.querySelectorAll(selector)).filter(
        (e) => (e as HTMLElement).offsetParent !== null,
      );
      return document.activeElement === elements[elements.length - 1];
    });
    expect(focusIsOnLast).toBe(true);
  });
});

test.describe('Modal — Escape fecha e restaura foco', () => {
  test('Escape fecha o modal', async ({ page, wails }) => {
    await wails.setResponse('GetConversationTokenStats', fullTokenStats());
    await wails.waitForApp();

    const tokenBtn = page.locator('.token-stats-button');
    await tokenBtn.click();

    const dialog = page.locator('.modal-overlay[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 5_000 });

    // Pressiona Escape
    await page.keyboard.press('Escape');

    // Modal deve fechar
    await expect(dialog).not.toBeVisible({ timeout: 3_000 });
  });

  test('foco retorna à área padrão após fechar modal', async ({ page, wails }) => {
    await wails.setResponse('GetConversationTokenStats', fullTokenStats());
    await wails.waitForApp();

    const textarea = page.locator('.chat-input__textarea');
    await expect(textarea).toBeFocused({ timeout: 5_000 });

    // Abre modal
    const tokenBtn = page.locator('.token-stats-button');
    await tokenBtn.click();
    await expect(page.locator('.modal-overlay[role="dialog"]')).toBeVisible({ timeout: 5_000 });

    // Fecha via Escape
    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-overlay[role="dialog"]')).not.toBeVisible({ timeout: 3_000 });

    // Foco deve voltar ao textarea (área padrão)
    await expect(textarea).toBeFocused({ timeout: 5_000 });
  });
});

test.describe('Modal — ARIA attributes', () => {
  test('modal tem role="dialog" e aria-labelledby', async ({ page, wails }) => {
    await wails.setResponse('GetConversationTokenStats', fullTokenStats());
    await wails.waitForApp();

    const tokenBtn = page.locator('.token-stats-button');
    await expect(tokenBtn).toBeVisible({ timeout: 5_000 });
    await expect(tokenBtn).toBeEnabled({ timeout: 5_000 });
    await tokenBtn.press('Enter');

    const dialog = page.locator('.modal-overlay[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 7_000 });

    // Verifica aria-labelledby aponta para um título existente
    const labelledBy = await dialog.getAttribute('aria-labelledby');
    expect(labelledBy).toBeTruthy();

    // O título referenciado deve existir dentro do modal
    if (labelledBy) {
      // CSS.escape não está disponível no Node — escapar via page.evaluate
      const titleVisible = await page.evaluate((id) => {
        const el = document.getElementById(id);
        return el !== null && el.offsetParent !== null;
      }, labelledBy);
      expect(titleVisible).toBe(true);
    }
  });

  test('#root recebe aria-hidden e inert quando modal está aberto', async ({ page, wails }) => {
    await wails.setResponse('GetConversationTokenStats', fullTokenStats());
    await wails.waitForApp();

    // Antes de abrir, #root não tem aria-hidden
    const root = page.locator('#root');
    await expect(root).not.toHaveAttribute('aria-hidden');

    // Abre modal
    const tokenBtn = page.locator('.token-stats-button');
    await expect(tokenBtn).toBeVisible({ timeout: 5_000 });
    await expect(tokenBtn).toBeEnabled({ timeout: 5_000 });
    await tokenBtn.press('Enter');
    await expect(page.locator('.modal-overlay[role="dialog"]')).toBeVisible({ timeout: 7_000 });

    // #root deve ter aria-hidden="true" e inert
    await expect(root).toHaveAttribute('aria-hidden', 'true');
    const hasInert = await root.evaluate((el) => el.hasAttribute('inert'));
    expect(hasInert).toBe(true);

    // Fecha modal
    await page.keyboard.press('Escape');
    await expect(page.locator('.modal-overlay[role="dialog"]')).not.toBeVisible({ timeout: 3_000 });

    // #root deve restaurar — sem aria-hidden e sem inert
    await expect(root).not.toHaveAttribute('aria-hidden');
    const hasInertAfter = await root.evaluate((el) => el.hasAttribute('inert'));
    expect(hasInertAfter).toBe(false);
  });

  test('auto-focus no primeiro elemento focável ao abrir', async ({ page, wails }) => {
    await wails.setResponse('GetConversationTokenStats', fullTokenStats());
    await wails.waitForApp();

    const tokenBtn = page.locator('.token-stats-button');
    await expect(tokenBtn).toBeVisible({ timeout: 5_000 });
    await tokenBtn.focus();
    await expect(tokenBtn).toBeFocused({ timeout: 3_000 });
    await page.keyboard.press('Enter');

    const dialog = page.locator('.modal-overlay[role="dialog"]');
    await expect(dialog).toBeVisible({ timeout: 7_000 });

    // Aguarda o auto-focus (usa requestAnimationFrame)
    await page.waitForTimeout(200);

    // O foco deve estar dentro do dialog
    const focusInsideDialog = await dialog.evaluate((el) => {
      return el.contains(document.activeElement);
    });
    expect(focusInsideDialog).toBe(true);
  });
});
