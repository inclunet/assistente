import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { axe } from '../test/a11yAxe';

const mockGetNetworkAllowlist = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockAnnounceRequest = vi.fn();
const tStable = (key: string, fb?: string) => (typeof fb === 'string' ? fb : key);

vi.mock('@wailsjs/go/wailsapi/NetTrust', () => ({
  GetNetworkAllowlist: (...args: unknown[]) => mockGetNetworkAllowlist(...args),
  RemoveNetworkAllowlistEntry: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: tStable,
    i18n: { language: 'pt-BR', changeLanguage: vi.fn() },
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce, announceRequest: mockAnnounceRequest }),
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => vi.fn() }));
vi.mock('../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: vi.fn() }) }));
vi.mock('../hooks/useGridPageLandmarks', () => ({ useGridPageLandmarks: vi.fn() }));
vi.mock('../services/audioFeedback', () => ({ playBumpSound: vi.fn() }));
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof mockAddToast }) => unknown) =>
    selector({ addToast: mockAddToast }),
}));
vi.mock('../components/ui/PageLoading', () => ({
  PageLoading: ({ message }: { message: string }) => <div>{message}</div>,
}));

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
    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument();
    });

    expect(await axe(container)).toHaveNoViolations();
  });
});
