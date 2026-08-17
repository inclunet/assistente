import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import { axe } from '../test/a11yAxe';

const mockGetPathAllowlist = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockAnnounceRequest = vi.fn();
const tStable = (key: string, fb?: string) => (typeof fb === 'string' ? fb : key);

vi.mock('@wailsjs/go/wailsapi/FSTrust', () => ({
  GetPathAllowlist: (...args: unknown[]) => mockGetPathAllowlist(...args),
  RemovePathAllowlistEntry: vi.fn(),
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

import PathAllowlistPage from './PathAllowlistPage';

const entradaDeWorkspace = {
  path: '/tmp/projeto/docs/readme.md',
  kind: 'file',
  operation: 'read',
  scope: 'workspace',
  createdBy: 'user',
  createdAt: '2026-08-17T12:00:00Z',
  reason: 'ler docs fora do sandbox',
};

describe('PathAllowlistPage — acessibilidade', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('a tela com paths autorizados não tem violações', async () => {
    mockGetPathAllowlist.mockResolvedValue([entradaDeWorkspace]);

    const { container } = render(<PathAllowlistPage />);
    await waitFor(() => {
      expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument();
    });

    expect(await axe(container)).toHaveNoViolations();
  });
});
