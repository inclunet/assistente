import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { describe, expect, it } from 'vitest';
import { CommandObjectFieldsEditor } from './CommandObjectFieldsEditor';

function Harness() {
  const [value, setValue] = useState<Record<string, unknown>>({ amount: 3, confirm: false, nested: { names: ['one', 'two'] } });
  const [valid, setValid] = useState(true);
  return (
    <>
    <CommandObjectFieldsEditor
      label="Arguments"
      value={value}
      onChange={setValue}
      onValidityChange={setValid}
      valueLabel="Value"
      typeLabel="Type"
      removeLabel={(index) => `Remove ${index}`}
      addLabel="Add"
      typeOptions={{ string: 'Text', number: 'Number', boolean: 'Boolean', json: 'JSON' }}
    />
    <output data-testid="arguments-state">{JSON.stringify(value)}</output>
    <output data-testid="arguments-valid">{String(valid)}</output>
    </>
  );
}

describe('CommandObjectFieldsEditor', () => {
  it('preserva foco ao editar a chave e mantém valores tipados', async () => {
    const user = userEvent.setup();
    render(<Harness />);

    const key = screen.getByLabelText('commandSettings.argumentEditor.key 1');
    await user.clear(key);
    await user.type(key, 'total');
    expect(document.activeElement).toBe(screen.getByLabelText('commandSettings.argumentEditor.key 1'));
    expect(screen.getByLabelText('Value 1')).toHaveValue('3');
    expect(screen.getByLabelText('Value 2')).not.toBeChecked();
    expect(screen.getByLabelText('Value 3')).toHaveValue('{"names":["one","two"]}');

    fireEvent.change(screen.getByLabelText('Type 1'), { target: { value: 'string' } });
    expect(screen.getByLabelText('Value 1')).toHaveValue('');
  });

  it('mantém rascunho JSON incompleto e sinaliza invalidez sem gravar valor antigo', () => {
    render(<Harness />);
    const nested = screen.getByLabelText('Value 3');
    fireEvent.change(nested, { target: { value: '{' } });
    expect(nested).toHaveValue('{');
    expect(screen.getByTestId('arguments-valid')).toHaveTextContent('false');
    expect(screen.getByTestId('arguments-state')).toHaveTextContent('"names":["one","two"]');
    fireEvent.change(nested, { target: { value: '{"names":["new"]}' } });
    expect(screen.getByTestId('arguments-valid')).toHaveTextContent('true');
    expect(screen.getByTestId('arguments-state')).toHaveTextContent('"names":["new"]');
  });

  it('não sobrescreve outro argumento ao editar uma chave duplicada', () => {
    render(<Harness />);
    fireEvent.change(screen.getByLabelText('commandSettings.argumentEditor.key 1'), { target: { value: 'confirm' } });
    expect(screen.getByTestId('arguments-valid')).toHaveTextContent('false');
    expect(screen.getByTestId('arguments-state')).toHaveTextContent('"amount":3,"confirm":false');
    expect(screen.getByLabelText('Value 1')).toHaveValue('3');
    expect(screen.getByLabelText('Value 2')).not.toBeChecked();
  });

  it('não converte número vazio em zero e permite mudar booleano por checkbox', () => {
    render(<Harness />);
    fireEvent.change(screen.getByLabelText('Value 1'), { target: { value: '' } });
    expect(screen.getByTestId('arguments-valid')).toHaveTextContent('false');
    expect(screen.getByTestId('arguments-state')).toHaveTextContent('"amount":3');
    fireEvent.change(screen.getByLabelText('Value 1'), { target: { value: '4.5' } });
    fireEvent.click(screen.getByLabelText('Value 2'));
    expect(screen.getByTestId('arguments-state')).toHaveTextContent('"amount":4.5,"confirm":true');
  });
});
