import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { ACPAgentCatalog, type Catalog, type CatalogAgent } from './ACPAgentCatalog';

const announceMock = vi.hoisted(() => vi.fn());
const bumpMock = vi.hoisted(() => vi.fn());
const getCatalogMock = vi.hoisted(() => vi.fn());
const refreshCatalogMock = vi.hoisted(() => vi.fn());

/**
 * Resolve a chave no locale de verdade em vez de devolver a própria chave: o
 * critério de aceitação é que a frase exista nos três locales, e um `t` que
 * ecoa a chave passaria mesmo com o locale vazio.
 */
function resolveLocaleString(key: string, vars?: Record<string, unknown>): string | undefined {
  const root = (ptBR as { translation: Record<string, unknown> }).translation;
  const value = key.split('.').reduce<unknown>((acc, part) => {
    if (!acc || typeof acc !== 'object') return undefined;
    return (acc as Record<string, unknown>)[part];
  }, root);

  if (typeof value !== 'string') return undefined;
  if (!vars) return value;
  return value.replace(/\{\{\s*(\w+)\s*\}\}/g, (_match, varName: string) => {
    const v = vars[varName];
    return v == null ? '' : String(v);
  });
}

vi.mock('react-i18next', async (importOriginal) => {
  const actual = await importOriginal<typeof import('react-i18next')>();
  return {
    ...actual,
    useTranslation: () => ({
      t: (key: string, options?: string | Record<string, unknown>) => {
        const vars = options && typeof options === 'object' ? (options as Record<string, unknown>) : undefined;
        return resolveLocaleString(key, vars) ?? key;
      },
      i18n: { language: 'pt-BR' },
    }),
  };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: bumpMock,
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetACPCatalog: getCatalogMock,
  RefreshACPCatalog: refreshCatalogMock,
}));

const agente = (over: Partial<CatalogAgent> & Pick<CatalogAgent, 'id' | 'name'>): CatalogAgent => ({
  state: 'not_installed',
  distributions: ['npx'],
  runtime: 'node',
  runtime_found: true,
  integrity: '',
  ...over,
});

/** Carimbo de coleta a tantos segundos atrás, como o backend o manda. */
const coletadoHa = (segundos: number): string =>
  new Date(Date.now() - segundos * 1000).toISOString();

const catalogo = (over: Partial<Catalog> = {}): Catalog => ({
  version: '1.0.0',
  agents: [],
  fetched_at: coletadoHa(60),
  age_seconds: 60,
  from_cache: false,
  stale: false,
  platform: 'windows-x64',
  ...over,
});

const tresAgentes = [
  agente({
    id: 'claude-code',
    name: 'Claude Code',
    version: '2.0.0',
    description: 'Agente de código da Anthropic.',
    authors: ['Anthropic'],
    license: 'MIT',
    state: 'installed',
    detected_version: '2.0.1',
  }),
  agente({
    id: 'gemini-cli',
    name: 'Gemini CLI',
    version: '0.4.0',
    description: 'Agente de código do Google.',
    authors: ['Google'],
    license: 'Apache-2.0',
    state: 'no_detection',
  }),
  agente({
    id: 'zed-industries/zeta',
    name: 'Zeta',
    version: '0.1.0',
    description: 'Agente experimental.',
    distributions: ['binary'],
    runtime: '',
    runtime_found: false,
    integrity: 'no_digest',
    state: 'not_installed',
  }),
];

/** Os itens da lista, na ordem em que estão na tela. */
const itens = () => screen.getAllByRole('listitem');

