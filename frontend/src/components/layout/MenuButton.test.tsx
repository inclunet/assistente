import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MenuButton } from './MenuButton';

const openForTriggerSpy = vi.fn();
const closeMenuSpy = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../../hooks/useAnchoredContextMenu', async () => {
  const React = await import('react');
  return {
    useAnchoredContextMenu: () => {
      const [menu, setMenu] = React.useState<{
        visible: boolean;
        x: number;
        y: number;
        items: Array<{ id: string; label: string; icon?: string }>;
        ariaLabel: string;
      }>(
        {
          visible: false,
          x: 0,
          y: 0,
          items: [],
          ariaLabel: '',
        }
      );

      const openForTrigger = (
        _trigger: HTMLElement,
        ariaLabel: string,
        items: Array<{ id: string; label: string; icon?: string }>
      ) => {
        openForTriggerSpy(_trigger, ariaLabel, items);
        setMenu({ visible: true, x: 0, y: 0, items, ariaLabel });
      };

      const closeMenu = () => {
        closeMenuSpy();
        setMenu((prev) => ({ ...prev, visible: false }));
      };

      return {
        menu,
        openForTrigger,
        closeMenu,
        onSelectItem: vi.fn(),
      };
    },
  };
});

describe('MenuButton', () => {
  it('abre menu ao clicar no botao', async () => {
    const user = userEvent.setup();
    render(
      <MenuButton
        buttonLabel="Acoes"
        items={[{ id: 'a', label: 'Acao', icon: '✓' }]}
      />
    );

    await user.click(screen.getByRole('button', { name: 'Acoes' }));

    expect(openForTriggerSpy).toHaveBeenCalled();
    expect(screen.getByRole('menuitem', { name: 'Acao' })).toBeInTheDocument();
  });

  it('usa tabIndex -1 quando dentro do grid', () => {
    render(
      <div className="datagrid-cell">
        <MenuButton
          buttonLabel="Acoes"
          items={[{ id: 'a', label: 'Acao', icon: '✓' }]}
        />
      </div>
    );

    const button = screen.getByRole('button', { name: 'Acoes' });
    expect(button).toHaveAttribute('tabindex', '-1');
  });
});
