import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { Topbar } from './Topbar';

const navigateSpy = vi.fn();
const toggleMenuSpy = vi.fn();
const setThemeSpy = vi.fn();
const updateConfigSpy = vi.fn();
const changeLanguageSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: changeLanguageSpy },
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

vi.mock('../../hooks/useTheme', () => ({
  useTheme: () => ({ theme: 'assistente', setTheme: setThemeSpy }),
  THEMES: [
    { id: 'assistente', label: 'Assistente' },
    { id: 'claro', label: 'Claro' },
  ],
}));

vi.mock('../../lib/i18n', () => ({
  LANGUAGES: [
    { id: 'pt-BR', nativeLabel: 'Portugues' },
    { id: 'en', nativeLabel: 'English' },
  ],
}));

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: { updateConfig: (cfg: unknown) => void }) => unknown) =>
    selector({ updateConfig: updateConfigSpy }),
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
  it('renderiza titulo e configura item atual', () => {
    render(<Topbar />);

    expect(screen.getByRole('button', { name: 'menu.navLabel' })).toBeInTheDocument();
    expect(screen.getByText('menu.appTitle')).toBeInTheDocument();
    expect(screen.getByTestId('current-item')).toHaveTextContent('history');

    const items = screen.getByTestId('menu-items').textContent || '';
    expect(items).toContain('theme');
    expect(items).toContain('language');
  });

  it('abre menu com Alt+M', () => {
    render(<Topbar />);

    fireEvent.keyDown(window, { key: 'm', altKey: true });
    expect(toggleMenuSpy).toHaveBeenCalled();
  });
});
