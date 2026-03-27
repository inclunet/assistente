import { configureAxe } from 'vitest-axe';

/**
 * Instância do axe para jsdom: a regra color-contrast usa Canvas (getContext),
 * que o jsdom não implementa; desabilitar evita erros e falsos positivos.
 */
export const axe = configureAxe({
  rules: {
    'color-contrast': { enabled: false },
  },
});
