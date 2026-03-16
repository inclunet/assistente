import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { JsonEditor } from './JsonEditor';

vi.mock('./CodeEditor', () => ({
  CodeEditor: (props: { onChange: (value: string) => void }) => (
    <button onClick={() => props.onChange('novo')}>Editor</button>
  ),
}));

describe('JsonEditor', () => {
  it('renderiza wrapper com aria-label', () => {
    render(<JsonEditor value="{}" onChange={() => {}} />);

    expect(
      screen.getByRole('region', { name: 'Editor de JSON' })
    ).toBeInTheDocument();
  });

  it('propaga onChange para o editor', () => {
    const onChange = vi.fn();
    render(<JsonEditor value="{}" onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: 'Editor' }));
    expect(onChange).toHaveBeenCalledWith('novo');
  });
});
