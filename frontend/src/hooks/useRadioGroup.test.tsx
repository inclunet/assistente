/** @vitest-environment jsdom */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { useRadioGroup, type UseRadioGroupOptions } from './useRadioGroup';

const playBumpSoundMock = vi.fn();

vi.mock('../services/audioFeedback', () => ({
  playBumpSound: () => playBumpSoundMock(),
}));

type TestId = 'a' | 'b' | 'c';

function RadioGroupHarness(props: UseRadioGroupOptions<TestId>) {
  const ref = useRadioGroup(props);
  return (
    <div ref={ref} role="radiogroup" aria-label="test">
      {props.items.map((id) => (
        <button
          key={id}
          role="radio"
          aria-checked={props.selectedId === id}
          tabIndex={props.selectedId === id ? 0 : -1}
          onClick={() => props.onChange(id)}
        >
          {id}
        </button>
      ))}
    </div>
  );
}

describe('useRadioGroup', () => {
  const items: readonly TestId[] = ['a', 'b', 'c'];
  let onChange: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    onChange = vi.fn();
  });

  afterEach(() => {
    playBumpSoundMock.mockClear();
  });

  it('aplica roving tabindex — somente o selecionado tem tabindex 0', () => {
    render(<RadioGroupHarness items={items} selectedId="b" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    expect(radios[0]).toHaveAttribute('tabindex', '-1');
    expect(radios[1]).toHaveAttribute('tabindex', '0');
    expect(radios[2]).toHaveAttribute('tabindex', '-1');
  });

  it('ArrowRight move foco e seleciona o próximo item', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: 'ArrowRight' });

    expect(document.activeElement).toBe(radios[1]);
    expect(onChange).toHaveBeenCalledWith('b');
  });

  it('ArrowDown move foco e seleciona o próximo item', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: 'ArrowDown' });

    expect(document.activeElement).toBe(radios[1]);
    expect(onChange).toHaveBeenCalledWith('b');
  });

  it('ArrowLeft move foco e seleciona o item anterior', () => {
    render(<RadioGroupHarness items={items} selectedId="b" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[1].focus();

    fireEvent.keyDown(radios[1], { key: 'ArrowLeft' });

    expect(document.activeElement).toBe(radios[0]);
    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('ArrowUp move foco e seleciona o item anterior', () => {
    render(<RadioGroupHarness items={items} selectedId="b" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[1].focus();

    fireEvent.keyDown(radios[1], { key: 'ArrowUp' });

    expect(document.activeElement).toBe(radios[0]);
    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('wrapping circular: último → primeiro com ArrowRight', () => {
    render(<RadioGroupHarness items={items} selectedId="c" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[2].focus();

    fireEvent.keyDown(radios[2], { key: 'ArrowRight' });

    expect(document.activeElement).toBe(radios[0]);
    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('wrapping circular: primeiro → último com ArrowLeft', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: 'ArrowLeft' });

    expect(document.activeElement).toBe(radios[2]);
    expect(onChange).toHaveBeenCalledWith('c');
  });

  it('sem wrapping: toca bump ao tentar ir além do último', () => {
    render(<RadioGroupHarness items={items} selectedId="c" onChange={onChange} wrap={false} />);

    const radios = screen.getAllByRole('radio');
    radios[2].focus();

    fireEvent.keyDown(radios[2], { key: 'ArrowRight' });

    expect(document.activeElement).toBe(radios[2]);
    expect(onChange).not.toHaveBeenCalled();
    expect(playBumpSoundMock).toHaveBeenCalled();
  });

  it('sem wrapping: toca bump ao tentar ir antes do primeiro', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} wrap={false} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: 'ArrowLeft' });

    expect(document.activeElement).toBe(radios[0]);
    expect(onChange).not.toHaveBeenCalled();
    expect(playBumpSoundMock).toHaveBeenCalled();
  });

  it('Home move para o primeiro item', () => {
    render(<RadioGroupHarness items={items} selectedId="c" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[2].focus();

    fireEvent.keyDown(radios[2], { key: 'Home' });

    expect(document.activeElement).toBe(radios[0]);
    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('End move para o último item', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: 'End' });

    expect(document.activeElement).toBe(radios[2]);
    expect(onChange).toHaveBeenCalledWith('c');
  });

  it('Space aciona click no item focado', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[0].focus();

    fireEvent.keyDown(radios[0], { key: ' ' });

    expect(onChange).toHaveBeenCalledWith('a');
  });

  it('focusin atualiza tabindex para o elemento focado', () => {
    render(<RadioGroupHarness items={items} selectedId="a" onChange={onChange} />);

    const radios = screen.getAllByRole('radio');
    radios[1].focus();

    expect(radios[1]).toHaveAttribute('tabindex', '0');
    expect(radios[0]).toHaveAttribute('tabindex', '-1');
  });
});
