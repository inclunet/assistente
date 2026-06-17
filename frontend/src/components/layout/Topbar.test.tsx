import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Topbar } from './Topbar';

const navigateSpy = vi.fn();
const toggleMenuSpy = vi.fn();
const announceSpy = vi.fn();
const modalState = vi.hoisted(() => ({ open: false }));

vi.mock('../ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ui/Modal')>();
  return { ...actual, isModalOpen: () => modalState.open };
});

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => navigateSpy,
    useLocation: () => ({ pathname: '/history' }),
  };
});

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: { updateConfig: (cfg: unknown) => void }) => unknown) =>
    selector({ updateConfig: vi.fn() }),
}));

vi.mock('../../store/workspaceStore', () => ({
  useWorkspaceStore: Object.assign(
    (selector?: (state: Record<string, unknown>) => unknown) => {
      const state = {
        workspace: { name: 'Test Workspace', profile: '' },
        workspaces: [],
        switchWorkspace: vi.fn(),
        createWorkspace: vi.fn(),
        renameWorkspace: vi.fn(),
      };
      return selector ? selector(state) : state;
    },
    { getState: () => ({ exportWorkspace: vi.fn(), importWorkspace: vi.fn() }) },
  ),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
    openForTrigger: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy }),
}));

vi.mock('./MenuButton', () => {
  const React = require('react');
  const MenuButton = React.forwardRef((props: { items: Array<{ id: string; onClick?: () => void }>; buttonLabel: string; currentItemId: string }, ref: React.Ref<{ toggleMenu: () => void }>) => {
    React.useImperativeHandle(ref, () => ({ toggleMenu: toggleMenuSpy }));

    return (
      <div>
        <button onClick={() => props.items[0]?.onClick?.()}>{props.buttonLabel}</button>
        <div data-testid="current-item">{props.currentItemId}</div>
        <div data-testid="menu-items">{props.items.map((item) => item.id).join(',')}</div>
      </div>
    );
  });

  return { MenuButton, MenuItem: {}, MenuButtonRef: {} };
});

describe('Topbar', () => {
  it('renderiza titulo da página e configura item atual', () => {
    render(<Topbar />);

    expect(screen.getByRole('button', { name: 'menu.navLabel' })).toBeInTheDocument();
    // On /history sub-route, the h1 shows the page title
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('menu.history');
    expect(screen.getByTestId('current-item')).toHaveTextContent('history');

    const items = screen.getByTestId('menu-items').textContent || '';
    expect(items).toContain('memories');
    expect(items).toContain('settings');
    expect(items).not.toContain('export-data');
    expect(items).not.toContain('import-data');
    expect(items).not.toContain('theme');
    expect(items).not.toContain('language');
  });

  it('mostra botão voltar em sub-rota', () => {
    render(<Topbar />);

    const backButton = screen.getByRole('button', { name: 'menu.backToWorkspace' });
    expect(backButton).toBeInTheDocument();

    fireEvent.click(backButton);
    expect(navigateSpy).toHaveBeenCalledWith('/');
  });

  it('abre menu com Alt+M', () => {
    render(<Topbar />);

    fireEvent.keyDown(window, { key: 'm', altKey: true });
    expect(toggleMenuSpy).toHaveBeenCalled();
  });

  it.each([
    ['h', '/history'],
    ['e', '/settings/data?action=export'],
    ['i', '/settings/data?action=import'],
    ['p', '/profiles'],
  ])('navega com Alt+%s para %s', (key, route) => {
    render(<Topbar />);
    navigateSpy.mockClear();

    fireEvent.keyDown(window, { key, altKey: true });
    expect(navigateSpy).toHaveBeenCalledWith(route);
  });

  it('não navega se Ctrl também está pressionado', () => {
    render(<Topbar />);
    navigateSpy.mockClear();

    fireEvent.keyDown(window, { key: 'h', altKey: true, ctrlKey: true });
    expect(navigateSpy).not.toHaveBeenCalled();
  });

  it('navega para /help com F1', () => {
    render(<Topbar />);
    navigateSpy.mockClear();

    fireEvent.keyDown(window, { key: 'F1' });
    expect(navigateSpy).toHaveBeenCalledWith('/help');
  });

  it('F1 chama preventDefault e abre ajuda mesmo com um modal aberto', () => {
    modalState.open = true;
    try {
      render(<Topbar />);
      navigateSpy.mockClear();

      const event = new KeyboardEvent('keydown', { key: 'F1', bubbles: true, cancelable: true });
      window.dispatchEvent(event);

      expect(event.defaultPrevented).toBe(true);
      expect(navigateSpy).toHaveBeenCalledWith('/help');
    } finally {
      modalState.open = false;
    }
  });

  it('Alt+M não age quando um modal está aberto', () => {
    modalState.open = true;
    try {
      render(<Topbar />);
      toggleMenuSpy.mockClear();

      fireEvent.keyDown(window, { key: 'm', altKey: true });

      expect(toggleMenuSpy).not.toHaveBeenCalled();
    } finally {
      modalState.open = false;
    }
  });
});
