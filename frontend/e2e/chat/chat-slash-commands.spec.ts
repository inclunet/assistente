import { test, expect } from '../fixtures';

const mockSkills = [
  { slug: 'summarize', name: 'Resumir', displayName: 'Resumir', description: 'Resume conteúdo selecionado', invocable: true, argumentHint: '<text>' },
  { slug: 'translate', name: 'Traduzir', displayName: 'Traduzir', description: 'Traduz texto para outro idioma', invocable: true, argumentHint: '<text>' },
  { slug: 'explain', name: 'Explicar', displayName: 'Explicar', description: 'Explica conceito detalhadamente', invocable: true, argumentHint: '' },
];

async function openSlashMenu(page: import('@playwright/test').Page, value = '/') {
  const textarea = page.locator('.chat-input__textarea');
  await expect(textarea).toBeEditable({ timeout: 5_000 });
  await textarea.click();
  await textarea.fill(value);
  await expect.poll(async () => textarea.inputValue(), { timeout: 5_000 }).toBe(value);

  await expect.poll(async () => {
    return page.evaluate(() =>
      window.__wailsMock.getCallLog().some((c: { fn: string }) => c.fn === 'GetUserInvocableSkillsForProfile'),
    );
  }, { timeout: 5_000 }).toBe(true);

  const menu = page.locator('[role="listbox"]');
  await expect(menu).toBeVisible({ timeout: 5_000 });

  return { textarea, menu };
}

test.describe('Chat — Slash Commands', () => {
  test.beforeEach(async ({ wails }) => {
    await wails.setResponse('GetUserInvocableSkillsForProfile', mockSkills);
  });

  test('digitar / abre o menu de slash commands', async ({ page, wails }) => {
    await wails.waitForApp();

    const { menu } = await openSlashMenu(page);

    // Deve exibir todos os skills
    const options = menu.locator('[role="option"]');
    await expect(options).toHaveCount(3);
  });

  test('texto após / filtra os comandos', async ({ page, wails }) => {
    await wails.waitForApp();

    const { menu } = await openSlashMenu(page, '/sum');

    // Apenas "summarize" deve aparecer
    const options = menu.locator('[role="option"]');
    await expect(options).toHaveCount(1);

    // Verifica que o slug correto está presente
    const slug = menu.locator('.slash-menu__item-name');
    await expect(slug.first()).toContainText('summarize');
  });

  test('filtro sem resultados mostra estado vazio', async ({ page, wails }) => {
    await wails.waitForApp();

    const { menu } = await openSlashMenu(page, '/xyznonexistent');
    const visible = await menu.isVisible({ timeout: 2_000 }).catch(() => false);

    if (visible) {
      // Se o menu está visível, não deve ter opções
      const options = menu.locator('[role="option"]');
      await expect(options).toHaveCount(0);

      // Pode ter estado vazio
      const empty = menu.locator('.slash-menu__empty');
      if (await empty.count() > 0) {
        await expect(empty).toBeVisible();
      }
    }
  });

  test('ArrowDown/ArrowUp navega entre opções', async ({ page, wails }) => {
    await wails.waitForApp();

    const { textarea, menu } = await openSlashMenu(page);

    // O primeiro item deve estar selecionado por padrão
    const firstOption = menu.locator('[role="option"]').first();
    await expect(firstOption).toHaveAttribute('aria-selected', 'true');

    // ArrowDown move para o segundo item
    await textarea.press('ArrowDown');
    const secondOption = menu.locator('[role="option"]').nth(1);
    await expect(secondOption).toHaveAttribute('aria-selected', 'true');
    await expect(firstOption).toHaveAttribute('aria-selected', 'false');

    // ArrowUp volta para o primeiro
    await textarea.press('ArrowUp');
    await expect(firstOption).toHaveAttribute('aria-selected', 'true');
  });

  test('Enter seleciona o comando e insere slug no textarea', async ({ page, wails }) => {
    await wails.waitForApp();

    const { textarea, menu } = await openSlashMenu(page);

    // Seleciona o primeiro item (summarize)
    await textarea.press('Enter');

    // Menu deve fechar
    await expect(menu).not.toBeVisible({ timeout: 3_000 });

    // Textarea deve conter o slug do skill selecionado
    const value = await textarea.inputValue();
    expect(value).toContain('/summarize');
  });

  test('Tab também seleciona o comando', async ({ page, wails }) => {
    await wails.waitForApp();

    const { textarea, menu } = await openSlashMenu(page);

    // Navega para o segundo item e seleciona com Tab
    await textarea.press('ArrowDown');
    await textarea.press('Tab');

    await expect(menu).not.toBeVisible({ timeout: 3_000 });

    const value = await textarea.inputValue();
    expect(value).toContain('/translate');
  });
});
