import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { AgentInstall } from './AgentInstall';

const announceMock = vi.hoisted(() => vi.fn());
const planMock = vi.hoisted(() => vi.fn());
const installMock = vi.hoisted(() => vi.fn());
const cancelMock = vi.hoisted(() => vi.fn());
const removeMock = vi.hoisted(() => vi.fn());
const updateMock = vi.hoisted(() => vi.fn());
const eventsOnMock = vi.hoisted(() => vi.fn());

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
        const resolved = resolveLocaleString(key, vars);
        if (resolved !== undefined) return resolved;
        // O componente pede a etapa com texto padrão vazio: etapa que o locale
        // não conhece não deve virar chave crua na tela.
        return typeof options === 'string' ? options : key;
      },
    }),
  };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  ACPAgentInstallPlan: planMock,
  InstallACPAgent: installMock,
  CancelACPAgentInstall: cancelMock,
  RemoveACPAgent: removeMock,
  UpdateACPAgent: updateMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: eventsOnMock,
}));

/** Progresso que o teste dispara como se viesse do backend. */
type Progresso = {
  agent_id: string;
  agent?: string;
  stage: string;
  step?: string;
  reason?: string;
};

/**
 * Entrega os marcos aos ouvintes registrados, como o Wails faz. Mais de um
 * ouvinte existe porque o React monta o efeito de novo a cada render que muda a
 * dependência dele.
 */
function emitirProgresso(progresso: Progresso) {
  const ouvintes = eventsOnMock.mock.calls
    .filter(([evento]) => evento === 'acp:install:progress')
    .map(([, callback]) => callback as (p: Progresso) => void);
  act(() => {
    for (const ouvinte of ouvintes) ouvinte(progresso);
  });
}

const nodeEncontrado = {
  name: 'Node.js',
  required: true,
  found: true,
  path: 'C:\\Program Files\\nodejs\\node.exe',
  version: '22.14.0',
  searched: [],
};

const planoInstalavel = {
  agent_id: 'codex-acp',
  name: 'Codex',
  version: '1.1.9',
  distribution: 'npm',
  origin: '@agentclientprotocol/codex-acp@1.1.9',
  dir: 'C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.1.9',
  install_command:
    '"C:\\Program Files\\nodejs\\node.exe" "C:\\Program Files\\nodejs\\node_modules\\npm\\bin\\npm-cli.js" install --prefix "C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.1.9" @agentclientprotocol/codex-acp@1.1.9',
  run_args: [],
  runtime: nodeEncontrado,
  can_install: true,
  installing: false,
};

const instalacao = {
  agent_id: 'codex-acp',
  name: 'Codex',
  version: '1.1.9',
  distribution: 'npm',
  target: '@agentclientprotocol/codex-acp@1.1.9',
  command: 'C:\\Program Files\\nodejs\\node.exe',
  args: ['C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.1.9\\node_modules\\@agentclientprotocol\\codex-acp\\dist\\index.js'],
  dir: 'C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.1.9',
  installed_at: '2026-08-06T12:00:00Z',
};

/** Hospeda o bloco com o mesmo estado que o formulário do provedor dá a ele. */
const Host = ({ agentId = 'codex-acp' }: { agentId?: string }) => {
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <span data-testid="comando">{command}</span>
      <span data-testid="argumentos">{args.join('\u0000')}</span>
      <AgentInstall
        agentId={agentId}
        onResolved={(novoComando, novosArgumentos) => {
          setCommand(novoComando);
          setArgs(novosArgumentos);
        }}
      />
    </div>
  );
};

/** Instalação que só responde quando o teste quiser. */
function instalacaoControlada() {
  let concluir: (installation: unknown) => void = () => {};
  let falhar: (erro: unknown) => void = () => {};
  installMock.mockReturnValue(
    new Promise((resolve, reject) => {
      concluir = resolve;
      falhar = reject;
    }),
  );
  return { concluir: (v: unknown) => concluir(v), falhar: (e: unknown) => falhar(e) };
}

afterEach(() => {
  vi.clearAllMocks();
});

