import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { AllowlistGeneralSection } from './AllowlistGeneralSection';
import { allowlist } from '../../../wailsjs/go/models';

const buildItem = (overrides: Partial<allowlist.Allowlist> = {}): allowlist.Allowlist => new allowlist.Allowlist({
  name: '',
  description: '',
  auto_approve: [],
  always_deny: [],
  default_action: 'confirm',
  ...overrides,
});

describe('AllowlistGeneralSection', () => {
  it('renderiza campos com valores informados', () => {
    const onFieldChange = vi.fn();
    render(
      <AllowlistGeneralSection
        item={buildItem({
          name: 'Minha allowlist',
          description: 'Permissões padrão',
          default_action: 'deny',
        })}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Nome')).toHaveValue('Minha allowlist');
    expect(screen.getByLabelText('Descrição')).toHaveValue('Permissões padrão');
    expect(screen.getByLabelText('Ação Padrão')).toHaveValue('deny');
  });

  it('usa confirm como valor padrão quando default_action está vazio', () => {
    const onFieldChange = vi.fn();
    render(
      <AllowlistGeneralSection
        item={buildItem({ name: 'Sem ação' })}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Ação Padrão')).toHaveValue('confirm');
  });

  it('dispara onFieldChange ao editar nome e descrição', async () => {
    const onFieldChange = vi.fn();
    render(
      <AllowlistGeneralSection
        item={buildItem({ name: '', description: '' })}
        onFieldChange={onFieldChange}
      />
    );

    fireEvent.change(screen.getByLabelText('Nome'), { target: { value: 'Nova' } });
    fireEvent.change(screen.getByLabelText('Descrição'), { target: { value: 'Desc' } });

    expect(onFieldChange).toHaveBeenCalledWith('name', 'Nova');
    expect(onFieldChange).toHaveBeenCalledWith('description', 'Desc');
  });

  it('dispara onFieldChange ao trocar ação padrão', async () => {
    const user = userEvent.setup();
    const onFieldChange = vi.fn();
    render(
      <AllowlistGeneralSection
        item={buildItem({ default_action: 'confirm' })}
        onFieldChange={onFieldChange}
      />
    );

    await user.selectOptions(screen.getByLabelText('Ação Padrão'), 'deny');

    expect(onFieldChange).toHaveBeenCalledWith('default_action', 'deny');
  });

  it('exibe placeholders esperados', () => {
    const onFieldChange = vi.fn();
    render(
      <AllowlistGeneralSection
        item={buildItem()}
        onFieldChange={onFieldChange}
      />
    );

    expect(screen.getByLabelText('Nome')).toHaveAttribute('placeholder', 'Nome da allowlist');
    expect(screen.getByLabelText('Descrição')).toHaveAttribute(
      'placeholder',
      'Para que serve esta allowlist'
    );
  });
});
