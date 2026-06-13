import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../locales/pt-BR';

function resolveKey(key: string): unknown {
  return key.split('.').reduce<unknown>((acc, part) => {
    if (acc && typeof acc === 'object') {
      return (acc as Record<string, unknown>)[part];
    }
    return undefined;
  }, ptBR.translation);
}

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, options?: string | { returnObjects?: boolean }) => {
      const value = resolveKey(key);
      if (value === undefined) {
        return typeof options === 'string' ? options : key;
      }
      return value;
    },
    i18n: { language: 'pt-BR', changeLanguage: () => Promise.resolve() },
  }),
}));

import HelpPage from './HelpPage';

describe('HelpPage', () => {
  it('expande e colapsa secoes pelo header', async () => {
    const user = userEvent.setup();
    render(<HelpPage />);

    expect(
      screen.getByText((content) => content.includes('O grande diferencial do Assistente')),
    ).toBeInTheDocument();

    const sectionButton = screen.getByRole('button', { name: /Comandos por Chat e Voz/ });
    await user.click(sectionButton);

    expect(
      screen.queryByText((content) => content.includes('O grande diferencial do Assistente')),
    ).not.toBeInTheDocument();
  });

  it('expandir tudo mostra todas as secoes', async () => {
    const user = userEvent.setup();
    render(<HelpPage />);

    const expandButton = screen.getByRole('button', { name: 'Expandir Tudo' });
    await user.click(expandButton);

    const expandedBodies = document.querySelectorAll('.help-section-body');
    expect(expandedBodies.length).toBeGreaterThan(1);
  });

  it('renderiza a mini-marcacao (kbd, tabela de atalhos e icone inline)', async () => {
    const user = userEvent.setup();
    render(<HelpPage />);

    await user.click(screen.getByRole('button', { name: 'Expandir Tudo' }));

    // kbd: tokens [[...]] viram <kbd>
    expect(document.querySelectorAll('kbd').length).toBeGreaterThan(0);

    // tabela de atalhos: bloco table com cabecalho semantico
    expect(document.querySelectorAll('table.help-shortcuts thead th').length).toBeGreaterThan(0);

    // icone inline: o stub de @ant-design/icons renderiza null, entao validamos
    // que o token [icon:...] foi consumido pelo renderInline (nao vaza como texto).
    const main = document.querySelector('.help-main');
    expect(main?.textContent ?? '').not.toContain('[icon:');
  });
});