describe('AgentInstall — antes de baixar', () => {
  it('não baixa nada sem confirmação, e a confirmação diz o que vai ser baixado', async () => {
    // AEP-0086 D3: agente, versão, origem e o que será executado à vista antes
    // de qualquer byte sair da rede.
    planMock.mockResolvedValue(planoInstalavel);
    const user = userEvent.setup();

    render(<Host />);
    const botao = await screen.findByRole('button', { name: /instalar pelo catálogo/i });
    expect(installMock).not.toHaveBeenCalled();

    await user.click(botao);

    const dialogo = await screen.findByRole('dialog');
    expect(dialogo).toHaveTextContent(/instalar codex pelo catálogo\?/i);
    expect(dialogo).toHaveTextContent('1.1.9');
    expect(dialogo).toHaveTextContent('@agentclientprotocol/codex-acp@1.1.9');
    expect(dialogo).toHaveTextContent(planoInstalavel.dir);
    expect(dialogo).toHaveTextContent(/install --prefix/);
    // Abrir a confirmação não é instalar.
    expect(installMock).not.toHaveBeenCalled();
  });

  it('desistir da confirmação não instala nada', async () => {
    planMock.mockResolvedValue(planoInstalavel);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /^cancelar$/i }));

    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(installMock).not.toHaveBeenCalled();
  });

  it('diz o que o botão faz antes de alguém clicar nele', async () => {
    planMock.mockResolvedValue(planoInstalavel);

    render(<Host />);

    const botao = await screen.findByRole('button', { name: /instalar pelo catálogo/i });
    expect(botao).toHaveAccessibleDescription(/mostra o que será baixado/i);
  });

  it('agente que o catálogo não publica não ganha oferta nenhuma', async () => {
    // O backend devolve plano vazio: configurar comando à mão continua valendo,
    // e um botão que só sabe falhar seria pior do que não haver botão.
    planMock.mockResolvedValue({});

    render(<Host agentId="algum-agente-proprio" />);

    await waitFor(() => expect(planMock).toHaveBeenCalledWith('algum-agente-proprio'));
    expect(screen.queryByRole('button', { name: /instalar pelo catálogo/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/instalar pelo catálogo de agentes/i)).not.toBeInTheDocument();
  });

  it('catálogo que não responde explica em texto por que não há oferta', async () => {
    planMock.mockRejectedValue(new Error('sem rede para consultar o registro'));

    render(<Host />);

    expect(await screen.findByText(/sem rede para consultar o registro/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /instalar pelo catálogo/i })).not.toBeInTheDocument();
  });
});

describe('AgentInstall — sem o runtime', () => {
  it('não oferece instalação e diz em texto o que falta e onde procurou', async () => {
    // AEP-0086 D7: o app não instala Node. O que ele faz é nomear o
    // pré-requisito, e um botão cinza sem explicação seria o pior desfecho para
    // quem navega por teclado.
    planMock.mockResolvedValue({
      ...planoInstalavel,
      install_command: '',
      runtime: {
        name: 'Node.js',
        required: true,
        found: false,
        searched: ['C:\\Program Files\\nodejs', 'C:\\Users\\ana\\AppData\\Roaming\\nvm'],
      },
      can_install: false,
      reason: 'o Node.js não foi encontrado nesta máquina',
    });

    render(<Host />);

    expect(await screen.findByText(/exige o node\.js, que não foi encontrado/i)).toBeInTheDocument();
    expect(screen.getByText(/o aplicativo não instala o node\.js/i)).toBeInTheDocument();
    expect(screen.getByText(/appdata\\roaming\\nvm/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /instalar pelo catálogo/i })).not.toBeInTheDocument();
  });

  it('sem saber o nome do agente, não monta a frase de apresentação', async () => {
    // A consulta ao catálogo que falha deixa um plano com o identificador e o
    // motivo, e nada mais. A apresentação viraria "publica  como pacote" com
    // buracos onde deveriam estar o agente e a versão.
    planMock.mockResolvedValue({
      agent_id: 'codex-acp',
      name: '',
      version: '',
      distribution: 'npm',
      run_args: [],
      runtime: nodeEncontrado,
      can_install: false,
      installing: false,
      reason: 'não foi possível consultar o catálogo',
    });

    render(<Host />);

    expect(await screen.findByText(/não foi possível consultar o catálogo/i)).toBeInTheDocument();
    expect(screen.queryByText(/publica/i)).not.toBeInTheDocument();
  });
});

