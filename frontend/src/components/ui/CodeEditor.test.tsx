import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { CodeEditor } from './CodeEditor';

vi.mock('@monaco-editor/react', async () => {
  const React = await import('react');
  return {
    default: (props: {
      value: string;
      onChange?: (value: string) => void;
      onMount?: (editor: unknown, monaco: unknown) => void;
    }) => {
      React.useEffect(() => {
        props.onMount?.({ getDomNode: () => document.createElement('div') }, {});
      }, [props]);

      return (
        <textarea
          data-testid="monaco"
          value={props.value}
          onChange={(event) => props.onChange?.(event.target.value)}
        />
      );
    },
  };
});

vi.mock('../../lib/monacoLanguageLoader', () => ({
  loadMonacoLanguage: () => Promise.resolve(),
}));

describe('CodeEditor', () => {
  it('renderiza placeholder quando vazio', () => {
    render(
      <CodeEditor
        value=""
        onChange={() => {}}
        placeholder="Sem conteudo"
        ariaLabel="Editor"
      />
    );

    expect(screen.getByText('Sem conteudo')).toBeInTheDocument();
    expect(screen.getByRole('region', { name: 'Editor' })).toBeInTheDocument();
  });

  it('dispara onChange ao editar', () => {
    const onChange = vi.fn();

    render(<CodeEditor value="a" onChange={onChange} ariaLabel="Editor" />);

    const textarea = screen.getByTestId('monaco');
    fireEvent.change(textarea, { target: { value: 'b' } });

    expect(onChange).toHaveBeenCalledWith('b');
  });
});
