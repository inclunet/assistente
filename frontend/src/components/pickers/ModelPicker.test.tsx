import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ModelPicker } from './ModelPicker';

const getModelsSpy = vi.fn();
const refreshModelsSpy = vi.fn();

/** catalogo monta a resposta do backend a partir de nomes simples. */
const catalogo = (models: Array<string | { value: string; label: string }>, agent = false) => ({
  agent,
  models: models.map(m => (typeof m === 'string' ? { value: m, label: m } : m)),
});

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string, options?: unknown) => (
      options && typeof options === 'object' ? `${key}|${JSON.stringify(options)}` : key
    ),
  }),
}));

vi.mock('@wailsjs/go/wailsapi/LLMModels', () => ({
  GetModels: () => getModelsSpy(),
  GetModelCatalogByProvider: (providerId: string) => getModelsSpy(providerId),
  RefreshModels: () => refreshModelsSpy(),
  RefreshModelCatalogByProvider: (providerId: string) => refreshModelsSpy(providerId),
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: () => Promise.resolve([]),
}));

vi.mock('./BasePicker', () => ({
  BasePicker: (props: {
    items: Array<{ value: string; label: string }>;
    allowFreeInput?: boolean;
    error?: string | null;
    helpText?: string;
    description?: string;
  }) => (
    <div
      data-testid="base-picker"
      data-items={props.items.length}
      data-labels={props.items.map(item => item.label).join('|')}
      data-allowfree={props.allowFreeInput ? 'yes' : 'no'}
      data-error={props.error ?? ''}
      data-help={props.helpText ?? ''}
      data-description={props.description ?? ''}
    />
  ),
}));

beforeEach(() => {
  getModelsSpy.mockReset();
  refreshModelsSpy.mockReset();
});

describe('ModelPicker', () => {
  it('carrega modelos por provider', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-items', '1');
    });
  });

  // O identificador de um modelo de agente é feito para o protocolo, não para
  // ser lido: mostrá-lo cru é o app repassando detalhe interno a quem só queria
  // escolher um modelo (AEP-0084, Fase 8).
  it('mostra o modelo do agente pelo nome que ele deu', async () => {
    getModelsSpy.mockResolvedValueOnce(
      catalogo([{ value: 'grok-4.5[max]', label: 'Grok 4.5 (max)' }], true),
    );

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-labels', 'Grok 4.5 (max)');
    });
  });

  it('modelo sem nome continua sendo exibido pelo identificador', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo([{ value: 'gpt-5', label: '' }], true));

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-labels', 'gpt-5');
    });
  });

  // Agente que não expõe escolha de modelo está funcionando: quem escolhe é
  // ele. Tratar como erro mandaria a pessoa procurar conserto para o normal.
  it('agente sem escolha de modelo não vira erro', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo([], true));

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" variant="form" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-help', 'pickers.model.agentChooses');
    });
    expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', '');
  });

  it('provedor http sem modelo continua sendo falta de modelo', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo([]));

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" variant="form" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', 'pickers.model.noModels');
    });
  });

  // "executable file not found in %PATH%" não conta a quem lê o que fazer. O
  // que fazer é refazer a detecção na tela de provedores.
  it('agente que não sobe manda a pessoa à tela de provedores', async () => {
    getModelsSpy.mockRejectedValueOnce(
      new Error('listar modelos do agente Cursor: acp_agent_unavailable: iniciar agente cursor-agent: executable file not found in %PATH%'),
    );

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" variant="form" />);

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-error', 'pickers.model.agentUnavailable');
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
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));

    render(<ModelPicker value="" onChange={() => {}} providerID="p1" variant="form" />);

    await waitFor(() => expect(getModelsSpy).toHaveBeenCalledWith('p1'));
    expect(refreshModelsSpy).not.toHaveBeenCalled();
  });

  it('recarregar na tela faz o provedor perguntar de novo', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));
    refreshModelsSpy.mockResolvedValueOnce(catalogo(['m1', 'm2']));
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
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));
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

  it('falha sem texto não anuncia recado em português', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));
    // Falha que não traz mensagem alguma: rejeição sem `message`, como acontece
    // quando a ponte devolve um valor vazio. O recado é lido em voz alta e vale
    // nos três idiomas, então só a parte traduzida pode sair.
    refreshModelsSpy.mockRejectedValueOnce(new Error(''));
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
    expect(anuncios).toContain('pickers.model.loadError');
    expect(anuncios.join(' ')).not.toContain('desconhecido');
  });

  it('recarregar que volta vazio anuncia que não há modelos', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));
    refreshModelsSpy.mockResolvedValueOnce(catalogo([]));
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

  it('agente sem escolha de modelo é anunciado como resposta, não como falta', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1'], true));
    refreshModelsSpy.mockResolvedValueOnce(catalogo([], true));
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
    expect(anuncios).toContain('pickers.model.agentChooses');
    expect(anuncios.join(' ')).not.toContain('pickers.model.noModels');
  });

  it('a barra de ferramentas não ganha botão de recarregar', async () => {
    getModelsSpy.mockResolvedValueOnce(['m1']);

    render(<ModelPicker value="" onChange={() => {}} variant="toolbar" />);

    await waitFor(() => expect(getModelsSpy).toHaveBeenCalled());
    expect(screen.queryByRole('button', { name: 'pickers.model.refreshLabel' })).toBeNull();
  });

  it('oferece retorno acessível ao padrão no uso por aba', async () => {
    getModelsSpy.mockResolvedValueOnce(catalogo(['m1']));

    render(
      <ModelPicker
        value="$default"
        onChange={() => {}}
        providerID="p1"
        variant="toolbar"
        includeDefaultOption
        defaultOptionLabel="Modelo do perfil"
        description="Escolhe o modelo desta aba"
      />,
    );

    await waitFor(() => {
      expect(screen.getByTestId('base-picker')).toHaveAttribute('data-labels', 'Modelo do perfil|m1');
    });
    expect(screen.getByTestId('base-picker')).toHaveAttribute(
      'data-description',
      'Escolhe o modelo desta aba',
    );
  });
});
