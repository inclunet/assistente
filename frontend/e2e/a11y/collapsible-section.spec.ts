import { test, expect } from '../fixtures';

/**
 * Testes de acessibilidade do CollapsibleSection em contexto real.
 *
 * CollapsibleSection é usado na página de perfis (editor de perfil, aba Audio).
 * Testa:
 * - aria-expanded alterna ao clicar/teclado
 * - aria-controls aponta para região válida
 * - Conteúdo hidden quando colapsado
 * - role="region" no conteúdo expandido
 * - Enter/Space ativa toggle via teclado
 */

const voiceRoleOff = {
  enabled: false,
  provider: 'disabled',
  rate: 1.0,
  pitch: 1.0,
  volume: 1.0,
};

function fullProfile() {
  return {
    name: 'Padrão',
    description: 'Perfil padrão de teste',
    icon: 'chatbox',
    chat: {
      llm_provider: '$default',
      model: 'gpt-4',
      temperature: 0.7,
      max_tokens: 4096,
      top_p: 1.0,
      response_timeout: 180,
      enabled_tools: [],
      enabled_skills: [],
    },
    voice: {
      assistant: { ...voiceRoleOff },
      user: { ...voiceRoleOff },
      system: { ...voiceRoleOff },
    },
    input: {
      enabled: false,
      stt_provider: '',
      language: 'pt-BR',
      feedback_sounds: true,
      triggers: [],
    },
    channels: { response_mode: 'mirror' },
  };
}

async function openProfileEditorAudioTab(
  page: import('@playwright/test').Page,
  wails: Parameters<Parameters<typeof test>[2]>[0]['wails'],
) {
  // Mocks necessários para a página de perfis
  await wails.setResponse('GetProfiles', [
    { slug: 'padrao', name: 'Padrão', description: 'Perfil padrão', icon: 'chatbox', source: 'workdir' },
  ]);
  await wails.setResponse('GetActiveProfileSlug', 'padrao');
  await wails.setResponse('GetProfileSearchPaths', []);
  await wails.setResponse('GetProfile', fullProfile());
  await wails.setResponse('UpdateProfile', undefined);
  await wails.setResponse('GetSpeechProviders', []);
  await wails.setResponse('GetSTTModels', []);
  await wails.setResponse('GetUserInvocableSkills', []);
  await wails.setResponse('GetAvailableTools', []);
  await wails.setResponse('GetToolCatalog', { tools: [] });
  await wails.setResponse('ListMCPServers', []);
  await wails.setResponse('GetAllowlists', []);
  await wails.waitForApp();

  // Navega para a página de perfis
  await page.goto('/#/profiles');

  await expect(page.locator('.profiles-page')).toBeVisible({ timeout: 10_000 });

  // Aguarda o grid carregar a linha do perfil padrão
  const firstCell = page.locator('[role="gridcell"]').filter({ hasText: 'Padrão' }).first();
  await expect(firstCell).toBeVisible({ timeout: 10_000 });

  // Aguarda overlay de empty state desaparecer
  const emptyState = page.locator('.profiles-empty');
  await expect(emptyState).toBeHidden({ timeout: 5_000 }).catch(() => {
    // Se timeout, ignora — pode já ter sumido
  });

  // Seleciona a linha via foco e keyboard (evita intercept por overlay)
  await firstCell.focus();
  await page.waitForTimeout(200);

  // Enter no grid cell dispara onActivate → abre o editor modal
  await page.keyboard.press('Enter');

  // Aguarda o modal/editor abrir
  const modal = page.locator('.modal-overlay[role="dialog"]');
  await expect(modal).toBeVisible({ timeout: 10_000 });

  // Navega para a aba "Audio" que tem CollapsibleSections (Voz TTS)
  const audioTab = page.locator('[role="tab"]').filter({ hasText: /Audio|Áudio/ });
  if (await audioTab.count() > 0) {
    await audioTab.click();
    await page.waitForTimeout(500);
  }

  // Aguarda seções colapsáveis VISÍVEIS aparecerem no painel ativo
  const visibleSection = modal.locator('.collapsible-section__header:visible');
  await expect(visibleSection.first()).toBeVisible({ timeout: 5_000 });

  return modal;
}

