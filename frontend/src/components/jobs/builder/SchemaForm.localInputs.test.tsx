import { useState } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { SchemaForm } from './SchemaForm';

it('permite editar uma entrada do job ausente no esquema local', () => {
  const change = vi.fn();
  function Form() {
    const [drafts,setDrafts]=useState<Record<string,string>>({});
    return <SchemaForm schema={null} values={{options:null}} onChange={change}
      localDrafts={drafts} onLocalDraft={(name,raw) => setDrafts(previous=>({...previous,[name]:raw}))} />;
  }
  render(<Form />);
  const field = screen.getByRole('textbox', {name:'options'});
  fireEvent.change(field, {target:{value:'{"count":0}'}});
  expect(change).toHaveBeenLastCalledWith('options', {count:0});
  fireEvent.change(field, {target:{value:'false'}});
  expect(change).toHaveBeenLastCalledWith('options', false);
  fireEvent.change(field, {target:{value:'{'}});
  expect(field).toHaveAttribute('aria-invalid', 'true');
  expect(change).toHaveBeenCalledTimes(2);
  fireEvent.change(field,{target:{value:''}});
  expect(change).toHaveBeenLastCalledWith('options', undefined);
});

it.each([{value: {data: {count: 0}}}, {value: [{count: 0}]}])('preserva o JSON salvo ao reabrir com esquema', ({value}) => {
  const schema = {properties: {options: {type: Array.isArray(value) ? 'array' : 'object'}}};
  const change = vi.fn();
  const form = render(<SchemaForm schema={null} values={{options: value}} onChange={change} templateMode />);
  expect(screen.getByRole('textbox', {name: 'options'})).toHaveValue(JSON.stringify(value));
  form.unmount();
  render(<SchemaForm schema={schema} values={{options: value}} onChange={change} templateMode />);
  const field = screen.getByRole('textbox', {name: 'options'});
  expect(field).toHaveValue(JSON.stringify(value));
  fireEvent.change(field, {target: {value: '{"updated":true}'}});
  expect(change).toHaveBeenLastCalledWith('options', {updated: true});
});
