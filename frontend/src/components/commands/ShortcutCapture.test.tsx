import { axe } from 'vitest-axe';
import userEvent from '@testing-library/user-event';
import { act, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { ShortcutCapture } from './ShortcutCapture';
import { createLocalCommandKeyboard } from '../../lib/commandLocalKeyboard';
import { isCommandShortcutSequence, serializeCommandKeyboardTrigger } from '../../lib/commandShortcut';

const announce = vi.fn();
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce }) }));

describe('ShortcutCapture', () => {
  const sequence = { version: 2 as const, steps: [
    { code: 'KeyY', modifiers: ['Control' as const] }, { code: 'KeyL', modifiers: [] },
  ] as [{ code: string; modifiers: ('Control')[] }, { code: string; modifiers: [] }] };

  it('grava dois passos atomicamente e o dispatcher executa exatamente o documento gravado', async () => {
    const onChange = vi.fn();
    const onCapturingChange = vi.fn();
    const view = render(<ShortcutCapture allowSequences value={null} onChange={onChange} onCapturingChange={onCapturingChange} />);
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'sequence' } });
    const button = screen.getByRole('button');
    // Clique sintetizado também funciona, inclusive por tecnologia assistiva.
    fireEvent.click(button);
    fireEvent.keyDown(button, { code: 'KeyY', key: 'y', ctrlKey: true });
    expect(onChange).not.toHaveBeenCalled();
    expect(screen.getByRole('combobox')).toBeDisabled();
    expect(announce).toHaveBeenLastCalledWith('commandShortcutCapture.nextStep');
    fireEvent.keyUp(button, { code: 'KeyY', ctrlKey: true });
    fireEvent.keyUp(button, { code: 'ControlLeft' });
    fireEvent.keyDown(button, { code: 'KeyL', key: 'l' });
    expect(onChange).toHaveBeenCalledExactlyOnceWith(sequence);
    expect(onCapturingChange.mock.calls).toEqual([[true], [false]]);
    const recorded = onChange.mock.calls[0][0];
    expect(isCommandShortcutSequence(JSON.parse(serializeCommandKeyboardTrigger(recorded)))).toBe(true);
    view.unmount();
    const onDown = vi.fn().mockResolvedValue(undefined);
    const controller = createLocalCommandKeyboard({ target: window,
      loadMap: async () => ({ generation: 'captured', bindings: [{ shortcut: recorded, commandId: 'workspace.list', handler: 'backend' }] }),
      blocked: () => false, onDown, onUp: async () => {}, reset: async () => {},
    });
    try {
      await controller.refresh();
      fireEvent.keyDown(window, { code: 'KeyY', key: 'y', ctrlKey: true });
      expect(onDown).not.toHaveBeenCalled();
      fireEvent.keyUp(window, { code: 'KeyY' });
      fireEvent.keyDown(window, { code: 'KeyL', key: 'l' });
      expect(onDown).toHaveBeenCalledExactlyOnceWith(expect.objectContaining({ shortcut: sequence, commandId: 'workspace.list', repeat: false }));
    } finally { controller.dispose(); }
  });

  it('exige prefixo modificado, release físico e final sem modificadores; ignora repeat e IME', () => {
    const onChange = vi.fn();
    render(<ShortcutCapture allowSequences value={sequence} onChange={onChange} />);
    const button = screen.getByRole('button');
    fireEvent.click(button);
    fireEvent.keyDown(button, { code: 'KeyY', key: 'y' });
    expect(announce).toHaveBeenLastCalledWith('commandShortcutCapture.prefixInvalid');
    fireEvent.keyDown(button, { code: 'KeyY', key: 'y', ctrlKey: true });
    fireEvent.keyDown(button, { code: 'KeyL', key: 'l' });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.keyUp(button, { code: 'KeyY' });
    fireEvent.keyDown(button, { code: 'KeyL', key: 'L', shiftKey: true });
    fireEvent.keyDown(button, { code: 'KeyL', key: 'l', repeat: true });
    fireEvent.keyDown(button, { code: 'KeyL', key: 'l', isComposing: true });
    expect(onChange).not.toHaveBeenCalled();
    fireEvent.keyDown(button, { code: 'KeyL', key: 'l' });
    expect(onChange).toHaveBeenCalledExactlyOnceWith(sequence);
  });

  it.each(['Escape', 'Tab', 'window-blur', 'disabled', 'unmount'])('cancela prefixo parcial por %s sem substituir v2 anterior', (reason) => {
    const onChange = vi.fn();
    const changed = vi.fn();
    const view = render(<ShortcutCapture allowSequences value={sequence} onChange={onChange} onCapturingChange={changed} />);
    const button = screen.getByRole('button');
    fireEvent.click(button);
    fireEvent.keyDown(button, { code: 'KeyK', key: 'k', ctrlKey: true });
    if (reason === 'disabled') view.rerender(<ShortcutCapture allowSequences value={sequence} onChange={onChange} onCapturingChange={changed} disabled />);
    else if (reason === 'unmount') view.unmount();
    else if (reason === 'window-blur') fireEvent.blur(window);
    else fireEvent.keyDown(button, { code: reason, key: reason });
    expect(onChange).not.toHaveBeenCalled();
    expect(changed.mock.calls).toEqual([[true], [false]]);
    if (reason !== 'unmount') expect(button).toHaveTextContent('Control+KeyY KeyL');
  });

  it('permite mudar o próximo tipo sem apagar o documento atual e mantém captura simples para global', () => {
    const onChange = vi.fn();
    const view = render(<ShortcutCapture allowSequences value={sequence} onChange={onChange} />);
    fireEvent.change(screen.getByRole('combobox'), { target: { value: 'simple' } });
    expect(onChange).not.toHaveBeenCalled();
    const button = screen.getByRole('button');
    fireEvent.click(button);
    fireEvent.keyDown(button, { code: 'KeyY', key: 'y', ctrlKey: true });
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ version: 1, code: 'KeyY', modifiers: ['Control'] });
    view.rerender(<ShortcutCapture value={null} onChange={onChange} />);
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
  });

  it('modo sequência tem rótulos e descrição acessíveis', async () => {
    const { container } = render(<ShortcutCapture allowSequences value={sequence} onChange={vi.fn()} />);
    expect(screen.getByRole('button')).toHaveAccessibleDescription();
    expect(await axe(container, { rules: { 'color-contrast': { enabled: false } } })).toHaveNoViolations();
  });

  it.each([{ code: 'Enter', key: '{Enter}' }, { code: 'Space', key: ' ' }])('final $code não reinicia a gravação por clique sintetizado', async ({ code, key }) => {
    const onChange = vi.fn();
    render(<ShortcutCapture allowSequences value={sequence} onChange={onChange} />);
    const button = screen.getByRole('button');
    button.focus();
    await userEvent.setup().keyboard('{Enter}');
    fireEvent.keyDown(button, { code: 'KeyY', key: 'y', ctrlKey: true });
    fireEvent.keyUp(button, { code: 'KeyY' });
    await userEvent.setup().keyboard(key);
    expect(onChange).toHaveBeenCalledExactlyOnceWith({ version: 2, steps: [sequence.steps[0], { code, modifiers: [] }] });
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });
  it('captura Ctrl+N sem deixar o atalho legado receber o evento', async () => {
    const onChange = vi.fn();
    const legacy = vi.fn();
    render(<div onKeyDown={legacy}><ShortcutCapture value={null} onChange={onChange} /></div>);
    const button = screen.getByRole('button');
    await userEvent.setup().click(button);
    fireEvent.keyDown(button, { code: 'KeyN', key: 'n', ctrlKey: true });
    expect(onChange).toHaveBeenCalledWith({ version: 1, code: 'KeyN', modifiers: ['Control'] });
    expect(legacy).not.toHaveBeenCalled();
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });

  it('Esc, Tab e blur encerram sem alterar o valor', async () => {
    const onChange = vi.fn();
    render(<ShortcutCapture value={null} onChange={onChange} />);
    const button = screen.getByRole('button');
    const user = userEvent.setup();
    await user.click(button);
    expect(button).toHaveAttribute('aria-pressed', 'true');
    fireEvent.keyDown(button, { key: 'Escape', code: 'Escape' });
    expect(button).toHaveAttribute('aria-pressed', 'false');
    expect(onChange).not.toHaveBeenCalled();
    await user.click(button);
    fireEvent.blur(button, { relatedTarget: null });
    expect(button).toHaveAttribute('aria-pressed', 'false');
    expect(onChange).not.toHaveBeenCalled();
    await user.click(button);
    await user.keyboard('{Tab}');
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });

  it('blur não rouba foco do elemento que recebeu o foco externo', async () => {
    const onChange = vi.fn();
    render(<><input aria-label="outside" /><ShortcutCapture value={null} onChange={onChange} /></>);
    const input = screen.getByRole('textbox');
    const button = screen.getByRole('button');
    input.focus();
    await userEvent.setup().click(button);
    const other = document.createElement('button');
    document.body.append(other);
    act(() => {
      other.focus();
      fireEvent.blur(button, { relatedTarget: null });
    });
    expect(document.activeElement).toBe(other);
    other.remove();
  });

  it('não reabre após Enter ou Space e cancela ao desabilitar', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    const { rerender } = render(<ShortcutCapture value={null} onChange={onChange} />);
    const button = screen.getByRole('button');
    await user.click(button);
    await user.keyboard('{Enter}');
    expect(button).toHaveAttribute('aria-pressed', 'false');
    await user.click(button);
    await user.keyboard(' ');
    expect(button).toHaveAttribute('aria-pressed', 'false');
    expect(onChange).toHaveBeenNthCalledWith(1, { version: 1, code: 'Enter', modifiers: [] });
    expect(onChange).toHaveBeenNthCalledWith(2, { version: 1, code: 'Space', modifiers: [] });
    await user.click(button);
    expect(button).toHaveAttribute('aria-pressed', 'true');
    rerender(<ShortcutCapture value={null} onChange={onChange} disabled />);
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });

  it('ignora repeat, IME e modifiers-only sem anúncios inválidos', async () => {
    announce.mockClear();
    const onChange = vi.fn();
    render(<ShortcutCapture value={null} onChange={onChange} />);
    const button = screen.getByRole('button');
    await userEvent.setup().click(button);
    fireEvent.keyDown(button, { code: 'ControlLeft', key: 'Control', ctrlKey: true });
    fireEvent.keyDown(button, { code: 'KeyA', key: 'a', repeat: true });
    fireEvent.keyDown(button, { code: 'KeyA', key: 'a', isComposing: true });
    expect(onChange).not.toHaveBeenCalled();
    expect(announce).toHaveBeenCalledTimes(1);
  });

  it('exige observar ControlLeft e AltLeft antes de aceitar Ctrl+Alt', async () => {
    const onChange = vi.fn();
    render(<ShortcutCapture value={null} onChange={onChange} />);
    const button = screen.getByRole('button');
    await userEvent.setup().click(button);

    fireEvent.keyDown(button, { code: 'Digit1', key: '1', ctrlKey: true, altKey: true });
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.keyDown(button, { code: 'ControlLeft', key: 'Control', ctrlKey: true });
    fireEvent.keyDown(button, { code: 'AltLeft', key: 'Alt', ctrlKey: true, altKey: true });
    fireEvent.keyDown(button, { code: 'Digit1', key: '1', ctrlKey: true, altKey: true });

    expect(onChange).toHaveBeenCalledWith({ version: 1, code: 'Digit1', modifiers: ['Control', 'Alt'] });
    expect(button).toHaveAttribute('aria-pressed', 'false');
  });

  it('observa release no keyup e rejeita AltRight/AltGraph', async () => {
    const onChange = vi.fn();
    render(<ShortcutCapture value={null} onChange={onChange} />);
    const button = screen.getByRole('button');
    await userEvent.setup().click(button);

    fireEvent.keyDown(button, { code: 'ControlLeft', key: 'Control', ctrlKey: true });
    fireEvent.keyDown(button, { code: 'AltLeft', key: 'Alt', ctrlKey: true, altKey: true });
    fireEvent.keyUp(button, { code: 'AltLeft', key: 'Alt', ctrlKey: true, altKey: false });
    fireEvent.keyDown(button, { code: 'Digit1', key: '1', ctrlKey: true, altKey: true });
    expect(onChange).not.toHaveBeenCalled();

    fireEvent.keyDown(button, { code: 'AltRight', key: 'Alt', ctrlKey: true, altKey: true });
    fireEvent.keyDown(button, { code: 'Digit1', key: '1', ctrlKey: true, altKey: true });
    expect(onChange).not.toHaveBeenCalled();

    const altGraph = new KeyboardEvent('keydown', {
      code: 'Digit1',
      key: '1',
      ctrlKey: true,
      altKey: true,
      bubbles: true,
      cancelable: true,
    });
    Object.defineProperty(altGraph, 'getModifierState', { value: (name: string) => name === 'AltGraph' });
    fireEvent(button, altGraph);
    expect(onChange).not.toHaveBeenCalled();
  });

  it('é acessível', async () => {
    const { container } = render(<ShortcutCapture value={null} onChange={vi.fn()} />);
    // jsdom não renderiza pixels para calcular contraste; a verificação visual
    // manual de contraste permanece pendente, como no padrão de JobsPage.
    expect(await axe(container, {
      rules: { 'color-contrast': { enabled: false } },
    })).toHaveNoViolations();
  });
});