describe('AgentInstall — artefato binário', () => {
  // O agente distribuído como binário não usa Node para nada, e sete dos que
  // publicam digest não têm alternativa npm: bloquear o download por falta de
  // um runtime que aquele caminho não usa deixaria justamente esses de fora.
  const planoBinario = {
    agent_id: 'opencode',
    name: 'opencode',
    version: '0.4.2',
    distribution: 'binary',
    origin: 'https://github.com/sst/opencode/releases/download/v0.4.2/opencode-windows-x64.zip',
    target: 'windows-x86_64',
    sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
    dir: 'C:\\Users\\ana\\.assistente\\agents\\opencode\\0.4.2',
    install_command: '',
    run_args: [],
    runtime: { name: 'Node.js', required: false, found: false, searched: [] },
    can_install: true,
    installing: false,
  };

  it('oferece a instalação numa máquina sem Node e não fala em pacote npm', async () => {
    planMock.mockResolvedValue(planoBinario);

    render(<Host agentId="opencode" />);

    const botao = await screen.findByRole('button', { name: /instalar pelo catálogo/i });
    expect(screen.queryByText(/exige o node\.js/i)).not.toBeInTheDocument();
    expect(screen.getByText(/programa pronto para o seu sistema/i)).toBeInTheDocument();
    // A descrição do botão é o que um leitor de telas lê antes do clique: ela
    // não pode prometer um pacote npm que não será baixado.
    expect(botao).toHaveAccessibleDescription(/conferindo o código de verificação/i);
  });

  it('manda o que foi confirmado junto do pedido de instalação', async () => {
    // O plano pode deixar de valer entre o diálogo e o clique, e é o backend
    // que recusa. Ele só consegue fazer isso se souber o que foi confirmado.
    planMock.mockResolvedValue(planoBinario);
    installMock.mockResolvedValue({ command: 'C:\\agents\\opencode.exe', args: [] });
    const user = userEvent.setup();

    render(<Host agentId="opencode" />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));

    await waitFor(() =>
      expect(installMock).toHaveBeenCalledWith('opencode', {
        distribution: 'binary',
        origin: planoBinario.origin,
        sha256: planoBinario.sha256,
        accept_unverified: false,
      }),
    );
  });

  it('a confirmação diz qual arquivo vem e com que código ele será conferido', async () => {
    // D3 continua valendo, com o que muda entre as distribuições: aqui não se
    // executa comando nenhum para instalar, e o que responde "o que vai
    // acontecer" é o arquivo, o alvo da plataforma e o digest.
    planMock.mockResolvedValue(planoBinario);
    const user = userEvent.setup();

    render(<Host agentId="opencode" />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

    const dialogo = await screen.findByRole('dialog');
    expect(dialogo).toHaveTextContent(/código de verificação publicado pelo registro/i);
    expect(dialogo).toHaveTextContent(planoBinario.origin);
    expect(dialogo).toHaveTextContent(planoBinario.target);
    expect(dialogo).toHaveTextContent(planoBinario.sha256);
    expect(dialogo).not.toHaveTextContent(/comando que será executado/i);
  });

  it('não tem violação na confirmação do artefato', async () => {
    planMock.mockResolvedValue(planoBinario);
    const user = userEvent.setup();

    render(<Host agentId="opencode" />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

    expect(await axe(screen.getByRole('dialog'))).toHaveNoViolations();
  });

  describe('sem código de verificação publicado', () => {
    // Metade dos agentes com binário não publica digest, e o Cursor é um deles.
    const planoSemDigest = {
      ...planoBinario,
      agent_id: 'cursor',
      name: 'Cursor',
      origin: 'https://downloads.cursor.com/cursor-agent-windows-x64.zip',
      sha256: '',
      unverified: true,
    };

    it('a confirmação nomeia em texto o que o aplicativo não consegue conferir', async () => {
      // D4: a ausência é dita por escrito, e não por um ícone de alerta que não
      // chega a quem não vê a tela. E o campo do digest não fica em branco: em
      // branco ele parece campo que não carregou.
      planMock.mockResolvedValue(planoSemDigest);
      const user = userEvent.setup();

      render(<Host agentId="cursor" />);
      await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

      const dialogo = await screen.findByRole('dialog');
      expect(dialogo).toHaveTextContent(/não publica verificação de integridade/i);
      expect(dialogo).toHaveTextContent(/não tem como conferir que o arquivo baixado/i);
      expect(dialogo).toHaveTextContent(/não publicado pelo registro/i);
      // E a abertura não promete a conferência que não vai acontecer: ela é a
      // frase do artefato com digest, e aqui diria o contrário do aviso.
      expect(dialogo).not.toHaveTextContent(/conferir se ele confere com o código de verificação/i);
    });

    it('o aviso entra na descrição do diálogo, e não só no corpo dele', async () => {
      // É o que um leitor de telas lê ao abrir. Fora da descrição, a frase que
      // nomeia a ausência vira texto que só quem varre a tela encontra (D4).
      planMock.mockResolvedValue(planoSemDigest);
      const user = userEvent.setup();

      render(<Host agentId="cursor" />);
      await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

      const dialogo = await screen.findByRole('dialog');
      expect(dialogo).toHaveAccessibleDescription(/não publica verificação de integridade/i);
    });

    it('o foco começa em cancelar, e instalar exige mover o foco até o outro botão', async () => {
      // O Enter ativa o botão focado, como em qualquer diálogo. O que esta regra
      // decide é onde o foco começa — e é o movimento até o botão afirmativo que
      // separa ler do reflexo de confirmar (D4).
      //
      // O `offsetParent` é encenado porque o jsdom não faz layout: sem ele o
      // modal considera invisível todo botão do diálogo e o foco inicial cai no
      // container, que é o que aconteceria em qualquer teste de foco daqui.
      const descritor = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'offsetParent');
      Object.defineProperty(HTMLElement.prototype, 'offsetParent', {
        configurable: true,
        get() {
          return document.body;
        },
      });
      try {
        planMock.mockResolvedValue(planoSemDigest);
        const user = userEvent.setup();

        render(<Host agentId="cursor" />);
        await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
        await screen.findByRole('dialog');

        const cancelar = screen.getByRole('button', { name: /^cancelar$/i });
        await waitFor(() => expect(cancelar).toHaveFocus());

        await user.keyboard('{Enter}');
        expect(installMock).not.toHaveBeenCalled();
        await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
      } finally {
        if (descritor) {
          Object.defineProperty(HTMLElement.prototype, 'offsetParent', descritor);
        } else {
          delete (HTMLElement.prototype as { offsetParent?: unknown }).offsetParent;
        }
      }
    });

    it('o botão afirmativo diz o que está sendo aceito, e não "confirmar"', async () => {
      // Num leitor de telas o nome do botão é o que se ouve antes de acioná-lo:
      // é a última chance de a frase acima não ter passado batida.
      planMock.mockResolvedValue(planoSemDigest);
      const user = userEvent.setup();

      render(<Host agentId="cursor" />);
      await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

      await screen.findByRole('dialog');
      expect(screen.getByRole('button', { name: /baixar mesmo sem verificação/i })).toBeInTheDocument();
      expect(screen.queryByRole('button', { name: /^baixar e instalar$/i })).not.toBeInTheDocument();
    });

    it('a resposta à pergunta viaja com o pedido, e não fica ligada em lugar nenhum', async () => {
      // O backend recusa sem ela. A decisão é por instalação: não há preferência
      // que a desligue, e por isso ela não sai de um estado guardado.
      planMock.mockResolvedValue(planoSemDigest);
      installMock.mockResolvedValue({ command: 'C:\\agents\\cursor-agent.exe', args: [] });
      const user = userEvent.setup();

      render(<Host agentId="cursor" />);
      await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
      await user.click(await screen.findByRole('button', { name: /baixar mesmo sem verificação/i }));

      await waitFor(() =>
        expect(installMock).toHaveBeenCalledWith('cursor', {
          distribution: 'binary',
          origin: planoSemDigest.origin,
          sha256: '',
          accept_unverified: true,
        }),
      );
    });

    it('depois de instalado o item continua dizendo que aquilo não foi verificado', async () => {
      // A marca não some (D4): quem abrir esta tela amanhã precisa saber o que
      // aquele agente é, e não só que ele está instalado.
      planMock.mockResolvedValue({
        ...planoSemDigest,
        can_install: false,
        installed: {
          agent_id: 'cursor',
          name: 'Cursor',
          version: '0.4.2',
          distribution: 'binary',
          target: 'windows-x86_64',
          sha256: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
          sha256_origin: 'observed',
          command: 'C:\\agents\\cursor-agent.exe',
          args: [],
          dir: 'C:\\Users\\ana\\.assistente\\agents\\cursor\\0.4.2',
          installed_at: '2026-08-07T12:00:00Z',
        },
      });

      render(<Host agentId="cursor" />);

      expect(await screen.findByText(/esta instalação não foi verificada/i)).toBeInTheDocument();
    });

    it('a instalação conferida não ganha a marca de não verificada', async () => {
      planMock.mockResolvedValue({
        ...planoBinario,
        can_install: false,
        installed: {
          agent_id: 'opencode',
          name: 'opencode',
          version: '0.4.2',
          distribution: 'binary',
          target: 'windows-x86_64',
          sha256: planoBinario.sha256,
          sha256_origin: 'verified',
          command: 'C:\\agents\\opencode.exe',
          args: [],
          dir: 'C:\\Users\\ana\\.assistente\\agents\\opencode\\0.4.2',
          installed_at: '2026-08-07T12:00:00Z',
        },
      });

      render(<Host agentId="opencode" />);

      await screen.findByRole('button', { name: /usar o comando instalado/i });
      expect(screen.queryByText(/não foi verificada/i)).not.toBeInTheDocument();
    });

    it('não tem violação na confirmação reforçada', async () => {
      planMock.mockResolvedValue(planoSemDigest);
      const user = userEvent.setup();

      render(<Host agentId="cursor" />);
      await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

      expect(await axe(screen.getByRole('dialog'))).toHaveNoViolations();
    });
  });
});

