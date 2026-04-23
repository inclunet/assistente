import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';

const mockGetSkills = vi.fn();
const mockGetSkill = vi.fn();
const mockGetSkillSearchPaths = vi.fn();
const mockDuplicateSkill = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
    i18n: { language: 'pt-BR' },
  }),
  Trans: ({ children, defaults }: { children?: ReactNode; defaults?: string }) => <>{defaults ?? children}</>,
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetSkills: () => mockGetSkills(),
  GetSkill: (slug: string) => mockGetSkill(slug),
  GetSkillSearchPaths: () => mockGetSkillSearchPaths(),
  CreateSkill: vi.fn(),
  UpdateSkill: vi.fn(),
  DeleteSkill: vi.fn(),
  DuplicateSkill: (slug: string) => mockDuplicateSkill(slug),
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/models', () => ({
  main: {
    SkillCreateRequest: {
      createFrom: (data: unknown) => data,
    },
  },
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

const mockAddToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: mockAddToast,
  }),
}));

const mockAnnounce = vi.fn();
vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick: () => void }> }) => (
    <div>
      {left}
      <div>
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick}>
            {action.label}
          </button>
        ))}
      </div>
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string }>;
    getRowActions?: (item: { id: string; name: string }) => Array<{ id: string; label: string; onClick: () => void }>;
  }) => (
    <div>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} onClick={action.onClick}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children: ReactNode }) => (isOpen ? <div>{children}</div> : null),
  isModalOpen: () => false,
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({
    onClick,
    children,
    loading,
    ...rest
  }: {
    onClick?: () => void;
    children?: ReactNode;
    loading?: boolean;
  }) => (
    <button onClick={onClick} disabled={loading} {...rest}>
      {children}
    </button>
  ),
  PageLoading: ({ message }: { message?: string }) => <div role="status">{message}</div>,
}));

describe('SkillsPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockGetSkills.mockReset();
    mockGetSkill.mockReset();
    mockGetSkillSearchPaths.mockReset();
    mockDuplicateSkill.mockReset();
    mockAddToast.mockReset();
    mockAnnounce.mockReset();

    mockGetSkills
      .mockResolvedValueOnce([
        {
          slug: 'skill-base',
          name: 'skill-base',
          description: 'Descricao valida',
          disableModelInvocation: false,
          source: 'home',
          tools: { allowed: [] },
        },
      ])
      .mockResolvedValueOnce([
        {
          slug: 'skill-base',
          name: 'skill-base',
          description: 'Descricao valida',
          disableModelInvocation: false,
          source: 'home',
          tools: { allowed: [] },
        },
        {
          slug: 'skill-base-copia',
          name: 'skill-base-copia',
          description: 'Descricao valida',
          disableModelInvocation: false,
          source: 'home',
          tools: { allowed: [] },
        },
      ]);

    mockGetSkill.mockResolvedValue({
      slug: 'skill-base-copia',
      name: 'skill-base-copia',
      description: 'Descricao valida',
      disableModelInvocation: false,
      source: 'home',
      tools: { allowed: [] },
      content: 'conteudo',
    });

    mockGetSkillSearchPaths.mockResolvedValue([]);
    mockDuplicateSkill.mockResolvedValue('skill-base-copia');
  });

  it('duplica um skill via menu de acoes', async () => {
    const { default: SkillsPage } = await import('./SkillsPage');

    render(<SkillsPage />);

    await screen.findByText('skill-base');

    const row = screen.getByText('skill-base').closest('div');
    const duplicateButton = within(row as HTMLElement).getByRole('button', { name: 'Duplicar' });
    fireEvent.click(duplicateButton);

    await waitFor(() => {
      expect(mockDuplicateSkill).toHaveBeenCalledWith('skill-base');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('Skill duplicado!', 'success');
    });

    await waitFor(() => {
      expect(mockGetSkill).toHaveBeenCalledWith('skill-base-copia');
    });
  });
});
