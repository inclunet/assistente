import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ProfilePicker } from './ProfilePicker';

const getProfilesSpy = vi.fn();
const getActiveSpy = vi.fn();
const setActiveSpy = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetProfiles: () => getProfilesSpy(),
  GetActiveProfileSlug: () => getActiveSpy(),
  SetActiveProfile: (slug: string) => setActiveSpy(slug),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: {
    items: Array<{ value: string }>;
    onSelect: (value: string) => void;
    emptyState?: React.ReactNode;
    loading?: boolean;
    loadingState?: React.ReactNode;
    label?: string;
  }) => (
    <div>
      <span data-testid="picker-label">{props.label}</span>
      {props.loading ? props.loadingState : null}
      {!props.loading && props.items.length === 0 ? props.emptyState : null}
      {props.items.map((item, i) => (
        <button key={item.value || `empty-${i}`} onClick={() => props.onSelect(item.value)}>
          {i === 0 ? 'Selecionar' : `Selecionar-${item.value}`}
        </button>
      ))}
      <div data-testid="base-picker" data-items={props.items.length} />
    </div>
  ),
}));

describe('ProfilePicker', () => {
  it('dispara onChange no modo controlado', async () => {
    getProfilesSpy.mockResolvedValueOnce([
      { name: 'Padrao', slug: 'padrao', description: '', icon: '', source: 'local' },
    ]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    const onChange = vi.fn();

    render(<ProfilePicker value="padrao" onChange={onChange} />);

    await waitFor(() => {
      // 2 items: "usar perfil ativo global" + "padrao"
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });

    // Seleciona o perfil "padrao" (segundo item)
    fireEvent.click(screen.getByRole('button', { name: 'Selecionar-padrao' }));
    expect(onChange).toHaveBeenCalledWith('padrao');
    expect(setActiveSpy).not.toHaveBeenCalled();
  });

  it('dispara onChange com valor vazio ao selecionar perfil global', async () => {
    getProfilesSpy.mockResolvedValueOnce([
      { name: 'Padrao', slug: 'padrao', description: '', icon: '', source: 'local' },
    ]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    const onChange = vi.fn();

    render(<ProfilePicker value="padrao" onChange={onChange} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });

    // Seleciona "usar perfil ativo global" (primeiro item, valor vazio)
    fireEvent.click(screen.getByRole('button', { name: 'Selecionar' }));
    expect(onChange).toHaveBeenCalledWith('');
    expect(setActiveSpy).not.toHaveBeenCalled();
  });

  it('anuncia seleção no modo controlado via i18n', async () => {
    getProfilesSpy.mockResolvedValueOnce([
      { name: 'Padrao', slug: 'padrao', description: '', icon: '', source: 'local' },
    ]);

    const onAnnounce = vi.fn();
    render(<ProfilePicker value="padrao" onChange={vi.fn()} onAnnounce={onAnnounce} />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2');
    });

    fireEvent.click(screen.getByRole('button', { name: 'Selecionar-padrao' }));
    expect(onAnnounce).toHaveBeenCalledWith('Perfil selecionado: Padrao');
  });

  it('usa label padrão i18n e anuncia empty state em live region', async () => {
    getProfilesSpy.mockResolvedValueOnce([]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    render(<ProfilePicker />);

    await waitFor(() => {
      expect(screen.getByTestId('picker-label')).toHaveTextContent('Perfil');
    });

    expect(screen.getByRole('status')).toHaveTextContent('Nenhum perfil');
  });
});