describe('AgentInstall — instalando', () => {
  it('anuncia os marcos, mostra o estado em texto e preenche o comando resolvido', async () => {
    // O critério de aceitação do AEP: instalar pelo catálogo produz um provider
    // que sobe, sem ninguém digitar caminho.
    planMock.mockResolvedValue(planoInstalavel);
    const { concluir } = instalacaoControlada();
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));

    await waitFor(() =>
      expect(installMock).toHaveBeenCalledWith('codex-acp', {
        distribution: 'npm',
        origin: planoInstalavel.origin,
        sha256: '',
        accept_unverified: false,
      }),
    );

    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'installing' });
    expect(screen.getByText(/baixando e instalando codex/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(expect.stringMatching(/baixando e instalando codex/i), 'polite');

    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'verifying' });
    expect(screen.getByText(/conferindo se codex responde ao protocolo/i)).toBeInTheDocument();

    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });
    await act(async () => {
      concluir(instalacao);
    });
    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'done' });

    expect(screen.getByTestId('comando')).toHaveTextContent(instalacao.command);
    expect(screen.getByTestId('argumentos')).toHaveTextContent(instalacao.args[0]);
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/instalado e respondendo ao protocolo/i),
      'polite',
    );
  });

  it('marca o bloco como ocupado enquanto a instalação corre, e o solta no fim (D13)', async () => {
    // `aria-busy`, e não uma região viva: quem anuncia cada marco é o announcer
    // global (AEP-0058), e um `role="status"` aqui diria tudo duas vezes. O que
    // o bloco precisa dizer é que está mudando, para quem o atravessa no meio da
    // instalação não ler metade de um marco e metade do seguinte.
    planMock.mockResolvedValue(planoInstalavel);
    const { concluir } = instalacaoControlada();
    const user = userEvent.setup();

    render(<Host />);
    const bloco = (await screen.findByRole('group')) as HTMLElement;
    expect(bloco).toHaveAttribute('aria-busy', 'false');

    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));
    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'true'));

    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });
    await act(async () => {
      concluir(instalacao);
    });

    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'false'));
  });

  it('reencontra a instalação que já estava em voo quando a tela abriu', async () => {
    // A instalação roda no backend e sobrevive ao formulário fechado. Voltando à
    // tela no meio dela, o botão de instalar não pode convidar a começar de novo
    // — e o de cancelar precisa estar ali, porque há o que cancelar.
    planMock.mockResolvedValue({ ...planoInstalavel, installing: true });

    render(<Host />);

    const instalar = await screen.findByRole('button', { name: /instalar pelo catálogo/i });
    expect(instalar).toBeDisabled();
    expect(screen.getByRole('button', { name: /cancelar instalação/i })).toBeInTheDocument();
  });

  it('solta a tela quando a instalação adotada termina', async () => {
    // Quem começou a instalação foi outra montagem do formulário, então não há
    // promessa nenhuma para encerrar o estado ocupado aqui: sem o marco de
    // desfecho, o botão de cancelar continuaria oferecendo cancelar o que já
    // acabou, e a tela nunca diria que o agente está instalado.
    planMock.mockResolvedValue({ ...planoInstalavel, installing: true });

    render(<Host />);
    expect(await screen.findByRole('button', { name: /cancelar instalação/i })).toBeInTheDocument();

    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });
    await act(async () => {
      emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'done' });
    });

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /cancelar instalação/i })).not.toBeInTheDocument(),
    );
    expect(await screen.findByRole('button', { name: /usar o comando instalado/i })).toBeInTheDocument();
  });

  it('solta a tela quando o plano diz que a instalação adotada já acabou', async () => {
    // O marco de desfecho pode não chegar: a instalação adotada pode ter
    // terminado entre o plano que a encontrou e o registro do ouvinte. O plano
    // seguinte é a outra resposta possível, e sem ele a tela ficaria ocupada
    // para sempre — o botão de cancelar oferecendo cancelar o que não existe.
    planMock.mockResolvedValue({ ...planoInstalavel, installing: true });
    const user = userEvent.setup();

    render(<Host />);
    expect(await screen.findByRole('button', { name: /cancelar instalação/i })).toBeInTheDocument();

    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });
    await user.click(screen.getByRole('button', { name: /cancelar instalação/i }));

    await waitFor(() =>
      expect(screen.queryByRole('button', { name: /cancelar instalação/i })).not.toBeInTheDocument(),
    );
    expect(await screen.findByRole('button', { name: /usar o comando instalado/i })).toBeInTheDocument();
  });

  it('o cancelar acompanha a instalação, e não o estado do plano', async () => {
    // O plano pode passar a dizer "indisponível" — Node que sumiu do PATH,
    // catálogo que parou de responder — com o npm ainda escrevendo no disco. Sem
    // o botão, sobraria uma instalação em voo e nenhuma forma de pará-la.
    planMock.mockResolvedValue({
      ...planoInstalavel,
      can_install: false,
      installing: true,
      reason: 'o catálogo parou de responder',
    });

    render(<Host />);

    expect(await screen.findByText(/o catálogo parou de responder/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /cancelar instalação/i })).toBeInTheDocument();
  });

  it('trocar de agente não leva junto a frase do anterior', async () => {
    // Estado é do agente que estava na tela. Depois da troca, a frase do
    // progresso descreveria o agente antigo — e num leitor de telas ela é a
    // única pista de qual agente está sendo instalado.
    planMock.mockResolvedValue(planoInstalavel);
    const { rerender } = render(<Host />);
    await screen.findByRole('button', { name: /instalar pelo catálogo/i });

    await act(async () => {
      emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'installing' });
    });
    expect(await screen.findByText(/baixando/i)).toBeInTheDocument();

    rerender(<Host agentId="claude-acp" />);

    await waitFor(() => expect(screen.queryByText(/baixando/i)).not.toBeInTheDocument());
  });

  it('trocar de agente com instalação em voo não aplica o comando dela', async () => {
    // A instalação é do backend e sobrevive à troca. Se ela terminar depois, o
    // comando do agente antigo cairia nos campos do provedor novo — e o
    // formulário salvaria um executável que não é o do agente escolhido.
    planMock.mockResolvedValue(planoInstalavel);
    const { concluir } = instalacaoControlada();
    const user = userEvent.setup();

    const { rerender } = render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));
    await waitFor(() => expect(installMock).toHaveBeenCalled());

    planMock.mockResolvedValue({ ...planoInstalavel, agent_id: 'claude-acp', name: 'Claude Code' });
    rerender(<Host agentId="claude-acp" />);
    await waitFor(() => expect(planMock).toHaveBeenLastCalledWith('claude-acp'));

    await act(async () => {
      concluir(instalacao);
    });

    expect(screen.getByTestId('comando')).toHaveTextContent('');
    expect(screen.getByTestId('argumentos')).toHaveTextContent('');
  });

  it('trocar de agente com instalação em voo não mostra a falha dela sob o agente novo', async () => {
    // A frase é a única pista de qual agente está sendo instalado: sob o nome do
    // agente novo, ela diz que falhou uma instalação que ninguém pediu ali.
    planMock.mockResolvedValue(planoInstalavel);
    const { falhar } = instalacaoControlada();
    const user = userEvent.setup();

    const { rerender } = render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));
    await waitFor(() => expect(installMock).toHaveBeenCalled());

    planMock.mockResolvedValue({ ...planoInstalavel, agent_id: 'claude-acp', name: 'Claude Code' });
    rerender(<Host agentId="claude-acp" />);
    await waitFor(() => expect(planMock).toHaveBeenLastCalledWith('claude-acp'));
    announceMock.mockClear();

    await act(async () => {
      falhar(new Error('npm ERR! network request failed'));
    });

    expect(screen.queryByText(/a instalação falhou/i)).not.toBeInTheDocument();
    expect(announceMock).not.toHaveBeenCalled();
    // O plano do agente novo continua sendo o último: um pedido pelo agente
    // antigo devolveria a este bloco a oferta do que saiu da tela.
    expect(planMock).toHaveBeenLastCalledWith('claude-acp');
  });

  it('marco de outro agente não descreve este', async () => {
    // Duas instalações podem estar em voo: um progresso sem dono descreveria na
    // tela do Codex o que está acontecendo com outro agente.
    planMock.mockResolvedValue(planoInstalavel);

    render(<Host />);
    await screen.findByRole('button', { name: /instalar pelo catálogo/i });

    emitirProgresso({ agent_id: 'gemini', agent: 'Gemini', stage: 'installing' });

    expect(screen.queryByText(/baixando e instalando/i)).not.toBeInTheDocument();
    expect(announceMock).not.toHaveBeenCalled();
  });

  it('oferece cancelar enquanto instala e diz que nada ficou no disco', async () => {
    planMock.mockResolvedValue(planoInstalavel);
    const { falhar } = instalacaoControlada();
    cancelMock.mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));
    await waitFor(() => expect(installMock).toHaveBeenCalled());

    await user.click(screen.getByRole('button', { name: /cancelar instalação/i }));
    expect(cancelMock).toHaveBeenCalledWith('codex-acp');

    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'cancelled' });
    await act(async () => {
      falhar(new Error('context canceled'));
    });

    expect(screen.getByText(/cancelada\. nada ficou no disco/i)).toBeInTheDocument();
    // Cancelar é decisão, e não defeito: não interrompe a leitura em curso, e o
    // erro que a chamada devolve depois não vira "a instalação falhou" na tela.
    expect(announceMock).toHaveBeenCalledWith(expect.stringMatching(/nada ficou no disco/i), 'polite');
    expect(screen.queryByText(/a instalação falhou/i)).not.toBeInTheDocument();
    expect(announceMock).not.toHaveBeenCalledWith(expect.stringMatching(/falhou/i), 'assertive');
    // O comando não foi preenchido: não houve instalação.
    expect(screen.getByTestId('comando')).toHaveTextContent('');
  });

  it('a falha nomeia a etapa, mantém o motivo do npm e interrompe a leitura', async () => {
    // "Falha na instalação" não é acionável (D13): quem vai resolver um proxy
    // corporativo precisa da etapa e do texto que o npm escreveu.
    planMock.mockResolvedValue(planoInstalavel);
    const { falhar } = instalacaoControlada();
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));
    await waitFor(() => expect(installMock).toHaveBeenCalled());

    emitirProgresso({
      agent_id: 'codex-acp',
      agent: 'Codex',
      stage: 'failed',
      step: 'install',
      reason: 'npm ERR! network request to https://registry.npmjs.org failed',
    });
    await act(async () => {
      falhar(new Error('npm ERR! network request to https://registry.npmjs.org failed'));
    });

    expect(screen.getByText(/falhou ao baixar o pacote com o npm/i)).toBeInTheDocument();
    expect(screen.getByText(/registry\.npmjs\.org failed/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/falhou ao baixar o pacote com o npm/i),
      'assertive',
    );
  });

  it('recusa que acontece antes de a instalação começar ainda aparece na tela', async () => {
    // Runtime ausente e agente já instalado voltam do backend sem marco nenhum:
    // o texto do erro é tudo o que existe, e ele não pode ficar sem aparecer.
    planMock.mockResolvedValue(planoInstalavel);
    installMock.mockRejectedValue(new Error('este agente já está instalado: Codex 1.1.9'));
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e instalar/i }));

    expect(await screen.findByText(/já está instalado: codex 1\.1\.9/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(expect.stringMatching(/já está instalado/i), 'assertive');
  });
});

