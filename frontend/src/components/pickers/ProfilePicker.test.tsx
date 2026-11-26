import React from 'react';
import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { ProfilePicker } from './ProfilePicker';

const getConversationProfilesSpy = vi.fn();
const getProfilesSpy = vi.fn();
const getActiveSpy = vi.fn();
const setActiveSpy = vi.fn();

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetConversationProfiles: () => getConversationProfilesSpy(),
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
    triggerRef?: React.Ref<HTMLButtonElement>;
    onOpen?: () => void;
    onSelect: (value: string) => void;
    error?: string;
    errorState?: React.ReactNode;
    emptyState?: React.ReactNode;
    loading?: boolean;
    loadingState?: React.ReactNode;
    label?: string;
  }) => props.error ? props.errorState : props.loading ? props.loadingState : (
    <div>
      {props.onOpen && <button ref={props.triggerRef} onClick={props.onOpen}>Abrir</button>}
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
  it('seleciona um perfil instalado somente para a conversa', async () => {
    getConversationProfilesSpy.mockResolvedValueOnce([{ name: 'Writer', slug: 'plugin--12345678-1234-4234-8234-123456789abc', source: 'plugin' }]);
    const change = vi.fn();
    render(<ProfilePicker includePluginProfiles value="" onChange={change} />);
    fireEvent.click(await screen.findByRole('button', {name: 'Selecionar-plugin--12345678-1234-4234-8234-123456789abc'}));
    expect(change).toHaveBeenCalledWith('plugin--12345678-1234-4234-8234-123456789abc');
    expect(setActiveSpy).not.toHaveBeenCalled();
  });

  it('permite tentar novamente após falha de ambas as fontes ao reabrir', async () => {
    const installed = [{name:'Writer', slug:'installed'}];
    getConversationProfilesSpy.mockResolvedValueOnce(installed).mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(installed);
    getProfilesSpy.mockRejectedValueOnce(new Error('offline'));
    render(<ProfilePicker includePluginProfiles value="" />);
    await screen.findByRole('button', {name:'Selecionar-installed'});
    fireEvent.click(screen.getByRole('button', {name:'Abrir'}));
    const retry = await screen.findByRole('button', {name:'Tentar novamente'});
    retry.focus();
    fireEvent.click(retry);
    expect(await screen.findByRole('button', {name:'Selecionar-installed'})).toBeVisible();
    expect(screen.getByRole('button', {name:'Abrir'})).toHaveFocus();
  });

  it('mantém os perfis locais e recupera a lista de plugins ao reabrir', async () => {
    getConversationProfilesSpy.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce([{name:'Writer', slug:'installed'}]);
    getProfilesSpy.mockResolvedValueOnce([{name:'Local', slug:'local'}]);
    const change = vi.fn();
    render(<ProfilePicker includePluginProfiles value="" onChange={change} />);
    fireEvent.click(await screen.findByRole('button', {name:'Selecionar-local'}));
    expect(change).toHaveBeenCalledWith('local');
    expect(screen.getByText('profiles.pluginCatalogUnavailable')).toBeVisible();
    fireEvent.click(screen.getByRole('button', {name:'Abrir'}));
    expect(await screen.findByRole('button', {name:'Selecionar-installed'})).toBeVisible();
    expect(screen.queryByText('profiles.pluginCatalogUnavailable')).toBeNull();
  });

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

  it('usa label padrão i18n e empty state traduzido', async () => {
    getProfilesSpy.mockResolvedValueOnce([]);
    getActiveSpy.mockResolvedValueOnce('padrao');

    render(<ProfilePicker />);

    await waitFor(() => {
      expect(screen.getByTestId('picker-label')).toHaveTextContent('Perfil');
    });

    expect(screen.getByText('Nenhum perfil')).toBeInTheDocument();
  });
});
