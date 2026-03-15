import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

import HelpPage from './HelpPage';

describe('HelpPage', () => {
  it('expande e colapsa secoes pelo header', async () => {
    const user = userEvent.setup();
    render(<HelpPage />);

    expect(screen.getByText((content) => content.includes('O grande diferencial do Assistente'))).toBeInTheDocument();

    const sectionButton = screen.getByRole('button', { name: /help\.sections\.commands/ });
    await user.click(sectionButton);

    expect(screen.queryByText((content) => content.includes('O grande diferencial do Assistente'))).not.toBeInTheDocument();
  });

  it('expandir tudo mostra todas as secoes', async () => {
    const user = userEvent.setup();
    render(<HelpPage />);

    const expandButton = screen.getByRole('button', { name: 'Expandir Tudo' });
    await user.click(expandButton);

    const expandedBodies = document.querySelectorAll('.help-section-body');
    expect(expandedBodies.length).toBeGreaterThan(1);
  });
});
