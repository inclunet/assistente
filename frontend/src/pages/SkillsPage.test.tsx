import { describe, expect, it, vi, beforeEach } from 'vitest';
import type { ReactNode } from 'react';
import { render, screen, waitFor, within, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetSkills = vi.fn();
const mockGetSkill = vi.fn();
const mockGetSkillSearchPaths = vi.fn();
const mockDuplicateSkill = vi.fn();
const mockCreateSkill = vi.fn();
const mockUpdateSkill = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
    i18n: { language: 'pt-BR' },
  }),
  Trans: ({ children, defaults }: { children?: ReactNode; defaults?: string }) => <>{defaults ?? children}</>,
}));

vi.mock('@wailsjs/go/wailsapi/Skills', () => ({
  GetSkills: () => mockGetSkills(),
  GetSkill: (slug: string) => mockGetSkill(slug),
  GetSkillSearchPaths: () => mockGetSkillSearchPaths(),
  CreateSkill: (request: unknown) => mockCreateSkill(request),
  UpdateSkill: (slug: string, request: unknown) => mockUpdateSkill(slug, request),
  DeleteSkill: vi.fn(),
  DuplicateSkill: (slug: string) => mockDuplicateSkill(slug),
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: vi.fn().mockResolvedValue([]),
}));

vi.mock('@wailsjs/go/models', () => ({
  apidto: {
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

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn().mockResolvedValue(true),
}));

const mockAddToast = vi.fn();
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
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

import SkillsPage from './SkillsPage';

describe('SkillsPage', { timeout: 60_000 }, () => {
  beforeEach(() => {
    mockGetSkills.mockReset();
    mockGetSkill.mockReset();
    mockGetSkillSearchPaths.mockReset();
    mockDuplicateSkill.mockReset();
    mockCreateSkill.mockReset();
    mockUpdateSkill.mockReset();
    mockAddToast.mockReset();
    mockAnnounce.mockReset();

    mockGetSkills
      .mockResolvedValueOnce([
        {
          slug: 'skill-base',
          name: 'skill-base',
          version: '2.3.4',
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
          version: '2.3.4',
          description: 'Descricao valida',
          disableModelInvocation: false,
          source: 'home',
          tools: { allowed: [] },
        },
        {
          slug: 'skill-base-copia',
          name: 'skill-base-copia',
          version: '2.3.4',
          description: 'Descricao valida',
          disableModelInvocation: false,
          source: 'home',
          tools: { allowed: [] },
        },
      ]);

    mockGetSkill.mockResolvedValue({
      slug: 'skill-base-copia',
      name: 'skill-base-copia',
      version: '2.3.4',
      description: 'Descricao valida',
      disableModelInvocation: false,
      source: 'home',
      tools: { allowed: [] },
      content: 'conteudo',
    });

    mockGetSkillSearchPaths.mockResolvedValue([]);
    mockDuplicateSkill.mockResolvedValue('skill-base-copia');
    mockCreateSkill.mockResolvedValue('novo-skill');
    mockUpdateSkill.mockResolvedValue(undefined);
  });

  it('duplica um skill via menu de acoes', async () => {
    render(<SkillsPage />);

    await screen.findByText('skill-base');

    const row = screen.getByText('skill-base').closest('div');
    const duplicateButton = within(row as HTMLElement).getByRole('button', { name: 'Duplicar' });
    fireEvent.click(duplicateButton);

    await waitFor(() => {
      expect(mockDuplicateSkill).toHaveBeenCalledWith('skill-base');
    });

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('Skill duplicado!', 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
    });

    await waitFor(() => {
      expect(mockGetSkill).toHaveBeenCalledWith('skill-base-copia');
    });
  });

  it('cria skill com a versão semântica padrão', async () => {
    const user = userEvent.setup();
    render(<SkillsPage />);
    await screen.findByText('skill-base');

    await user.click(screen.getByRole('button', { name: 'Novo Skill' }));

    expect(screen.getByLabelText('skills.generalSection.version')).toHaveValue('1.0.0');
    await user.type(screen.getByLabelText('skills.generalSection.name'), 'novo-skill');
    await user.type(
      screen.getByLabelText('skills.generalSection.description'),
      'Descrição válida para o skill',
    );
    await user.click(screen.getByRole('button', { name: 'Salvar' }));

    await waitFor(() => {
      expect(mockCreateSkill).toHaveBeenCalledWith(
        expect.objectContaining({
          name: 'novo-skill',
          version: '1.0.0',
          description: 'Descrição válida para o skill',
        }),
      );
    });
  });

  it('preserva versão existente e defaulta skill legada sem version ao editar', async () => {
    const user = userEvent.setup();
    mockGetSkill
      .mockResolvedValueOnce({
        slug: 'skill-base',
        name: 'skill-base',
        version: '2.3.4',
        description: 'Descricao valida',
        disableModelInvocation: false,
        source: 'home',
        tools: { allowed: [] },
        content: 'conteudo',
      })
      .mockResolvedValueOnce({
        slug: 'skill-base',
        name: 'skill-base',
        description: 'Descricao valida',
        disableModelInvocation: false,
        source: 'home',
        tools: { allowed: [] },
        content: 'conteudo',
      });

    const { unmount } = render(<SkillsPage />);
    await screen.findByText('skill-base');
    let editButtons = screen.getAllByRole('button', { name: 'Editar skill' });
    await user.click(editButtons[editButtons.length - 1]);
    expect(await screen.findByLabelText('skills.generalSection.version')).toHaveValue('2.3.4');
    unmount();

    render(<SkillsPage />);
    await screen.findByText('skill-base');
    editButtons = screen.getAllByRole('button', { name: 'Editar skill' });
    await user.click(editButtons[editButtons.length - 1]);
    expect(await screen.findByLabelText('skills.generalSection.version')).toHaveValue('1.0.0');
  });
});