describe('AgentInstall — já instalado', () => {
  const planoInstalado = { ...planoInstalavel, can_install: false, installed: instalacao };

  it('mostra o que está instalado e deixa reaproveitar o comando', async () => {
    planMock.mockResolvedValue(planoInstalado);
    const user = userEvent.setup();

    render(<Host />);

    expect(await screen.findByText(/instalado pelo aplicativo: codex versão 1\.1\.9/i)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(instalacao.dir.replace(/\\/g, '\\\\'), 'i'))).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /instalar pelo catálogo/i })).not.toBeInTheDocument();

    await user.click(screen.getByRole('button', { name: /usar o comando instalado/i }));

    expect(screen.getByTestId('comando')).toHaveTextContent(instalacao.command);
    expect(announceMock).toHaveBeenCalledWith(expect.stringContaining(instalacao.command), 'polite');
  });

  it('remover confirma, apaga o diretório e avisa que o provedor fica', async () => {
    // AEP-0086 D5: remover apaga o diretório e não apaga o provider de quem o
    // criou. Quem espera que ele suma junto precisa saber que ele fica.
    planMock.mockResolvedValue(planoInstalado);
    removeMock.mockResolvedValue(undefined);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /remover agente instalado/i }));

    const dialogo = await screen.findByRole('dialog');
    expect(dialogo).toHaveTextContent(/remover codex\?/i);
    expect(dialogo).toHaveTextContent(instalacao.dir);
    expect(dialogo).toHaveTextContent(/o provedor continua salvo/i);
    expect(removeMock).not.toHaveBeenCalled();

    planMock.mockResolvedValue(planoInstalavel);
    await user.click(screen.getByRole('button', { name: /^remover$/i }));

    await waitFor(() => expect(removeMock).toHaveBeenCalledWith('codex-acp'));
    expect(await screen.findByText(/o provedor continua salvo, e o comando dele passou a não existir/i))
      .toBeInTheDocument();
    // Removido, o catálogo volta a oferecer a instalação.
    expect(await screen.findByRole('button', { name: /instalar pelo catálogo/i })).toBeInTheDocument();
  });

  it('fica ocupado enquanto o diretório está sendo apagado, e não oferece cancelar (D13)', async () => {
    // O diálogo de confirmação fechado não é a remoção: ela começa depois, e é
    // durante ela que o bloco muda. Ocupado aqui não pode ser o mesmo estado da
    // instalação, que põe na tela um botão de cancelar sem nada para cancelar.
    planMock.mockResolvedValue(planoInstalado);
    let concluirRemocao: () => void = () => {};
    removeMock.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          concluirRemocao = () => resolve();
        }),
    );
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /remover agente instalado/i }));
    await user.click(await screen.findByRole('button', { name: /^remover$/i }));

    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'true'));
    expect(screen.queryByRole('button', { name: /cancelar instalação/i })).not.toBeInTheDocument();

    planMock.mockResolvedValue(planoInstalavel);
    await act(async () => {
      concluirRemocao();
    });

    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'false'));
  });

  it('remoção que falha aparece em texto e interrompe a leitura', async () => {
    planMock.mockResolvedValue(planoInstalado);
    removeMock.mockRejectedValue(new Error('arquivo em uso por outro processo'));
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /remover agente instalado/i }));
    await user.click(await screen.findByRole('button', { name: /^remover$/i }));

    expect(await screen.findByText(/arquivo em uso por outro processo/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('arquivo em uso por outro processo', 'assertive');
  });
});

