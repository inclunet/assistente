import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { CommandArgumentsDialog, equalJSON } from './CommandArgumentsDialog';

describe('CommandArgumentsDialog', () => {
  it('preserves literal sentinel-looking strings and accepts a required empty string', () => {
    const onSubmit = vi.fn();
    const schema = JSON.parse('{"type":"object","required":["token","enumLike","empty"],"properties":{"token":{"type":"string"},"enumLike":{"type":"string"},"empty":{"type":"string","minLength":0}}}') as unknown;
    render(<CommandArgumentsDialog open commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);

    fireEvent.change(screen.getByRole('textbox', { name: /^token/ }), { target: { value: '__null__' } });
    fireEvent.change(screen.getByRole('textbox', { name: /^enumLike/ }), { target: { value: '__enum__:0' } });
    fireEvent.change(screen.getByRole('textbox', { name: /^empty/ }), { target: { value: '' } });
    expect(screen.getByRole('textbox', { name: /^empty/ })).toHaveAttribute('aria-required', 'true');
    expect(screen.getByRole('textbox', { name: /^empty/ })).not.toHaveAttribute('required');
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));

    expect(onSubmit).toHaveBeenCalledWith({ token: '__null__', enumLike: '__enum__:0', empty: '' });
  });

  it('converts the nullable-string select sentinel to null but preserves the same literal in its input', () => {
    const onSubmit = vi.fn();
    const schema = { type: 'object', required: ['value'], properties: { value: { type: ['string', 'null'] } } };
    const { rerender } = render(<CommandArgumentsDialog open commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);
    fireEvent.change(screen.getByLabelText('value'), { target: { value: 'null' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));
    expect(onSubmit).toHaveBeenCalledWith({ value: null });

    onSubmit.mockClear();
    rerender(<CommandArgumentsDialog open={false} commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);
    rerender(<CommandArgumentsDialog open commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);
    expect(screen.getByLabelText('value')).toHaveValue('');
    fireEvent.change(screen.getByLabelText('value'), { target: { value: 'value' } });
    fireEvent.change(screen.getAllByRole('textbox')[0], { target: { value: '__null__' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));
    expect(onSubmit).toHaveBeenCalledWith({ value: '__null__' });
  });

  it('normalizes the real Go Schema DTO and keeps boolean, null, and required nullable-empty selections', () => {
    const onSubmit = vi.fn();
    const schema = {
      Type: 'object',
      Optional: false,
      Nullable: false,
      Required: null,
      Properties: {
        enabled: { Type: 'boolean', Optional: false, Nullable: false, Required: null },
        nullableNull: { Type: 'string', Optional: false, Nullable: true, Required: null, MinLength: 0 },
        nullableBlank: { Type: 'string', Optional: false, Nullable: true, Required: null, MinLength: 0 },
      },
    };
    render(<CommandArgumentsDialog open commandName="tool.execute" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);

    const enabled = screen.getByLabelText('enabled');
    fireEvent.change(enabled, { target: { value: 'false' } });
    expect(enabled).toHaveValue('false');
    fireEvent.change(enabled, { target: { value: 'true' } });
    expect(enabled).toHaveValue('true');

    const nullableNull = screen.getByLabelText('nullableNull');
    fireEvent.change(nullableNull, { target: { value: 'null' } });
    expect(nullableNull).toHaveValue('null');

    const nullableBlank = screen.getByLabelText('nullableBlank');
    fireEvent.change(nullableBlank, { target: { value: 'value' } });
    expect(nullableBlank).toHaveValue('value');
    const blankInput = screen.getByRole('textbox', { name: /^commandPalette.arguments.value/ });
    expect(blankInput).toHaveValue('');
    expect(blankInput).toHaveAttribute('aria-required', 'true');
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));

    expect(onSubmit).toHaveBeenCalledWith({ enabled: true, nullableNull: null, nullableBlank: '' });
  });

  it('treats proto-sensitive schema and argument names as own data properties', () => {
    const onSubmit = vi.fn();
    const schema = JSON.parse('{"type":"object","required":["__proto__","constructor"],"properties":{"__proto__":{"type":"string"},"constructor":{"type":"string"}}}') as unknown;
    render(<CommandArgumentsDialog open commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);

    fireEvent.change(screen.getByRole('textbox', { name: /^__proto__/ }), { target: { value: 'safe' } });
    fireEvent.change(screen.getByRole('textbox', { name: /^constructor/ }), { target: { value: 'valid' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));

    expect(onSubmit).toHaveBeenCalledTimes(1);
    const args = onSubmit.mock.calls[0][0] as Record<string, unknown>;
    expect(Object.getPrototypeOf(args)).toBeNull();
    expect(Object.prototype.hasOwnProperty.call(args, '__proto__')).toBe(true);
    expect(Object.prototype.hasOwnProperty.call(args, 'constructor')).toBe(true);
    expect(args['__proto__']).toBe('safe');
    expect(args.constructor).toBe('valid');
  });

  it('does not allow an optional empty text field to bypass the explicit skip state', () => {
    const onSubmit = vi.fn();
    const schema = { type: 'object', properties: { note: { type: 'string' } } };
    render(<CommandArgumentsDialog open commandName="test" schema={schema} onCancel={vi.fn()} onSubmit={onSubmit} />);
    fireEvent.change(screen.getByRole('textbox', { name: /^note/ }), { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));
    expect(onSubmit).toHaveBeenCalledWith({});
  });

  it('edits arguments_json as a JSON textarea but submits the required raw string unchanged', () => {
    const onSubmit = vi.fn();
    const schema = { type: 'object', required: ['arguments_json'], properties: { arguments_json: { type: 'string' } } };
    render(<CommandArgumentsDialog open commandName="tool.execute" schema={schema} jsonStringFields={['arguments_json']} onCancel={vi.fn()} onSubmit={onSubmit} />);
    const textarea = screen.getByRole('textbox', { name: /^arguments_json/ });
    fireEvent.change(textarea, { target: { value: '{broken' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));
    expect(onSubmit).not.toHaveBeenCalled();
    fireEvent.change(textarea, { target: { value: '{"query":"hello"}' } });
    fireEvent.click(screen.getByRole('button', { name: 'commandPalette.arguments.submit' }));
    expect(onSubmit).toHaveBeenCalledWith({ arguments_json: '{"query":"hello"}' });
  });

  it('compares enum JSON objects recursively without depending on key order', () => {
    const schemaValue = { outer: { first: 1, second: [{ left: true, right: null }] } };
    const inputValue = { outer: { second: [{ right: null, left: true }], first: 1 } };
    expect(equalJSON(schemaValue, inputValue)).toBe(true);
    expect(equalJSON(schemaValue, { outer: { first: 1, second: [{ left: false, right: null }] } })).toBe(false);
  });
});
