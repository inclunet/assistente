import { act, fireEvent, render, renderHook, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { useAnchoredContextMenu } from './useAnchoredContextMenu';

it('callback capturado antes de abrir ainda fecha o menu de uma leitura assíncrona', () => {
  const completed = vi.fn();
  const hook = renderHook(() => useAnchoredContextMenu({ onAfterDismiss: completed }));
  const close = hook.result.current.closeMenu;
  act(() => hook.result.current.openForTrigger(document.createElement('button'), 'Carregando', []));
  act(() => close());
  expect(hook.result.current.menu.visible).toBe(false);
  expect(completed).toHaveBeenCalledOnce();
});

it('notifica o fechamento após remover o menu do DOM, sem depender de timer', () => {
  let menuAtCompletion: HTMLElement | null | undefined;
  const completed = vi.fn(() => { menuAtCompletion = screen.queryByRole('menu'); });
  function View() {
    const { menu, openForTrigger, closeMenu } = useAnchoredContextMenu({ onAfterDismiss: completed });
    return <>
      <button onClick={event => openForTrigger(event.currentTarget, 'Ações', [])}>Abrir</button>
      {menu.visible && <div role="menu"><button onClick={closeMenu}>Fechar</button></div>}
    </>;
  }
  render(<View />);
  fireEvent.click(screen.getByRole('button', { name: 'Abrir' }));
  fireEvent.click(screen.getByRole('button', { name: 'Fechar' }));
  expect(completed).toHaveBeenCalledOnce();
  expect(menuAtCompletion).toBeNull();
});

it.each(['reabrir', 'desmontar'] as const)('não executa fechamento obsoleto ao %s antes do commit', change => {
  const completed = vi.fn();
  const first = document.createElement('button');
  const second = document.createElement('button');
  const hook = renderHook(() => useAnchoredContextMenu({ onAfterDismiss: completed }));
  vi.useFakeTimers();
  try {
    act(() => hook.result.current.openForTrigger(first, 'Primeiro', []));
    act(() => {
      hook.result.current.closeMenu();
      if (change === 'reabrir') hook.result.current.openForTrigger(second, 'Segundo', []);
      else hook.unmount();
    });
    act(() => vi.runAllTimers());
    expect(completed).not.toHaveBeenCalled();
    if (change === 'reabrir') {
      expect(hook.result.current.menu.visible).toBe(true);
      expect(hook.result.current.menu.ariaLabel).toBe('Segundo');
    }
  } finally {
    hook.unmount();
    vi.useRealTimers();
  }
});
