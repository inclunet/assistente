import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetProviders = vi.fn();
const mockCreateProvider = vi.fn();
const mockDeleteProvider = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (_key: string, fallback?: string) => fallback ?? _key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetLLMProvidersWithStatus: () => mockGetProviders(),
  CreateLLMProvider: (payload: unknown) => mockCreateProvider(payload),
  DeleteLLMProvider: (_ctx: unknown, id: string) => mockDeleteProvider(id),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({
    addToast: mockAddToast,
  }),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick?: () => void; disabled?: boolean }> }) => (
    <div>
      {left}
      {actions?.map((action) => (
        <button
          key={action.key}
          data-testid={`toolbar-action-${action.key}`}
          onClick={action.onClick}
          disabled={action.disabled}
        >
          {action.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onFocusChange,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string; type: string; base_url: string }>;
    onFocusChange?: (item: { id: string; name: string; type: string; base_url: string } | null) => void;
    getRowActions?: (item: { id: string; name: string; type: string; base_url: string }) => Array<{ id: string; label?: string; onClick?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} type="button" onClick={action.onClick}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children?: ReactNode }) => (isOpen ? <div>{children}</div> : null),
  isModalOpen: () => false,
}));

vi.mock('../components/settings/ProviderForm', () => ({
  ProviderForm: ({ onSave, onCancel }: { onSave: () => void; onCancel: () => void }) => (
    <div>
      <button type="button" onClick={onSave}>Salvar</button>
      <button type="button" onClick={onCancel}>Cancelar</button>
    </div>
  ),
}));

import ProvidersPage from './ProvidersPage';

describe('ProvidersPage', () => {
  let nowSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    nowSpy = vi.spyOn(Date, 'now');
    mockGetProviders.mockResolvedValue([
      {
        id: 'openai-1',
        name: 'OpenAI',
        type: 'openai',
        base_url: 'https://api.openai.com',
        credential_required: true,
        credential_status: 'configured',
      },
    ]);
    mockCreateProvider.mockResolvedValue(undefined);
    mockDeleteProvider.mockResolvedValue(undefined);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    nowSpy.mockReturnValue(123);
  });

  afterEach(() => {
    nowSpy.mockRestore();
  });

  it('duplica provedor via menu de acoes', async () => {
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    const menuDuplicate = duplicateButtons.find((button) => !button.hasAttribute('disabled'));
    expect(menuDuplicate).toBeTruthy();
    await user.click(menuDuplicate!);

    await waitFor(() => {
      expect(mockCreateProvider).toHaveBeenCalledWith(expect.objectContaining({
        id: 'openai-123',
        name: 'OpenAI (Copia)',
        type: 'openai',
        base_url: 'https://api.openai.com',
      }));
    });
  });

  it('habilita acao de excluir na toolbar apos foco', async () => {
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument();
    });

    const deleteButton = screen.getByTestId('toolbar-action-delete');
    expect(deleteButton).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteProvider).toHaveBeenCalledWith('openai-1');
    });
  });
});
