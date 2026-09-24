import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MenuButton } from './MenuButton';

const openForTriggerSpy = vi.fn();
const closeMenuSpy = vi.fn();
let afterSelectCallback: (() => void) | undefined;

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../../hooks/useAnchoredContextMenu', async () => {
  const React = await import('react');
  return {
    useAnchoredContextMenu: (options: { onAfterSelect?: () => void } = {}) => {
      afterSelectCallback = options.onAfterSelect;
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
  it('chama callback depois da restauração síncrona de foco', () => {
    const onAfterSelect = vi.fn();
    let animationFrame: FrameRequestCallback | undefined;
    vi.spyOn(window, 'requestAnimationFrame').mockImplementation((callback) => {
      animationFrame = callback;
      return 1;
    });

    render(
      <MenuButton
        buttonLabel="Acoes"
        onAfterSelect={onAfterSelect}
        items={[{ id: 'a', label: 'Acao' }]}
      />
    );

    afterSelectCallback?.();
    expect(onAfterSelect).not.toHaveBeenCalled();
    animationFrame?.(0);
    expect(onAfterSelect).toHaveBeenCalledTimes(1);
  });

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

  it('anuncia o menu e seu estado ao abrir pelo teclado', async () => {
    const user = userEvent.setup();
    render(
      <MenuButton
        buttonLabel="Acoes"
        items={[{ id: 'a', label: 'Acao', icon: '✓' }]}
      />
    );

    const button = screen.getByRole('button', { name: 'Acoes' });
    expect(button).toHaveAttribute('aria-haspopup', 'menu');
    expect(button).toHaveAttribute('aria-expanded', 'false');

    button.focus();
    await user.keyboard('{Enter}');

    expect(button).toHaveAttribute('aria-haspopup', 'menu');
    expect(button).toHaveAttribute('aria-expanded', 'true');

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
