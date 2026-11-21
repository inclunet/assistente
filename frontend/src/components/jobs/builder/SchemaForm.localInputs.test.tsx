import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { SchemaForm } from './SchemaForm';

it('permite configurar um requisito do plugin ausente no esquema local', () => {
  const change = vi.fn();
  render(<SchemaForm schema={null} configuredFields={['options']} values={{}} onChange={change} />);
  const field = screen.getByRole('textbox', {name:'options'});
  fireEvent.change(field, {target:{value:'{"count":0}'}});
  expect(change).toHaveBeenLastCalledWith('options', {count:0});
  fireEvent.change(field, {target:{value:'false'}});
  expect(change).toHaveBeenLastCalledWith('options', false);
  fireEvent.change(field, {target:{value:'{'}});
  expect(change).toHaveBeenLastCalledWith('options', undefined);
});