describe('AgentInstall — versão nova', () => {
  // O catálogo publica a 1.2.0 e no disco está a 1.1.9. É o plano que o
  // instalador devolve depois que o registro avança (AEP-0086 D10).
  const planoComVersaoNova = {
    ...planoInstalavel,
    version: '1.2.0',
    origin: '@agentclientprotocol/codex-acp@1.2.0',
    dir: 'C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.2.0',
    can_install: false,
    installed: instalacao,
    update: true,
    can_update: true,
  };
  const novaInstalacao = {
    ...instalacao,
    version: '1.2.0',
    args: ['C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.2.0\\node_modules\\@agentclientprotocol\\codex-acp\\dist\\index.js'],
    dir: 'C:\\Users\\ana\\.assistente\\agents\\codex-acp\\1.2.0',
  };

  it('avisa em texto e não atualiza nada sozinho', async () => {
    planMock.mockResolvedValue(planoComVersaoNova);

    render(<Host />);

    expect(await screen.findByText(/publica a versão 1\.2\.0 deste agente, e a instalada é a 1\.1\.9/i))
      .toBeInTheDocument();
    expect(updateMock).not.toHaveBeenCalled();
    const botao = screen.getByRole('button', { name: /atualizar para a versão 1\.2\.0/i });
    expect(botao).toHaveAccessibleDescription(/instala a versão nova ao lado da atual/i);
  });

  it('a confirmação mostra as duas versões antes de baixar', async () => {
    // Mesmo D3 da instalação: o que vai ser baixado fica à vista. Aqui a versão
    // que sai entra junto — é ela que dá sentido à que entra.
    planMock.mockResolvedValue(planoComVersaoNova);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));

    const dialogo = await screen.findByRole('dialog');
    expect(dialogo).toHaveTextContent(/atualizar codex para a versão 1\.2\.0\?/i);
    expect(dialogo).toHaveTextContent(/versão instalada agora/i);
    expect(dialogo).toHaveTextContent('1.1.9');
    expect(dialogo).toHaveTextContent('@agentclientprotocol/codex-acp@1.2.0');
    expect(dialogo).toHaveTextContent(/só então apagar a anterior/i);
    expect(updateMock).not.toHaveBeenCalled();
  });

  it('atualizar troca o comando do provedor e diz que os provedores foram repontados', async () => {
    planMock.mockResolvedValue(planoComVersaoNova);
    updateMock.mockResolvedValue(novaInstalacao);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e atualizar/i }));

    await waitFor(() =>
      expect(updateMock).toHaveBeenCalledWith('codex-acp', {
        distribution: 'npm',
        origin: '@agentclientprotocol/codex-acp@1.2.0',
        sha256: '',
        accept_unverified: false,
      }),
    );
    expect(screen.getByTestId('argumentos')).toHaveTextContent(novaInstalacao.args[0]);
    const anuncio = await screen.findByText(/atualizado para a versão 1\.2\.0/i);
    expect(anuncio).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringContaining('1.2.0'),
      'polite',
    );
  });

  it('o marco de instalação concluída não apaga a frase da atualização', async () => {
    // O evento e a resposta da chamada correm por caminhos diferentes. O marco
    // fala de instalação e não sabe dos provedores repontados; quem pediu a
    // atualização é quem dá a última palavra, chegue o marco quando chegar.
    planMock.mockResolvedValue(planoComVersaoNova);
    updateMock.mockResolvedValue(novaInstalacao);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e atualizar/i }));
    await screen.findByText(/atualizado para a versão 1\.2\.0/i);

    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'verifying' });
    emitirProgresso({ agent_id: 'codex-acp', agent: 'Codex', stage: 'done' });

    expect(screen.getByText(/atualizado para a versão 1\.2\.0/i)).toBeInTheDocument();
    expect(screen.queryByText(/comando preenchido/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/responde ao protocolo\.\.\./i)).not.toBeInTheDocument();
  });

  it('a recusa por conversa em voo aparece em texto e interrompe a leitura', async () => {
    // O agente em uso não é trocado debaixo de quem conversa com ele (D10), e o
    // motivo é dito — a atualização não fica esperando em silêncio.
    planMock.mockResolvedValue(planoComVersaoNova);
    updateMock.mockRejectedValue(
      new Error('o provedor "Codex" está no meio de uma conversa com este agente; espere o turno terminar para atualizar'),
    );
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e atualizar/i }));

    expect(await screen.findByText(/no meio de uma conversa com este agente/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringContaining('espere o turno terminar'),
      'assertive',
    );
  });

  it('quando não dá para atualizar, o motivo fica no lugar do botão', async () => {
    planMock.mockResolvedValue({
      ...planoComVersaoNova,
      can_update: false,
      update_reason:
        'a versão nova deste agente não publica verificação de integridade, e a instalada foi conferida',
    });

    render(<Host />);

    expect(await screen.findByText(/não publica verificação de integridade/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /atualizar para a versão/i })).not.toBeInTheDocument();
  });

  it('sem versão nova não há aviso nem botão', async () => {
    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });

    render(<Host />);

    await screen.findByText(/instalado pelo aplicativo/i);
    expect(screen.queryByText(/o catálogo publica a versão/i)).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /atualizar para a versão/i })).not.toBeInTheDocument();
  });

  it('durante a atualização o bloco fica ocupado, oferece cancelar e não oferece remover', async () => {
    // Apagar a pasta no meio do download é apagar o que ele está escrevendo; o
    // caminho de desistir é o cancelar (D13).
    planMock.mockResolvedValue(planoComVersaoNova);
    let concluir: (installation: unknown) => void = () => {};
    updateMock.mockReturnValue(new Promise((resolve) => { concluir = resolve; }));
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));
    await user.click(await screen.findByRole('button', { name: /baixar e atualizar/i }));

    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'true'));
    expect(screen.getByRole('button', { name: /cancelar instalação/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /remover agente instalado/i })).toBeDisabled();
    // O comando que está na tela é o da versão que vai sair: reaproveitá-lo
    // agora devolveria ao provedor o que o backend acabou de trocar.
    expect(screen.getByRole('button', { name: /usar o comando instalado/i })).toBeDisabled();

    planMock.mockResolvedValue({ ...planoComVersaoNova, update: false, installed: novaInstalacao });
    await act(async () => {
      concluir(novaInstalacao);
    });

    await waitFor(() => expect(screen.getByRole('group')).toHaveAttribute('aria-busy', 'false'));
  });

  it('não tem violação no aviso de versão nova nem na confirmação dela', async () => {
    planMock.mockResolvedValue(planoComVersaoNova);
    const user = userEvent.setup();

    const { container } = render(<Host />);
    await screen.findByRole('button', { name: /atualizar para a versão 1\.2\.0/i });
    expect(await axe(container)).toHaveNoViolations();

    await user.click(screen.getByRole('button', { name: /atualizar para a versão 1\.2\.0/i }));
    expect(await axe(await screen.findByRole('dialog'))).toHaveNoViolations();
  });
});

