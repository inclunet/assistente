import { test, expect } from '../fixtures';

test.describe('AuthGate', () => {
  test('bloqueia a aplicação e mostra login quando refresh falha', async ({ page, wails }) => {
    await wails.setError('RefreshAuth', 'refresh expirado');

    await wails.gotoApp();

    await expect(page.getByRole('heading', { name: 'Entrar' })).toBeVisible();
    await expect(page.locator('.workspace-layout')).toHaveCount(0);
  });
});