test.describe('CollapsibleSection — acessibilidade em contexto', () => {
  test('seção colapsável tem aria-expanded e aria-controls', async ({ page, wails }) => {
    const modal = await openProfileEditorAudioTab(page, wails);

    const firstButton = modal.locator('.collapsible-section__header:visible').first();

    // Deve ter aria-expanded
    const ariaExpanded = await firstButton.getAttribute('aria-expanded');
    expect(ariaExpanded === 'true' || ariaExpanded === 'false').toBe(true);

    // Deve ter aria-controls apontando para um ID válido
    const controls = await firstButton.getAttribute('aria-controls');
    expect(controls).toBeTruthy();

    // O elemento referenciado por aria-controls deve existir
    if (controls) {
      const regionExists = await page.evaluate((id) => {
        return document.getElementById(id) !== null;
      }, controls);
      expect(regionExists).toBe(true);
    }
  });

  test('Enter no botão da seção alterna aria-expanded', async ({ page, wails }) => {
    const modal = await openProfileEditorAudioTab(page, wails);

    const firstButton = modal.locator('.collapsible-section__header:visible').first();
    const initialState = await firstButton.getAttribute('aria-expanded');

    // Foca e pressiona Enter
    await firstButton.focus();
    await page.keyboard.press('Enter');
    await page.waitForTimeout(200);

    // aria-expanded deve ter invertido
    const newState = await firstButton.getAttribute('aria-expanded');
    expect(newState).not.toBe(initialState);
  });

  test('Space no botão da seção alterna aria-expanded', async ({ page, wails }) => {
    const modal = await openProfileEditorAudioTab(page, wails);

    const firstButton = modal.locator('.collapsible-section__header:visible').first();
    const initialState = await firstButton.getAttribute('aria-expanded');

    // Foca e pressiona Space
    await firstButton.focus();
    await page.keyboard.press('Space');
    await page.waitForTimeout(200);

    // aria-expanded deve ter invertido
    const newState = await firstButton.getAttribute('aria-expanded');
    expect(newState).not.toBe(initialState);
  });

  test('região expandida tem role="region" com aria-labelledby', async ({ page, wails }) => {
    const modal = await openProfileEditorAudioTab(page, wails);

    const firstButton = modal.locator('.collapsible-section__header:visible').first();

    // Expande a seção se estiver colapsada
    const isExpanded = await firstButton.getAttribute('aria-expanded');
    if (isExpanded === 'false') {
      await firstButton.click();
      await page.waitForTimeout(200);
    }

    // O conteúdo expandido deve ter role="region"
    const controlsId = await firstButton.getAttribute('aria-controls');
    expect(controlsId).toBeTruthy();
    if (controlsId) {
      const region = await page.evaluate((id) => {
        const el = document.getElementById(id);
        return el ? { role: el.getAttribute('role'), labelledBy: el.getAttribute('aria-labelledby') } : null;
      }, controlsId);
      expect(region).not.toBeNull();
      expect(region!.role).toBe('region');
      expect(region!.labelledBy).toBeTruthy();
    }
  });

  test('conteúdo colapsado tem atributo hidden', async ({ page, wails }) => {
    const modal = await openProfileEditorAudioTab(page, wails);

    const firstButton = modal.locator('.collapsible-section__header:visible').first();

    // Garante que a seção está colapsada
    const isExpanded = await firstButton.getAttribute('aria-expanded');
    if (isExpanded === 'true') {
      await firstButton.click();
      await page.waitForTimeout(200);
    }

    // O conteúdo deve ter atributo hidden
    const controlsId = await firstButton.getAttribute('aria-controls');
    expect(controlsId).toBeTruthy();
    if (controlsId) {
      const hasHidden = await page.evaluate((id) => {
        const el = document.getElementById(id);
        return el ? el.hasAttribute('hidden') : null;
      }, controlsId);
      expect(hasHidden).toBe(true);
    }
  });
});
