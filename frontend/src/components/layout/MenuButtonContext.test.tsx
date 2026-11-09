import { expect, it, vi } from 'vitest';
import { act, createEvent, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { MenuButton } from './MenuButton';
import { Modal } from '../ui/Modal';
import { registerDefaultFocus, unregisterDefaultFocus } from '../../hooks/useDefaultFocus';

it('seleção que abre um modal não restaura a página atrás dele', () => {
  vi.useFakeTimers();
  const frames = new Map<number, FrameRequestCallback>();
  let frameID = 0;
  const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    const id = ++frameID;
    frames.set(id, callback);
    return id;
  });
  const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id); });
  const notified = vi.fn();
  const restore = vi.fn(() => true);
  function View() {
    const [open, setOpen] = useState(false);
    return <>
      <MenuButton buttonLabel="Ações" onAfterSelect={notified} items={[{ id: 'edit', label: 'Editar', onClick: () => setOpen(true) }]} />
      <Modal isOpen={open} onClose={() => setOpen(false)} title="Editor"><input aria-label="Nome" /></Modal>
    </>;
  }
  let view: ReturnType<typeof render> | undefined;
  try {
    registerDefaultFocus(restore);
    view = render(<View />);
    fireEvent.click(screen.getByRole('button', { name: 'Ações' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Editar' }));
    act(() => vi.advanceTimersByTime(10));
    screen.getByRole('textbox', { name: 'Nome' }).focus();
    act(() => { for (const callback of frames.values()) callback(0); });
    expect(screen.getByRole('textbox', { name: 'Nome' })).toHaveFocus();
    expect(restore).not.toHaveBeenCalled();
    expect(notified).toHaveBeenCalledOnce();
  } finally {
    view?.unmount();
    unregisterDefaultFocus(restore);
    requestFrame.mockRestore();
    cancelFrame.mockRestore();
    vi.useRealTimers();
  }
});

it.each([
  { navigated: false, beforeCallback: false },
  { navigated: true, beforeCallback: false },
  { navigated: true, beforeCallback: true },
])('seleção tem um único retorno e preserva nova navegação ($navigated, antes do callback=$beforeCallback)', ({ navigated, beforeCallback }) => {
  vi.useFakeTimers();
  const frames: FrameRequestCallback[] = [];
  const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    frames.push(callback);
    return frames.length;
  });
  let restore: (() => boolean) | undefined;
  try {
    const selected = vi.fn();
    const notified = vi.fn();
    render(<>
      <button>Área padrão</button><button>Próxima ação</button>
      <MenuButton buttonLabel="Ações" onAfterSelect={notified} items={[{ id: 'edit', label: 'Editar', onClick: selected }]} />
    </>);
    const defaultArea = screen.getByRole('button', { name: 'Área padrão' });
    const chosen = screen.getByRole('button', { name: 'Próxima ação' });
    const restoreSpy = vi.fn(() => { defaultArea.focus(); return true; });
    restore = restoreSpy;
    registerDefaultFocus(restore);
    fireEvent.click(screen.getByRole('button', { name: 'Ações' }));
    fireEvent.click(screen.getByRole('menuitem', { name: 'Editar' }));
    if (navigated && beforeCallback) chosen.focus();
    act(() => vi.advanceTimersByTime(10));
    if (navigated && !beforeCallback) chosen.focus();
    act(() => frames.forEach(callback => callback(0)));
    expect(selected).toHaveBeenCalledOnce();
    expect(notified).toHaveBeenCalledOnce();
    expect(restoreSpy).toHaveBeenCalledTimes(navigated ? 0 : 1);
    expect(navigated ? chosen : defaultArea).toHaveFocus();
  } finally {
    if (restore) unregisterDefaultFocus(restore);
    requestFrame.mockRestore();
    vi.useRealTimers();
  }
});

it('fecha opções obsoletas pelo fluxo normal e devolve foco ao mesmo gatilho', async () => {
  const action = vi.fn();
  const { rerender } = render(<MenuButton buttonLabel="Ações" contextKey="ready" items={[{ id: 'edit', label: 'Editar', onClick: action }]} />);
  const trigger = screen.getByRole('button', { name: 'Ações' });
  await userEvent.click(trigger);
  screen.getByRole('menuitem', { name: 'Editar' }).focus();
  rerender(<MenuButton buttonLabel="Ações" contextKey="loading" items={[{ id: 'edit', label: 'Editar', disabled: true, onClick: action }]} />);
  expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  expect(screen.getByRole('button', { name: 'Ações' })).toBe(trigger);
  await waitFor(() => expect(trigger).toHaveFocus());
  expect(action).not.toHaveBeenCalled();
  await userEvent.click(trigger);
  expect(screen.getByRole('menuitem', { name: 'Editar' })).toBeDisabled();
});

it('não troca nem desfoca o gatilho fechado quando o contexto muda', () => {
  const { rerender } = render(<MenuButton buttonLabel="Ações" contextKey="one" items={[]} />);
  const trigger = screen.getByRole('button', { name: 'Ações' });
  trigger.focus();
  rerender(<MenuButton buttonLabel="Ações" contextKey="two" items={[]} />);
  expect(screen.getByRole('button', { name: 'Ações' })).toBe(trigger);
  expect(trigger).toHaveFocus();
});


it.each([false, true])('Tab fecha menu desabilitado sem cancelar navegação nativa (reverse=%s)', async reverse => {
  render(<MenuButton buttonLabel="Ações" items={[{ id: 'edit', label: 'Editar', disabled: true }]} />);
  const trigger = screen.getByRole('button', { name: 'Ações' });
  await userEvent.click(trigger);
  expect(screen.getByRole('menu')).toHaveFocus();
  const event = createEvent.keyDown(document.activeElement!, { key: 'Tab', shiftKey: reverse });
  fireEvent(document.activeElement!, event);
  expect(event.defaultPrevented).toBe(false);
  expect(screen.queryByRole('menu')).not.toBeInTheDocument();
  expect(trigger).toHaveFocus();
});
