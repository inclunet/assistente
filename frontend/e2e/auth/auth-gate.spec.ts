import { test, expect } from '../fixtures';

test.describe('AuthGate', () => {
  test('bloqueia a aplicação e mostra login quando refresh falha', async ({ page, wails }) => {
    await wails.setError('RefreshAuth', 'refresh expirado');

    await wails.gotoApp();

    // O AuthGate usa i18n; conforme o locale detectado o heading muda
    // entre "Entrar" / "Sign in" / "Entrar". Usamos regex para casar
    // independente do idioma (a regra do CLAUDE.md sobre i18n proíbe
    // hardcode em pt-BR no markup).
    await expect(page.getByRole('heading', { name: /^(Entrar|Sign in)$/ })).toBeVisible();
    await expect(page.locator('.workspace-layout')).toHaveCount(0);
  });
});
