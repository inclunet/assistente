import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import {
  ChannelEnabledFields,
  ChannelLimitsProfileFields,
  ChannelVaultFields,
} from './ChannelCommonFields';

vi.mock('../pickers/ProfilePicker', () => ({
  ProfilePicker: ({
    value,
    onChange,
    label,
  }: {
    value: string;
    onChange: (value: string) => void;
    label: string;
  }) => (
    <div>
      <label htmlFor="profile-picker">{label}</label>
      <select id="profile-picker" value={value} onChange={(e) => onChange(e.target.value)}>
        <option value="">Nenhum</option>
        <option value="default">Default</option>
      </select>
    </div>
  ),
}));

describe('ChannelCommonFields', () => {
  const defaultForm = {
    enabled: true,
    profile: '',
    maxHistory: 50,
    maxContacts: 10,
  };

  it('renderiza conteúdo apenas quando o canal está habilitado', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <ChannelEnabledFields
        form={{ ...defaultForm, enabled: false }}
        onChange={onChange}
        enabledLabel="Habilitado"
      >
        <span>Campos do canal</span>
      </ChannelEnabledFields>
    );

    expect(screen.getByLabelText('Habilitado')).toBeInTheDocument();
    expect(screen.queryByText('Campos do canal')).not.toBeInTheDocument();

    await user.click(screen.getByLabelText('Habilitado'));

    expect(onChange).toHaveBeenCalledWith({
      ...defaultForm,
      enabled: true,
    });
  });

  it('mantém controles de cofre acessíveis e aciona remoção', async () => {
    const user = userEvent.setup();
    const onToggle = vi.fn();
    const onRemove = vi.fn();

    render(
      <ChannelVaultFields
        label="Salvar token no cofre"
        checked={true}
        onToggle={onToggle}
        hint="Token criptografado no cofre."
        credentials={[
          {
            stored: true,
            masked: '****123',
            storedLabel: 'Token salvo no cofre',
            removeLabel: 'Remover do cofre',
            onRemove,
          },
        ]}
      />
    );

    await user.click(screen.getByLabelText('Salvar token no cofre'));
    await user.click(screen.getByRole('button', { name: 'Remover do cofre' }));

    expect(screen.getByText('Token criptografado no cofre.')).toBeInTheDocument();
    expect(screen.getByText(/Token salvo no cofre/)).toBeInTheDocument();
    expect(onToggle).toHaveBeenCalledWith(false);
    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it('atualiza limites e perfil preservando o restante do formulário', async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();

    render(
      <ChannelLimitsProfileFields
        form={defaultForm}
        onChange={onChange}
        onAnnounce={vi.fn()}
        labels={{
          maxContacts: 'Max. contatos autorizados',
          maxContactsHint: 'Novos contatos são ignorados no limite.',
          channelProfile: 'Perfil do Canal',
          channelProfileHint: 'Perfil usado nas conversas deste canal.',
          maxHistory: 'Máximo de Histórico',
        }}
      />
    );

    fireEvent.change(screen.getByLabelText('Max. contatos autorizados'), {
      target: { value: '20' },
    });
    await user.selectOptions(screen.getByLabelText('Perfil do Canal'), 'default');

    expect(screen.getByText('Novos contatos são ignorados no limite.')).toBeInTheDocument();
    expect(screen.getByText('Perfil usado nas conversas deste canal.')).toBeInTheDocument();
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ maxContacts: 20 }));
    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ profile: 'default' }));
  });
});
