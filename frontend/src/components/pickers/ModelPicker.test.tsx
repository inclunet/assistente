import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ModelPicker } from './ModelPicker';

const getModelsSpy = vi.fn();
const refreshModelsSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: unknown) => (
      options && typeof options === 'object' ? `${key}|${JSON.stringify(options)}` : key
    ),
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetModels: () => getModelsSpy(),
  GetModelsByProvider: (providerId: string) => getModelsSpy(providerId),
  RefreshModels: () => refreshModelsSpy(),
  RefreshModelsByProvider: (providerId: string) => refreshModelsSpy(providerId),
  GetLLMProvidersWithStatus: () => Promise.resolve([]),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: { items: Array<{ value: string }>; allowFreeInput?: boolean; error?: string | null }) => (
    <div data-testid="base-picker" data-items={props.items.length} data-allowfree={props.allowFreeInput ? 'yes' : 'no'} data-error={props.error ?? ''} />
  ),
}));

beforeEach(() => {
  getModelsSpy.mockReset();
  refreshModelsSpy.mockReset();
});

describe('ModelPicker', () => {
  it('carrega modelos por provider', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });
  });

  it('habilita input livre quando endpoint nao suportado', async () => {
    getModelsSpy.mockRejectedValueOnce('models_endpoint_not_supported');

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-allowfree', 'yes');
    });
  });

  // O primeiro carregamento não pode invalidar nada: para um agente de código,
  // invalidar a cada render faria a tela de perfil abrir uma descoberta no
  // processo dele sem ninguém ter pedido (AEP-0084 D6).
  it('a abertura da tela lista sem descartar o que o provedor sabia', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" variant="form" />);

    await waitFor(() => expect(getModelsSpy).toHaveBeenCalledWith('p1'));
    expect(refreshModelsSpy).not.toHaveBeenCalled();
  });

  it('recarregar na tela faz o provedor perguntar de novo', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);
    refreshModelsSpy.mockResolvedValueOnce(['m1', 'm2']);
    const anuncios: string[] = [];

    render(
      <ModelPicker
        value=""
        onChange={() => {}}
        providerID="p1"
        variant="form"
        onAnnounce={(message) => anuncios.push(message)}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2'));

    await userEvent.click(screen.getByRole('button', { name: 'pickers.model.refreshLabel' }));

    await waitFor(() => expect(refreshModelsSpy).toHaveBeenCalledWith('p1'));
    // Um modelo a mais que o do primeiro carregamento, mais o "padrão do provedor".
    await waitFor(() => expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '3'));
    const anunciado = anuncios.join(' ');
    expect(anunciado).toContain('pickers.model.refreshed');
    // O anúncio conta o tamanho da lista, que é fato. Dizer que ela foi buscada
    // de novo seria promessa: um agente de código responde da sessão de
    // descoberta que ele já tem (AEP-0084 D6).
    expect(anunciado).toContain('"count":2');
  });

  // Anunciar "lista recarregada" quando a lista não veio diria a quem usa leitor
  // de telas o contrário do que a tela mostra.
  it('recarregar que falha anuncia o problema, não sucesso', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);
    refreshModelsSpy.mockRejectedValueOnce(new Error('credencial não configurada'));
    const anuncios: string[] = [];

    render(
      <ModelPicker
        value=""
        onChange={() => {}}
        providerID="p1"
        variant="form"
        onAnnounce={(message) => anuncios.push(message)}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2'));

    await userEvent.click(screen.getByRole('button', { name: 'pickers.model.refreshLabel' }));

    await waitFor(() => expect(anuncios.length).toBeGreaterThan(0));
    expect(anuncios).toContain('pickers.model.configureApiKey');
    expect(anuncios.join(' ')).not.toContain('pickers.model.refreshed');
  });

  it('recarregar que volta vazio anuncia que não há modelos', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);
    refreshModelsSpy.mockResolvedValueOnce([]);
    const anuncios: string[] = [];

    render(
      <ModelPicker
        value=""
        onChange={() => {}}
        providerID="p1"
        variant="form"
        onAnnounce={(message) => anuncios.push(message)}
      />,
    );
    await waitFor(() => expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '2'));

    await userEvent.click(screen.getByRole('button', { name: 'pickers.model.refreshLabel' }));

    await waitFor(() => expect(anuncios.length).toBeGreaterThan(0));
    expect(anuncios).toContain('pickers.model.noModels');
    expect(anuncios.join(' ')).not.toContain('pickers.model.refreshed');
  });

  it('a barra de ferramentas não ganha botão de recarregar', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);

    render(<ModelPicker value="" onChange={() => {}} variant="toolbar" />);

    await waitFor(() => expect(getModelsSpy).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: 'pickers.model.refreshLabel' })).toBeNull();
  });
});
