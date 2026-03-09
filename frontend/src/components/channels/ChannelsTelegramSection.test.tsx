import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ChannelsTelegramSection } from './ChannelsTelegramSection';

vi.mock('../pickers/ProfilePicker', () => ({
  ProfilePicker: ({ value, onChange, label }: any) => (
    <div>
      <label htmlFor="profile-picker">{label}</label>
      <select
        id="profile-picker"
        value={value}
        onChange={(e) => onChange(e.target.value)}
      >
        <option value="">Nenhum</option>
        <option value="default">Default</option>
      </select>
    </div>
  ),
}));

describe('ChannelsTelegramSection', () => {
  const mockOnChange = vi.fn();
  const mockOnAnnounce = vi.fn();
  const mockOnToggleVault = vi.fn();
  const mockOnRemoveCredential = vi.fn();

  const defaultForm = {
    enabled: false,
    botToken: '',
    profile: '',
    maxHistory: 50,
    maxContacts: 10,
  };

  it('renderiza checkbox de habilitado', () => {
    render(
      <ChannelsTelegramSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    expect(screen.getByLabelText('Habilitado')).toBeInTheDocument();
  });

  it('não mostra campos quando desabilitado', () => {
    render(
      <ChannelsTelegramSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    expect(screen.queryByLabelText('Token do Bot')).not.toBeInTheDocument();
  });

  it('mostra campos quando habilitado', () => {
    render(
      <ChannelsTelegramSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    expect(screen.getByText('Bot Token')).toBeInTheDocument();
    expect(screen.getByText('Máximo de Contatos')).toBeInTheDocument();
    expect(screen.getByText('Máximo de Histórico')).toBeInTheDocument();
  });

  it('chama onChange ao habilitar/desabilitar', async () => {
    const user = userEvent.setup();
    render(
      <ChannelsTelegramSection
        form={defaultForm}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    const checkbox = screen.getByLabelText('Habilitado');
    await user.click(checkbox);

    expect(mockOnChange).toHaveBeenCalledWith({
      ...defaultForm,
      enabled: true,
    });
  });

  it('chama onChange ao alterar botToken', async () => {
    const user = userEvent.setup();
    render(
      <ChannelsTelegramSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    const input = screen.getByPlaceholderText('123456789:ABCDEFG_xyz');
    await user.type(input, '123456');

    expect(mockOnChange).toHaveBeenCalledWith(
      expect.objectContaining({
        botToken: expect.any(String),
      })
    );
  });

  it('mostra opção de salvar no cofre quando habilitado', () => {
    render(
      <ChannelsTelegramSection
        form={{ ...defaultForm, enabled: true }}
        onChange={mockOnChange}
        onAnnounce={mockOnAnnounce}
        vaultEnabled={true}
        onToggleVault={mockOnToggleVault}
        credentialStored={false}
        credentialMasked=""
        onRemoveCredential={mockOnRemoveCredential}
      />
    );

    expect(screen.getByLabelText('Salvar token no cofre de credenciais')).toBeInTheDocument();
  });
});