describe('AgentInstall — acessibilidade', () => {
  it('não tem violação com a instalação oferecida', async () => {
    planMock.mockResolvedValue(planoInstalavel);

    const { container } = render(<Host />);
    await screen.findByRole('button', { name: /instalar pelo catálogo/i });

    expect(await axe(container)).toHaveNoViolations();
  });

  it('não tem violação no aviso de runtime ausente', async () => {
    planMock.mockResolvedValue({
      ...planoInstalavel,
      install_command: '',
      runtime: { name: 'Node.js', required: true, found: false, searched: ['C:\\Program Files\\nodejs'] },
      can_install: false,
      reason: 'o Node.js não foi encontrado nesta máquina',
    });

    const { container } = render(<Host />);
    await screen.findByText(/exige o node\.js/i);

    expect(await axe(container)).toHaveNoViolations();
  });

  it('não tem violação na confirmação do que será baixado', async () => {
    planMock.mockResolvedValue(planoInstalavel);
    const user = userEvent.setup();

    render(<Host />);
    await user.click(await screen.findByRole('button', { name: /instalar pelo catálogo/i }));

    // O diálogo sai por portal, fora do container do teste.
    expect(await axe(screen.getByRole('dialog'))).toHaveNoViolations();
  });

  it('não tem violação com o agente já instalado', async () => {
    planMock.mockResolvedValue({ ...planoInstalavel, can_install: false, installed: instalacao });

    const { container } = render(<Host />);
    await screen.findByRole('button', { name: /remover agente instalado/i });

    expect(await axe(container)).toHaveNoViolations();
  });
});
