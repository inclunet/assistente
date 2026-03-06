import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ProfileGeneralSection } from './ProfileGeneralSection';

describe('ProfileGeneralSection', () => {
  it('renderiza campos básicos', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test Profile"
        description="Test description"
        icon="chatbox"
        onChange={handleChange}
      />
    );

    expect(screen.getByTestId('input-name')).toHaveValue('Test Profile');
    expect(screen.getByTestId('input-description')).toHaveValue('Test description');
    expect(screen.getByTestId('input-icon')).toHaveValue('chatbox');
  });

  it('renderiza com valores vazios para description e icon', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test Profile"
        onChange={handleChange}
      />
    );

    expect(screen.getByTestId('input-name')).toHaveValue('Test Profile');
    expect(screen.getByTestId('input-description')).toHaveValue('');
    expect(screen.getByTestId('input-icon')).toHaveValue('');
  });

  it('chama onChange ao editar nome', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-name');
    await user.type(input, 'X');

    // Verifica que onChange foi chamado
    expect(handleChange).toHaveBeenCalledWith('name', 'TestX');
  });

  it('chama onChange ao editar description', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        description=""
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-description');
    await user.type(input, 'New desc');

    expect(handleChange).toHaveBeenCalledWith('description', expect.any(String));
  });

  it('chama onChange ao editar icon', async () => {
    const user = userEvent.setup();
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        icon=""
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-icon');
    await user.type(input, 'star');

    expect(handleChange).toHaveBeenCalledWith('icon', expect.any(String));
  });

  it('desabilita campos quando disabled = true', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
        disabled={true}
      />
    );

    expect(screen.getByTestId('input-name')).toBeDisabled();
    expect(screen.getByTestId('input-description')).toBeDisabled();
    expect(screen.getByTestId('input-icon')).toBeDisabled();
  });

  it('campo name possui atributo required', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-name');
    expect(input).toHaveAttribute('required');
    expect(input).toHaveAttribute('aria-required', 'true');
  });

  it('campo icon possui placeholder', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-icon');
    expect(input).toHaveAttribute('placeholder', 'chatbox');
  });

  it('renderiza labels corretos', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
      />
    );

    expect(screen.getByLabelText('Nome')).toBeInTheDocument();
    expect(screen.getByLabelText('Descrição')).toBeInTheDocument();
    expect(screen.getByLabelText('Ícone (Ionicons)')).toBeInTheDocument();
  });

  it('usa fireEvent diretamente para onChange (teste alternativo)', () => {
    const handleChange = vi.fn();
    render(
      <ProfileGeneralSection
        name="Test"
        onChange={handleChange}
      />
    );

    const input = screen.getByTestId('input-name');
    fireEvent.change(input, { target: { value: 'Changed' } });

    expect(handleChange).toHaveBeenCalledWith('name', 'Changed');
  });
});
