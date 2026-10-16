import { render } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { Modal } from '../components/ui/Modal';
import { useNewItemShortcut } from './useNewItemShortcut';

function Shortcut({ onNew, enabled = true }: { onNew: () => void; enabled?: boolean }) {
  useNewItemShortcut(onNew, enabled);
  return null;
}

function pressCtrlN(flags: KeyboardEventInit = {}, alreadyConsumed = false) {
  const event = new KeyboardEvent('keydown', {
    key: 'n',
    ctrlKey: true,
    bubbles: true,
    cancelable: true,
    ...flags,
  });
  if (alreadyConsumed) event.preventDefault();
  document.dispatchEvent(event);
  return event;
}

describe('useNewItemShortcut', () => {
  it('executa Ctrl+N normal fora de modal', () => {
    const onNew = vi.fn();
    render(<Shortcut onNew={onNew} />);

    const event = pressCtrlN();

    expect(onNew).toHaveBeenCalledOnce();
    expect(event.defaultPrevented).toBe(true);
  });

  it.each([
    ['evento já consumido', {}, true],
    ['repetição', { repeat: true }, false],
    ['composição IME', { isComposing: true }, false],
    ['keyCode 229', { keyCode: 229 }, false],
  ])('não executa com %s', (_reason, flags, alreadyConsumed) => {
    const onNew = vi.fn();
    render(<Shortcut onNew={onNew} />);

    pressCtrlN(flags, alreadyConsumed);

    expect(onNew).not.toHaveBeenCalled();
  });

  it('ignora AltGraph', () => {
    const onNew = vi.fn();
    render(<Shortcut onNew={onNew} />);
    const event = new KeyboardEvent('keydown', { key: 'n', ctrlKey: true, bubbles: true, cancelable: true });
    Object.defineProperty(event, 'getModifierState', { value: (modifier: string) => modifier === 'AltGraph' });

    document.dispatchEvent(event);

    expect(onNew).not.toHaveBeenCalled();
    expect(event.defaultPrevented).toBe(false);
  });

  it('executa somente no modal topmost, não no modal inativo', () => {
    const lowerOnNew = vi.fn();
    const upperOnNew = vi.fn();
    const { rerender } = render(
      <>
        <Modal isOpen onClose={vi.fn()} title="Inferior">
          <Shortcut onNew={lowerOnNew} />
        </Modal>
        <Modal isOpen={false} onClose={vi.fn()} title="Superior">
          <Shortcut onNew={upperOnNew} />
        </Modal>
      </>,
    );

    pressCtrlN();
    expect(lowerOnNew).toHaveBeenCalledOnce();
    expect(upperOnNew).not.toHaveBeenCalled();

    rerender(
      <>
        <Modal isOpen onClose={vi.fn()} title="Inferior">
          <Shortcut onNew={lowerOnNew} />
        </Modal>
        <Modal isOpen onClose={vi.fn()} title="Superior">
          <Shortcut onNew={upperOnNew} />
        </Modal>
      </>,
    );

    pressCtrlN();
    expect(lowerOnNew).toHaveBeenCalledOnce();
    expect(upperOnNew).toHaveBeenCalledOnce();
  });
});
