import { test as base, expect, type Page } from '@playwright/test';
import { buildWailsMockScript } from './mocks/wails-runtime';

/**
 * Fixture customizada que injeta o mock do Wails antes de cada teste.
 *
 * As respostas configuradas via setResponse ANTES de waitForApp são
 * injetadas via addInitScript (executam antes do JS do app).
 * Após waitForApp, setResponse usa page.evaluate.
 */

export interface WailsMock {
  /** Sobrescreve a resposta de uma função Wails. Pode ser chamado antes ou depois de waitForApp. */
  setResponse: (fn: string, value: unknown) => Promise<void>;
  /** Emite um evento Wails (simula backend → frontend). Só funciona após waitForApp. */
  emit: (event: string, data?: unknown) => Promise<void>;
  /** Retorna o log de chamadas feitas ao backend. Só funciona após waitForApp. */
  getCallLog: () => Promise<Array<{ fn: string; args: unknown[] }>>;
  /** Navega e espera a aplicação estar pronta (layout renderizado). */
  waitForApp: () => Promise<void>;
}

export const test = base.extend<{ wails: WailsMock }>({
  wails: async ({ page }, use) => {
    // Injeta mocks antes de qualquer navegação
    await page.addInitScript({ content: buildWailsMockScript() });

    let navigated = false;
    const pendingResponses: Array<{ fn: string; value: unknown }> = [];

    const mock: WailsMock = {
      async setResponse(fn: string, value: unknown) {
        if (!navigated) {
          // Antes da navegação, enfileirar como addInitScript
          pendingResponses.push({ fn, value });
        } else {
          await page.evaluate(
            ({ fn, value }) => window.__wailsMock.setResponse(fn, value),
            { fn, value },
          );
        }
      },

      async emit(event: string, data?: unknown) {
        await page.evaluate(
          ({ event, data }) => window.__wailsMock.emit(event, data),
          { event, data },
        );
      },

      async getCallLog() {
        return page.evaluate(() => window.__wailsMock.getCallLog());
      },

      async waitForApp() {
        // Injeta respostas pendentes como init scripts (rodam antes do app)
        if (pendingResponses.length > 0) {
          const script = pendingResponses
            .map(({ fn, value }) => {
              const serialized = JSON.stringify(value);
              return `window.__wailsMock.setResponse(${JSON.stringify(fn)}, ${serialized});`;
            })
            .join('\n');
          await page.addInitScript({ content: script });
          pendingResponses.length = 0;
        }

        await page.goto('/');
        navigated = true;

        // Espera o layout principal renderizar
        await page.waitForSelector('.workspace-layout, .layout', { timeout: 15_000 });
      },
    };

    await use(mock);
  },
});

export { expect };
