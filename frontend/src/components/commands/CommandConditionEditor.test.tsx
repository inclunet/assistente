import { axe } from '../../test/a11yAxe';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useState } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from 'i18next';
import CommandConditionEditor from './CommandConditionEditor';
import type { CommandCondition, CommandConditionField } from '../../types/commandSettingsTypes';

const fields: CommandConditionField[] = [
  { id: 'surface', label: 'Surface', valueKind: 'enum', options: [{ value: 'editor', label: 'Editor' }] },
  { id: 'focused', label: 'Focused', valueKind: 'boolean' },
  { id: 'profile', label: 'Profile', valueKind: 'opaque-id' },
];

const condition: CommandCondition = { version: 1, clauses: [] };

function ControlledConditionEditor({ initial, availableFields = fields }: { initial: CommandCondition; availableFields?: CommandConditionField[] }) {
  const [value, setValue] = useState(initial);
  return <CommandConditionEditor value={value} fields={availableFields} onChange={setValue} />;
}

describe('CommandConditionEditor', () => {
  beforeEach(() => {
    i18n.addResource('en', 'translation', 'commandSettings.conditionRowLabel', 'Condition {{index}}: {{field}}');
  });

  it('não mascara uma condição desconhecida como o primeiro campo disponível', () => {
    const onChange = vi.fn();
    render(<CommandConditionEditor value={{ version: 1, clauses: [{ field: 'unavailable', value: 'keep' }] }} fields={fields} onChange={onChange} />);
    expect(screen.getByLabelText('commandSettings.conditions.field')).toHaveValue('unavailable');
    expect(screen.getByLabelText('commandSettings.conditions.value')).toHaveValue('keep');
    expect(screen.getByLabelText('commandSettings.conditions.value')).toBeDisabled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('mostra referência enum ausente sem selecionar outra ou alterar o documento', () => {
    const onChange = vi.fn();
    render(<CommandConditionEditor value={{ version: 1, clauses: [{ field: 'surface', value: 'absent' }] }} fields={fields} onChange={onChange} />);
    expect(screen.getByLabelText('commandSettings.conditions.value')).toHaveValue('absent');
    expect(screen.getByRole('option', { name: 'commandSettings.unavailableConditionValue' })).toBeDisabled();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('edita cláusulas tipadas sem usar JSON livre', () => {
    const onChange = vi.fn();
    render(<CommandConditionEditor value={condition} fields={fields} onChange={onChange} />);

    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.add' }));
    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      clauses: [{ field: 'surface', value: 'editor' }],
    });
    expect(screen.queryByRole('textbox', { name: /JSON/i })).not.toBeInTheDocument();
  });

  it('altera o tipo da cláusula e remove sem mutar a entrada', () => {
    const onChange = vi.fn();
    const value: CommandCondition = { version: 1, clauses: [{ field: 'surface', value: 'editor' }] };
    render(<CommandConditionEditor value={value} fields={fields} onChange={onChange} />);

    fireEvent.change(screen.getByLabelText('commandSettings.conditions.field'), { target: { value: 'focused' } });
    expect(onChange).toHaveBeenCalledWith({
      version: 1,
      clauses: [{ field: 'focused', value: false }],
    });
    fireEvent.click(screen.getByRole('button', { name: 'commandSettings.conditions.remove' }));
    expect(onChange).toHaveBeenLastCalledWith({ version: 1, clauses: [] });
    expect(value).toEqual({ version: 1, clauses: [{ field: 'surface', value: 'editor' }] });
  });

  it('preserva o foco do seletor ao trocar o campo da cláusula', () => {
    const onChange = vi.fn();
    const value: CommandCondition = { version: 1, clauses: [{ field: 'surface', value: 'editor' }] };
    const { rerender } = render(<CommandConditionEditor value={value} fields={fields} onChange={onChange} />);
    const field = screen.getByLabelText('commandSettings.conditions.field');

    field.focus();
    fireEvent.change(field, { target: { value: 'focused' } });
    rerender(<CommandConditionEditor value={{ version: 1, clauses: [{ field: 'focused', value: false }] }} fields={fields} onChange={onChange} />);

    expect(screen.getByLabelText('commandSettings.conditions.field')).toHaveFocus();
  });

  it('contextualiza duas linhas por grupo acessível e não tem violações axe', async () => {
    const { container } = render(<CommandConditionEditor value={{ version: 1, clauses: [
      { field: 'surface', value: 'editor' },
      { field: 'focused', value: false },
    ] }} fields={fields} onChange={vi.fn()} />);

    const groups = Array.from(container.querySelectorAll<HTMLElement>('[role="group"]'));
    expect(groups).toHaveLength(2);
    expect(groups[0]).toHaveAccessibleName('Condition 1: Surface');
    expect(groups[1]).toHaveAccessibleName('Condition 2: Focused');
    expect(await axe(container)).toHaveNoViolations();
  });

  it('ao adicionar por Enter move o foco à cláusula criada; ao remover move ao vizinho', async () => {
    const user = userEvent.setup();
    render(<ControlledConditionEditor initial={condition} />);
    const add = screen.getByRole('button', { name: 'commandSettings.conditions.add' });
    add.focus();
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('commandSettings.conditions.field')).toHaveFocus();
    add.focus();
    await user.keyboard('{Enter}');
    expect(screen.getAllByLabelText('commandSettings.conditions.field')).toHaveLength(2);

    screen.getAllByLabelText('commandSettings.conditions.field')[0].focus();
    await user.keyboard('{Tab}{Tab}');
    await user.keyboard('{Enter}');
    expect(screen.getByLabelText('commandSettings.conditions.field')).toHaveFocus();
    expect(screen.getByLabelText('commandSettings.conditions.field')).toHaveValue('focused');
  });

  it('ao remover a última cláusula por teclado devolve foco ao controle de adicionar', async () => {
    const user = userEvent.setup();
    render(<ControlledConditionEditor initial={{ version: 1, clauses: [{ field: 'surface', value: 'editor' }] }} />);
    screen.getByLabelText('commandSettings.conditions.field').focus();
    await user.keyboard('{Tab}{Tab}{Enter}');
    expect(screen.getByRole('button', { name: 'commandSettings.conditions.add' })).toHaveFocus();
  });

  it('com cláusula legada desconhecida e sem opções, usa o fieldset como fallback de foco', async () => {
    const user = userEvent.setup();
    const { container } = render(<ControlledConditionEditor
      initial={{ version: 1, clauses: [{ field: 'removed-field', value: 'legacy' }] }}
      availableFields={[]}
    />);
    const fieldset = container.querySelector('fieldset.command-condition-editor') as HTMLFieldSetElement;
    screen.getByRole('button', { name: 'commandSettings.conditions.remove' }).focus();
    await user.keyboard('{Enter}');
    expect(fieldset).toHaveFocus();
    expect(screen.getByRole('button', { name: 'commandSettings.conditions.add' })).toBeDisabled();
  });
});
