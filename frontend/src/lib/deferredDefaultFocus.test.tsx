import { act, fireEvent, render, screen } from '@testing-library/react';
import { useRef, useState } from 'react';
import { expect, it, vi } from 'vitest';
import { Modal, useModalIsTopmost } from '../components/ui/Modal';
import { Menu } from '../components/menu';
import { useLandmarkNavigation } from '../hooks/useLandmarkNavigation';
import { restoreDefaultFocus } from '../hooks/useDefaultFocus';

it.each([false, true])('foco padrão não atravessa modal superior (origem modal=%s)', modalOrigin => {
  const focus = vi.fn(() => true);
  function Origin() {
    const isTopmost = useModalIsTopmost();
    useLandmarkNavigation({
      landmarks: [{ id: 'origin', label: 'Origem', focus, contains: () => false }],
      defaultLandmarkId: 'origin', allowWhenModalOpen: modalOrigin, shouldHandleKey: isTopmost,
    });
    return <textarea aria-label="Origem" />;
  }
  const view = render(<>
    {modalOrigin ? <Modal isOpen onClose={() => {}} title="Origem"><Origin /></Modal> : <Origin />}
    <Modal isOpen onClose={() => {}} title="Superior"><input aria-label="Destino" /></Modal>
  </>);
  try {
    expect(restoreDefaultFocus()).toBe(false);
    expect(focus).not.toHaveBeenCalled();
  } finally {
    view.unmount();
  }
});

it.each(['Escape', 'Copiar'])('menu dentro do modal retorna ao compositor com %s', action => {
  const frames = new Map<number, FrameRequestCallback>();
  let nextID = 0;
  const requestFrame = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(callback => {
    frames.set(++nextID, callback);
    return nextID;
  });
  const cancelFrame = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(id => { frames.delete(id); });
  const flushFrame = () => act(() => {
    const pending = [...frames.values()];
    frames.clear();
    pending.forEach(callback => callback(0));
  });
  const copied = vi.fn();
  function Content() {
    const input = useRef<HTMLTextAreaElement>(null);
    const [open, setOpen] = useState(false);
    const isTopmost = useModalIsTopmost();
    useLandmarkNavigation({
      landmarks: [{ id: 'composer', label: 'Compositor', focus: () => { input.current?.focus(); return true; }, contains: () => document.activeElement === input.current }],
      defaultLandmarkId: 'composer', allowWhenModalOpen: true, shouldHandleKey: isTopmost,
    });
    return <>
      <textarea aria-label="Compositor" ref={input} />
      <button onClick={() => setOpen(true)}>Código</button>
      <Menu visible={open} items={[{ id: 'copy', label: 'Copiar', action: copied }]} onClose={() => setOpen(false)} />
    </>;
  }
  let view: ReturnType<typeof render> | undefined;
  try {
    view = render(<Modal isOpen onClose={() => {}} title="Chat"><Content /></Modal>);
    flushFrame(); flushFrame();
    fireEvent.click(screen.getByRole('button', { name: 'Código' }));
    if (action === 'Escape') fireEvent.keyDown(screen.getByRole('menu'), { key: 'Escape' });
    else fireEvent.click(screen.getByRole('menuitem', { name: 'Copiar' }));
    flushFrame();
    expect(screen.queryByRole('menu')).not.toBeInTheDocument();
    expect(screen.getByRole('textbox', { name: 'Compositor' })).toHaveFocus();
    expect(copied).toHaveBeenCalledTimes(action === 'Copiar' ? 1 : 0);
  } finally {
    view?.unmount();
    requestFrame.mockRestore();
    cancelFrame.mockRestore();
  }
});