describe('ACPAgentCatalog', () => {
  beforeEach(() => {
    announceMock.mockClear();
    bumpMock.mockClear();
    getCatalogMock.mockReset();
    refreshCatalogMock.mockReset();
    getCatalogMock.mockResolvedValue(catalogo({ agents: tresAgentes }));
  });

  it('lista os agentes que o backend entregou, na ordem recebida', async () => {
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));
    expect(itens().map((li) => li.getAttribute('aria-label')?.split(',')[0])).toEqual([
      'Claude Code',
      'Gemini CLI',
      'Zeta',
    ]);
  });

  it('dá a cada item um nome acessível com nome, versão, estado, runtime, autoria e licença', async () => {
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));
    const label = itens()[0].getAttribute('aria-label') ?? '';
    expect(label).toContain('Claude Code, versão 2.0.0');
    expect(label).toContain('Agente de código da Anthropic.');
    expect(label).toContain('encontrado nesta máquina, versão 2.0.1');
    expect(label).toContain('Requer Node.js');
    expect(label).toContain('pacote npm (npx)');
    expect(label).toContain('Autoria de Anthropic');
    expect(label).toContain('Licença MIT');
    // A chave crua nunca chega ao nome acessível: se chegasse, o locale estaria
    // incompleto e o leitor de telas leria "acpCatalog.algo".
    expect(label).not.toMatch(/acpCatalog\./);
  });

  it('percorre a lista com as setas, Home e End, mantendo uma parada de Tab só', async () => {
    const user = userEvent.setup();
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));

    expect(itens().map((li) => li.getAttribute('tabindex'))).toEqual(['0', '-1', '-1']);

    act(() => itens()[0].focus());
    await user.keyboard('{ArrowDown}');
    expect(itens()[1]).toHaveFocus();
    expect(itens().map((li) => li.getAttribute('tabindex'))).toEqual(['-1', '0', '-1']);

    await user.keyboard('{End}');
    expect(itens()[2]).toHaveFocus();

    await user.keyboard('{Home}');
    expect(itens()[0]).toHaveFocus();

    // Na ponta da lista não há para onde ir: o som avisa sem gastar o anúncio.
    await user.keyboard('{ArrowUp}');
    expect(itens()[0]).toHaveFocus();
    expect(bumpMock).toHaveBeenCalled();
  });

  it('filtra por nome, identificador, autoria e licença', async () => {
    const user = userEvent.setup();
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));

    const busca = screen.getByLabelText('Buscar agente');

    await user.type(busca, 'gemini');
    await waitFor(() => expect(itens()).toHaveLength(1));
    expect(itens()[0].getAttribute('aria-label')).toContain('Gemini CLI');

    await user.clear(busca);
    await user.type(busca, 'zed-industries');
    await waitFor(() => expect(itens()).toHaveLength(1));
    expect(itens()[0].getAttribute('aria-label')).toContain('Zeta');

    await user.clear(busca);
    await user.type(busca, 'anthropic');
    await waitFor(() => expect(itens()).toHaveLength(1));
    expect(itens()[0].getAttribute('aria-label')).toContain('Claude Code');

    await user.clear(busca);
    await user.type(busca, 'Apache-2.0');
    await waitFor(() => expect(itens()).toHaveLength(1));
    expect(itens()[0].getAttribute('aria-label')).toContain('Gemini CLI');
  });

  it('explica quando a busca não acha nada, sem esvaziar a tela em silêncio', async () => {
    const user = userEvent.setup();
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));

    await user.type(screen.getByLabelText('Buscar agente'), 'kubernetes');
    expect(await screen.findByText('Nenhum agente corresponde a "kubernetes".')).toBeInTheDocument();
    expect(screen.queryByRole('list')).not.toBeInTheDocument();
  });

  it('anuncia o número de resultados só depois de a digitação parar', async () => {
    const user = userEvent.setup();
    render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));

    announceMock.mockClear();
    await user.type(screen.getByLabelText('Buscar agente'), 'agente');
    // A cada tecla o anúncio atropelaria a própria leitura do campo de busca.
    expect(announceMock).not.toHaveBeenCalled();

    await waitFor(() => expect(announceMock).toHaveBeenCalledWith('3 agentes encontrados.', 'polite'), {
      timeout: 3000,
    });
    expect(announceMock).toHaveBeenCalledTimes(1);
  });

  describe('estado do catálogo', () => {
    it('diz por que está vazio e aponta o caminho manual', async () => {
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: [],
          fetched_at: '',
          age_seconds: 0,
          reason_code: 'unreachable',
          reason_detail: 'dial tcp: lookup registry.agentclientprotocol.com: no such host',
        })
      );
      render(<ACPAgentCatalog />);

      expect(
        await screen.findByText(
          /O catálogo está vazio: não foi possível falar com o registro \(dial tcp/
        )
      ).toBeInTheDocument();
      expect(
        screen.getByText(/crie um provedor do tipo agente e informe o comando e os argumentos dele/)
      ).toBeInTheDocument();
      expect(screen.queryByRole('list')).not.toBeInTheDocument();
    });

    it('não chama de falha de rede o registro que respondeu com erro', async () => {
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: [],
          fetched_at: '',
          age_seconds: 0,
          reason_code: 'bad_status',
          reason_detail: 'HTTP 503',
        })
      );
      render(<ACPAgentCatalog />);

      const status = await screen.findByText(/O catálogo está vazio/);
      expect(status).toHaveTextContent('o registro respondeu com erro (HTTP 503)');
      // Mandar conferir a rede seria mandar procurar no lugar errado: a conversa
      // com o registro aconteceu.
      expect(status).not.toHaveTextContent('não foi possível falar com o registro');
    });

    it('mostra quando foi coletado e avisa que a cópia local está velha', async () => {
      const tresDias = 3 * 24 * 60 * 60;
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: tresAgentes,
          from_cache: true,
          stale: true,
          fetched_at: coletadoHa(tresDias),
          age_seconds: tresDias,
          reason_code: 'timeout',
        })
      );
      render(<ACPAgentCatalog />);

      const status = await screen.findByText(/Catálogo coletado/);
      expect(status).toHaveTextContent('há 3 dias');
      expect(status).toHaveTextContent('A cópia local está velha');
      expect(status).toHaveTextContent('o registro não respondeu no tempo esperado');
      // A lista continua na tela: o cache velho é melhor do que tela vazia (D2).
      expect(itens()).toHaveLength(3);
    });

    it('conta a idade a partir do carimbo da coleta, não da idade da resposta', async () => {
      // `age_seconds` envelhece junto com a tela aberta; o carimbo, não. Aqui os
      // dois discordam de propósito: quem manda é `fetched_at`.
      getCatalogMock.mockResolvedValue(
        catalogo({ agents: tresAgentes, fetched_at: coletadoHa(3 * 60 * 60), age_seconds: 60 })
      );
      render(<ACPAgentCatalog />);

      const status = await screen.findByText(/Catálogo coletado/);
      expect(status).toHaveTextContent('há 3 horas');
    });

    it('pede uma nova coleta ao backend quando se atualiza o catálogo', async () => {
      const user = userEvent.setup();
      refreshCatalogMock.mockResolvedValue(
        catalogo({ agents: [tresAgentes[0]], age_seconds: 0 })
      );
      render(<ACPAgentCatalog />);
      await waitFor(() => expect(itens()).toHaveLength(3));

      await user.click(screen.getByRole('button', { name: 'Atualizar catálogo' }));
      await waitFor(() => expect(itens()).toHaveLength(1));
      expect(refreshCatalogMock).toHaveBeenCalledTimes(1);
    });

    it('anuncia a falha de carga e não deixa a tela sem explicação', async () => {
      getCatalogMock.mockRejectedValue(new Error('serviço do registro não montado'));
      render(<ACPAgentCatalog />);

      expect(await screen.findByText(/serviço do registro não montado/)).toBeInTheDocument();
      expect(announceMock).toHaveBeenCalledWith(
        expect.stringContaining('serviço do registro não montado'),
        'assertive'
      );
    });
  });

  describe('estado de cada agente nesta máquina', () => {
    const comEstado = async (over: Partial<CatalogAgent>) => {
      getCatalogMock.mockResolvedValue(
        catalogo({ agents: [agente({ id: 'a', name: 'Agente', ...over })] })
      );
      render(<ACPAgentCatalog />);
      await waitFor(() => expect(itens()).toHaveLength(1));
      return itens()[0];
    };

    it('diz que encontrou, com a versão que a detecção leu', async () => {
      const item = await comEstado({ state: 'installed', detected_version: '1.2.3' });
      expect(within(item).getByText(/encontrado nesta máquina, versão 1\.2\.3/)).toBeInTheDocument();
    });

    it('diz que não encontrou quando o app sabe procurar e não achou', async () => {
      const item = await comEstado({ state: 'not_installed' });
      expect(within(item).getByText(/não encontrado nesta máquina/)).toBeInTheDocument();
    });

    it('não finge procura para o agente que o app não sabe detectar', async () => {
      const item = await comEstado({ state: 'no_detection' });
      expect(
        within(item).getByText(/este app não sabe procurar este agente na máquina/)
      ).toBeInTheDocument();
      expect(within(item).queryByText(/não encontrado nesta máquina/)).not.toBeInTheDocument();
    });

    it('nomeia o runtime que falta em vez de só dizer que não dá', async () => {
      const item = await comEstado({
        state: 'requirement_missing',
        runtime: 'uv',
        runtime_found: false,
      });
      expect(
        within(item).getByText(/uv não foi encontrado nesta máquina, e este agente precisa dele/)
      ).toBeInTheDocument();
      expect(within(item).getByText(/Requer uv, que não foi encontrado/)).toBeInTheDocument();
    });

    it('diz quando não há artefato publicado para esta plataforma', async () => {
      const item = await comEstado({
        state: 'no_platform_target',
        distributions: ['binary'],
        runtime: '',
        integrity: 'no_platform_target',
      });
      expect(within(item).getByText(/não há versão publicada para esta plataforma/)).toBeInTheDocument();
    });

    it('conta que a procura falhou, com o detalhe do erro', async () => {
      const item = await comEstado({
        state: 'detection_failed',
        state_detail: 'permission denied',
      });
      expect(within(item).getByText(/a procura por este agente falhou/)).toBeInTheDocument();
      expect(within(item).getByText('permission denied')).toBeInTheDocument();
    });

    it('marca o catálogo como ocupado enquanto ele carrega (D13)', async () => {
      // `aria-busy` no lugar de região viva: o anúncio é do announcer global
      // (AEP-0058), e o que a marca diz é que a lista está trocando de conteúdo
      // debaixo de quem a estiver percorrendo.
      let servir: (catalog: Catalog) => void = () => {};
      getCatalogMock.mockReturnValue(
        new Promise<Catalog>((resolve) => {
          servir = resolve;
        })
      );
      const { container } = render(<ACPAgentCatalog />);

      const bloco = container.querySelector('.acp-catalog') as HTMLElement;
      expect(bloco).toHaveAttribute('aria-busy', 'true');

      await act(async () => {
        servir(catalogo({ agents: [agente({ id: 'a', name: 'Agente' })] }));
      });

      await waitFor(() => expect(bloco).toHaveAttribute('aria-busy', 'false'));
    });

    it('diz que foi este app quem instalou, com a versão que ele pôs no disco', async () => {
      const item = await comEstado({
        state: 'installed',
        installed_by_app: true,
        installed_version: '1.0.0',
        state_detail: '/home/ana/.assistente/agents/codex-acp/1.0.0',
      });
      // A frase de estado é lida inteira porque é ela que muda: "encontrado"
      // descreveria uma procura, e quem pôs o agente ali foi o app.
      const estado = item.querySelector('.acp-catalog__state')?.textContent ?? '';
      expect(estado).toContain('instalado por este app, versão 1.0.0');
      expect(estado).not.toContain('encontrado nesta máquina');
      expect(within(item).getByText('/home/ana/.assistente/agents/codex-acp/1.0.0')).toBeInTheDocument();
    });

    it('continua marcando a instalação que não pôde ser conferida (D4)', async () => {
      const item = await comEstado({
        state: 'installed',
        installed_by_app: true,
        installed_version: '2026.01.02',
        installed_unverified: true,
        distributions: ['binary'],
        runtime: '',
        integrity: 'no_digest',
      });
      expect(within(item).getByText(/Esta instalação não foi verificada/)).toBeInTheDocument();
      // A marca também está no nome acessível: quem navega com leitor de telas
      // ouve o item inteiro e não passa por essa ressalva por acaso.
      expect(item.getAttribute('aria-label') ?? '').toContain('Esta instalação não foi verificada');
    });

    it('não marca como não verificada a instalação cujo digest bateu', async () => {
      const item = await comEstado({
        state: 'installed',
        installed_by_app: true,
        installed_version: '2.0.0',
        distributions: ['binary'],
        runtime: '',
        integrity: 'digest',
      });
      expect(within(item).queryByText(/Esta instalação não foi verificada/)).not.toBeInTheDocument();
    });

    it('avisa que o agente não publica soma de verificação nesta plataforma (D4)', async () => {
      const item = await comEstado({
        distributions: ['binary'],
        runtime: '',
        integrity: 'no_digest',
      });
      expect(
        within(item).getByText(/O registro não publica soma de verificação para esta plataforma/)
      ).toBeInTheDocument();
    });

    it('diz o caminho onde o runtime foi achado, que é o que resolve duas versões na máquina', async () => {
      const item = await comEstado({
        runtime: 'node',
        runtime_found: true,
        runtime_path: 'C:\\Program Files\\nodejs\\node.exe',
      });
      expect(
        within(item).getByText(/Requer Node\.js, encontrado em C:\\Program Files\\nodejs\\node\.exe/)
      ).toBeInTheDocument();
    });

    it('diz que o binário não exige runtime nenhum', async () => {
      const item = await comEstado({ distributions: ['binary'], runtime: '' });
      expect(
        within(item).getByText(/Não exige runtime: o agente é distribuído como binário/)
      ).toBeInTheDocument();
    });
  });

  it('não tem violação de acessibilidade com catálogo cheio', async () => {
    const { container } = render(<ACPAgentCatalog />);
    await waitFor(() => expect(itens()).toHaveLength(3));
    expect(await axe(container)).toHaveNoViolations();
  });

  it('não tem violação de acessibilidade com catálogo vazio e motivo', async () => {
    getCatalogMock.mockResolvedValue(
      catalogo({ agents: [], fetched_at: '', age_seconds: 0, reason_code: 'unreachable' })
    );
    const { container } = render(<ACPAgentCatalog />);
    await screen.findByText(/O catálogo está vazio/);
    expect(await axe(container)).toHaveNoViolations();
  });

  describe('browse acionável (tela de provedores)', () => {
    it('oferece Usar e Instalar sem transformar a lista em picker', async () => {
      const onUse = vi.fn();
      const onInstall = vi.fn();
      render(<ACPAgentCatalog onUseAgent={onUse} onInstallAgent={onInstall} />);
      await waitFor(() => expect(itens()).toHaveLength(3));

      expect(screen.getByText(/Em cada linha você pode instalar/)).toBeInTheDocument();
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();

      const usarGemini = await screen.findByRole('button', { name: 'Usar Gemini CLI neste provedor' });
      await userEvent.click(usarGemini);
      expect(onUse).toHaveBeenCalledWith(expect.objectContaining({ id: 'gemini-cli', name: 'Gemini CLI' }));

      const instalarZeta = screen.getByRole('button', { name: 'Instalar Zeta' });
      await userEvent.click(instalarZeta);
      expect(onInstall).toHaveBeenCalledWith(expect.objectContaining({ id: 'zed-industries/zeta' }));
    });

    it('não oferece Instalar quando o runtime falta ou não há alvo nesta plataforma', async () => {
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: [
            agente({
              id: 'sem-node',
              name: 'Sem Node',
              state: 'requirement_missing',
              runtime: 'node',
              runtime_found: false,
            }),
            agente({
              id: 'sem-alvo',
              name: 'Sem Alvo',
              state: 'no_platform_target',
              distributions: ['binary'],
              runtime: '',
            }),
            agente({ id: 'ok', name: 'Ok', state: 'not_installed' }),
          ],
        })
      );
      render(<ACPAgentCatalog onUseAgent={() => {}} onInstallAgent={() => {}} />);
      await screen.findByRole('button', { name: 'Usar Ok neste provedor' });

      expect(screen.queryByRole('button', { name: 'Instalar Sem Node' })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Instalar Sem Alvo' })).not.toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Instalar Ok' })).toBeInTheDocument();
      // Usar continua disponível: o formulário ainda serve para apontar à mão.
      expect(screen.getByRole('button', { name: 'Usar Sem Node neste provedor' })).toBeInTheDocument();
    });

    it('não oferece Instalar de novo quando o app já instalou o agente', async () => {
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: [
            agente({
              id: 'cursor',
              name: 'Cursor',
              state: 'installed',
              installed_by_app: true,
              installed_version: '1.0.0',
              distributions: ['binary'],
              runtime: '',
            }),
          ],
        })
      );
      render(<ACPAgentCatalog onUseAgent={() => {}} onInstallAgent={() => {}} />);
      await screen.findByRole('button', { name: 'Usar Cursor neste provedor' });
      expect(screen.queryByRole('button', { name: 'Instalar Cursor' })).not.toBeInTheDocument();
    });

    it('navega com setas também a partir do botão Instalar', async () => {
      const user = userEvent.setup();
      render(<ACPAgentCatalog onUseAgent={() => {}} onInstallAgent={() => {}} />);
      await waitFor(() => expect(itens()).toHaveLength(3));

      const instalarZeta = screen.getByRole('button', { name: 'Instalar Zeta' });
      instalarZeta.focus();
      await user.keyboard('{ArrowUp}');
      expect(screen.getByRole('button', { name: 'Usar Gemini CLI neste provedor' })).toHaveFocus();
    });

    it('foca Instalar quando a linha não tem Usar', async () => {
      getCatalogMock.mockResolvedValue(
        catalogo({
          agents: [
            agente({ id: 'so-instalar', name: 'Só Instalar', state: 'not_installed' }),
            agente({ id: 'outro', name: 'Outro', state: 'not_installed' }),
          ],
        })
      );
      const user = userEvent.setup();
      render(<ACPAgentCatalog onInstallAgent={() => {}} />);
      await screen.findByRole('button', { name: 'Instalar Só Instalar' });

      screen.getByRole('button', { name: 'Instalar Só Instalar' }).focus();
      await user.keyboard('{ArrowDown}');
      expect(screen.getByRole('button', { name: 'Instalar Outro' })).toHaveFocus();
    });

    it('não tem violação de acessibilidade no browse acionável', async () => {
      const { container } = render(
        <ACPAgentCatalog onUseAgent={() => {}} onInstallAgent={() => {}} />
      );
      await waitFor(() => expect(screen.getAllByRole('button', { name: /Usar .+ neste provedor/ })).toHaveLength(3));
      expect(await axe(container)).toHaveNoViolations();
    });
  });
});
