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
  ACPAgentInstallPlanForKind: planMock,
  InstallACPAgent: installMock,
  CancelACPAgentInstall: cancelMock,
  RemoveACPAgent: removeMock,
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
const Host = ({ agentKind = 'codex' }: { agentKind?: string }) => {
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <span data-testid="comando">{command}</span>
      <span data-testid="argumentos">{args.join('\u0000')}</span>
      <AgentInstall
        agentKind={agentKind}
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

  it('tipo de provedor que o catálogo não publica não ganha oferta nenhuma', async () => {
    // O backend devolve plano vazio: configurar comando à mão continua valendo,
    // e um botão que só sabe falhar seria pior do que não haver botão.
    planMock.mockResolvedValue({});

    render(<Host agentKind="algum-agente-proprio" />);

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

    await waitFor(() => expect(installMock).toHaveBeenCalledWith('codex-acp'));

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

    rerender(<Host agentKind="claude-code" />);

    await waitFor(() => expect(screen.queryByText(/baixando/i)).not.toBeInTheDocument());
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
      runtime: { name: 'Node.js', found: false, searched: ['C:\\Program Files\\nodejs'] },
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
