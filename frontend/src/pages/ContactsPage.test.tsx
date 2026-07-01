import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, fallback?: string) =>
      ({
        'contacts.title': 'Contatos Autorizados',
        'contacts.loading': 'Carregando contatos...',
        'contacts.description': 'Contatos que podem se comunicar com o assistente.',
        'contacts.empty': 'Nenhum contato autorizado.',
        'contacts.gridLabel': 'Contatos autorizados',
        'contacts.toolbarLabel': 'Barra de ferramentas de contatos',
        'channels.actions.removeContact': 'Remover',
        'channels.buttons.reload': 'Recarregar',
      } as Record<string, string>)[key] ?? fallback ?? key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetAuthorizedContacts: vi.fn(() => new Promise(() => {})),
  RemoveAuthorizedContact: vi.fn(),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: () => ({ addToast: vi.fn() }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn(() => true) }),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({ handleGridReady: vi.fn() }),
}));

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: vi.fn(),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: () => <div data-testid="toolbar">toolbar</div>,
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: () => <div data-testid="datagrid">grid</div>,
}));

vi.mock('../components/layout/MenuButton', () => ({
  MenuButton: () => null,
}));

import ContactsPage from './ContactsPage';

describe('ContactsPage', () => {
  it('renderiza loading state inicialmente', () => {
    render(<ContactsPage />);
    expect(screen.getByText('Carregando contatos...')).toBeInTheDocument();
    expect(screen.queryByRole('status')).not.toBeInTheDocument();
  });

  it('possui a classe CSS principal', () => {
    const { container } = render(<ContactsPage />);
    expect(container.querySelector('.contacts-page')).toBeInTheDocument();
  });
});
