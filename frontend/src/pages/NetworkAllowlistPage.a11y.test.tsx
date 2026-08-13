import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { axe } from '../test/a11yAxe';

const mockGetNetworkAllowlist = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetNetworkAllowlist: (...args: unknown[]) => mockGetNetworkAllowlist(...args),
  RemoveNetworkAllowlistEntry: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, fb?: string) => (typeof fb === 'string' ? fb : key),
    i18n: { language: 'pt-BR', changeLanguage: vi.fn() },
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn() }),
  announce: vi.fn(),
}));
vi.mock('../services/audioFeedback', () => ({ playBumpSound: vi.fn() }));

import NetworkAllowlistPage from './NetworkAllowlistPage';

const entradaDeWorkspace = {
  host: 'api.nu.workflows.dev',
  port: '',
  scope: 'workspace',
  category: 'cgnat',
  resolvedIps: ['100.64.1.112'],
  createdBy: 'skill:workflows-api',
  createdAt: '2026-08-01T10:00:00Z',
  reason: 'API interna de workflows',
};

describe('NetworkAllowlistPage — acessibilidade', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('a tela com hosts autorizados não tem violações', async () => {
    mockGetNetworkAllowlist.mockResolvedValue([entradaDeWorkspace]);

    const { container } = render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByRole('grid')).toBeInTheDocument());

    expect(await axe(container)).toHaveNoViolations();
  });

  it('a tela sem host autorizado não tem violações', async () => {
    mockGetNetworkAllowlist.mockResolvedValue([]);

    const { container } = render(<NetworkAllowlistPage />);
    await waitFor(() =>
      expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument(),
    );

    expect(await axe(container)).toHaveNoViolations();
  });
});
