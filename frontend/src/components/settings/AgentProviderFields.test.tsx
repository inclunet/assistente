import { StrictMode, useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { AgentProviderFields, agentLoginCommand } from './AgentProviderFields';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());
const testMock = vi.hoisted(() => vi.fn());
const listCredentialsMock = vi.hoisted(() => vi.fn());

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
    }),
  };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceMock }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  DetectACPAgent: detectMock,
  TestACPAgent: testMock,
  // A instalação pelo catálogo tem teste próprio. Aqui ela só precisa não
  // aparecer: um plano sem agente é o tipo de provedor que o catálogo não
  // publica, e o bloco não é renderizado.
  ACPAgentInstallPlan: vi.fn().mockResolvedValue({}),
  InstallACPAgent: vi.fn(),
  CancelACPAgentInstall: vi.fn(),
  RemoveACPAgent: vi.fn(),
  UpdateACPAgent: vi.fn(),
}));

vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
  ListCredentials: listCredentialsMock,
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: vi.fn(() => () => {}),
}));

const cursorFound = {
  detectable: true,
  found: true,
  command: 'C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\node.exe',
  args: ['C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\index.js', 'acp'],
  version: '2026.07.30-abc123',
  source: 'C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\index.js',
  searched: ['C:\\Users\\ana\\AppData\\Local\\cursor-agent'],
  work_dir: 'C:\\Users\\ana\\projetos\\assistente',
};

const cursorMissing = {
  detectable: true,
  found: false,
  command: '',
  args: [],
  searched: ['C:\\Users\\ana\\AppData\\Local\\cursor-agent', 'C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions'],
  work_dir: 'C:\\Users\\ana\\projetos\\assistente',
};

/**
 * Hospeda o componente com o mesmo estado controlado que o formulário dá a ele,
 * para os testes verem o que a detecção preenche de verdade.
 */
const Host = ({
  initialCommand = '',
  autoFill = true,
  agentId = 'cursor',
  initialCredentialEnv = {},
}: {
  initialCommand?: string;
  autoFill?: boolean;
  agentId?: string;
  initialCredentialEnv?: Record<string, string>;
}) => {
  const [command, setCommand] = useState(initialCommand);
  const [args, setArgs] = useState<string[]>([]);
  const [credentialEnv, setCredentialEnv] = useState<Record<string, string>>(initialCredentialEnv);
  return (
    <div>
      <span data-testid="args-atual">{args.join(',')}</span>
      <span data-testid="cofre-do-pai">{JSON.stringify(credentialEnv)}</span>
      <AgentProviderFields
        agentId={agentId}
        command={command}
        args={args}
        onCommandChange={setCommand}
        onArgsChange={setArgs}
        credentialEnv={credentialEnv}
        onCredentialEnvChange={setCredentialEnv}
        autoFill={autoFill}
      />
    </div>
  );
};

/**
 * Hospeda os campos do jeito que o formulário faz e permite sair do modo agente,
 * que é o que acontece quando alguém troca o tipo do provedor.
 */
const HostQueTrocaDeTipo = () => {
  const [ehAgente, setEhAgente] = useState(true);
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <span data-testid="comando-do-pai">{command}</span>
      <span data-testid="argumentos-do-pai">{JSON.stringify(args)}</span>
      <button type="button" onClick={() => setEhAgente(false)}>trocar para http</button>
      {ehAgente && (
        <AgentProviderFields
          agentId="cursor"
          command={command}
          args={args}
          onCommandChange={setCommand}
          onArgsChange={setArgs}
          credentialEnv={{}}
          onCredentialEnvChange={() => {}}
          autoFill
        />
      )}
    </div>
  );
};

/**
 * Hospeda os campos do jeito que o formulário faz quando alguém troca de um
 * agente para outro: os campos continuam na tela, e o comando é limpo para a
 * detecção do agente novo preencher.
 */
const HostQueTrocaDeAgente = () => {
  const [agentId, setAgentId] = useState('claude-acp');
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <button
        type="button"
        onClick={() => {
          setAgentId('cursor');
          setCommand('');
          setArgs([]);
        }}
      >
        trocar para cursor
      </button>
      <AgentProviderFields
        agentId={agentId}
        command={command}
        args={args}
        onCommandChange={setCommand}
        onArgsChange={setArgs}
        credentialEnv={{}}
        onCredentialEnvChange={() => {}}
        autoFill
      />
    </div>
  );
};

/**
 * Hospeda os campos de um agente que pode deixar de ser escolhido: é o
 * caminho de quem escolheu um agente e voltou atrás antes de a procura dele
 * responder.
 */
const HostQuePodeDesescolherOAgente = () => {
  const [agentId, setAgentId] = useState('cursor');
  const [command, setCommand] = useState('');
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <button type="button" onClick={() => { setAgentId(''); setCommand(''); setArgs([]); }}>
        desescolher agente
      </button>
      <AgentProviderFields
        agentId={agentId}
        command={command}
        args={args}
        onCommandChange={setCommand}
        onArgsChange={setArgs}
        credentialEnv={{}}
        onCredentialEnvChange={() => {}}
        autoFill
      />
    </div>
  );
};

/** Detecção que só responde quando o teste quiser. */
function deteccaoControlada() {
  let responder: (setup: unknown) => void = () => {};
  detectMock.mockReturnValue(new Promise((resolve) => { responder = resolve; }));
  return (setup: unknown) => responder(setup);
}

beforeEach(() => {
  // O cofre responde vazio por padrão: os testes que falam dele dizem o que
  // tem dentro, e os demais não deviam depender de credencial nenhuma.
  listCredentialsMock.mockResolvedValue([]);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('AgentProviderFields — agente encontrado', () => {
  it('preenche comando e argumentos com o que a detecção achou', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    expect(screen.getByLabelText(/argumentos/i)).toHaveValue(cursorFound.args.join('\n'));
    expect(detectMock).toHaveBeenCalledWith('cursor');
  });

  it('mostra de onde veio o comando, com a versão instalada', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    await waitFor(() => {
      expect(screen.getByText(new RegExp(`versão ${cursorFound.version}`, 'i'))).toBeInTheDocument();
    });
  });

  it('mostra o diretório de trabalho como leitura, e não como escolha', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    // Rótulo e valor ligados no mesmo par: quem usa leitor de telas ouve o que
    // o caminho significa, sem depender de o texto vir logo antes na tela.
    const rotulo = await screen.findByRole('term');
    expect(rotulo).toHaveTextContent(/diretório de trabalho/i);
    const valor = screen.getByRole('definition');
    await waitFor(() => expect(valor).toHaveTextContent(cursorFound.work_dir));
    expect(valor.parentElement).toBe(rotulo.parentElement);

    // Não é campo: não há caixa de texto do diretório para preencher, e as duas
    // que sobram são as que se editam de verdade (comando e argumentos).
    expect(screen.queryByLabelText(/diretório de trabalho/i)).not.toBeInTheDocument();
    expect(screen.getAllByRole('textbox')).toHaveLength(2);
  });

  it('explica que o diretório é o workspace ativo, e não uma escolha desta tela', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    expect(
      await screen.findByText(/é onde o agente lê e edita arquivos.*workspace ativo/i),
    ).toBeInTheDocument();
  });

  it('sem workspace ativo, diz que não há em vez de deixar o diretório em branco', async () => {
    detectMock.mockResolvedValue({ ...cursorFound, work_dir: '' });

    render(<Host />);

    await waitFor(() => {
      expect(screen.getByRole('definition')).toHaveTextContent(/nenhum workspace ativo/i);
    });
  });

  it('mantém apenas a ação explícita de testar o agente', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    const testar = await screen.findByRole('button', { name: /testar agente/i });
    expect(testar).toHaveAccessibleDescription(/informa se ele respondeu.*não altera os campos/i);
    expect(screen.queryByRole('button', { name: /detectar/i })).not.toBeInTheDocument();
  });

  it('não sobrescreve o comando já salvo ao abrir a edição', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host initialCommand="/usr/local/bin/cursor-agent" autoFill={false} />);

    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());
    expect(detectMock).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('/usr/local/bin/cursor-agent');
  });

  it('não pisa no comando digitado enquanto a detecção estava em voo', async () => {
    // A detecção automática decide preencher quando a resposta chega, não quando
    // a chamada sai: quem digita nesse meio-tempo não pode perder o que escreveu.
    const responder = deteccaoControlada();
    const user = userEvent.setup();

    render(<Host />);
    const campo = screen.getByLabelText(/comando do agente/i);
    await user.type(campo, '/opt/cursor/agente');

    await act(async () => {
      responder(cursorFound);
    });
    expect(campo).toHaveValue('/opt/cursor/agente');
  });

  it('não pisa nos argumentos digitados, e ainda preenche o comando que faltava', async () => {
    // Comando e argumentos são campos separados: decidir pelos dois olhando só o
    // comando fazia quem digitou argumentos perdê-los.
    const responder = deteccaoControlada();
    const user = userEvent.setup();

    render(<Host />);
    await user.type(screen.getByLabelText(/argumentos/i), 'acp{enter}--meu-modo');
    responder(cursorFound);

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    expect(screen.getByLabelText(/argumentos/i)).toHaveValue('acp\n--meu-modo');
  });

  it('deixa digitar mais de um argumento, uma linha por vez', async () => {
    // Linha vazia não é argumento, mas apagá-la a cada tecla tirava o Enter de
    // quem configura pelo teclado: dava para colar dois argumentos e não para
    // digitá-los.
    detectMock.mockResolvedValue(cursorMissing);
    const user = userEvent.setup();

    render(<Host />);
    const campo = screen.getByLabelText(/argumentos/i);
    await user.type(campo, 'acp{enter}--modo-teste');

    expect(campo).toHaveValue('acp\n--modo-teste');
    expect(screen.getByTestId('args-atual')).toHaveTextContent('acp,--modo-teste');
  });

  it('detecção em voo preenche o campo que continuou vazio', async () => {
    const responder = deteccaoControlada();

    render(<Host />);
    responder(cursorFound);

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
  });

  it('descarta resposta que chega depois de o formulário deixar de ser de agente', async () => {
    // Trocar o tipo desmonta estes campos. Escrever no pai depois disso deixaria
    // comando e argumentos de agente pendurados num provedor que agora é HTTP.
    const responder = deteccaoControlada();
    const user = userEvent.setup();

    render(<HostQueTrocaDeTipo />);
    await waitFor(() => expect(detectMock).toHaveBeenCalledWith('cursor'));

    await user.click(screen.getByRole('button', { name: /trocar para http/i }));
    expect(screen.queryByLabelText(/comando do agente/i)).not.toBeInTheDocument();

    await act(async () => {
      responder(cursorFound);
    });

    expect(screen.getByTestId('comando-do-pai')).toHaveTextContent('');
    expect(screen.getByTestId('argumentos-do-pai')).toHaveTextContent('[]');
    expect(announceMock).not.toHaveBeenCalled();
  });

  it('campo preenchido não dispara detecção nem é sobrescrito', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host initialCommand="cursor-agent-antigo" autoFill={false} />);
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());

    expect(detectMock).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('cursor-agent-antigo');
  });
});

// O app roda dentro de `React.StrictMode`, que em desenvolvimento monta,
// desmonta e remonta cada componente. A remontagem precisa deixar os campos tão
// vivos quanto na primeira vez: o que sobrevive à desmontagem simulada e não é
// refeito na volta cala a tela para sempre.
describe('AgentProviderFields — remontado pelo StrictMode', () => {
  it('a procura que responde depois da remontagem ainda preenche e destrava o formulário', async () => {
    const responder = deteccaoControlada();

    render(
      <StrictMode>
        <Host />
      </StrictMode>,
    );
    // A resposta só sai depois de a procura ter saído: responder antes disso
    // testaria outra coisa.
    await waitFor(() => expect(detectMock).toHaveBeenCalledWith('cursor'));

    await act(async () => {
      responder(cursorFound);
    });

    await waitFor(() =>
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command),
    );
    expect(screen.getByRole('button', { name: /testar agente/i })).toBeEnabled();
  });

  it('o teste do agente também volta a responder depois da remontagem', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({ state: 'online', agent_name: 'Cursor' });
    const user = userEvent.setup();

    render(
      <StrictMode>
        <Host />
      </StrictMode>,
    );
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command));

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/cursor respondeu e aceitou abrir sessão/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /testar agente/i })).toBeEnabled();
  });
});

describe('AgentProviderFields — agente ausente', () => {
  it('explica o que fazer e onde procurou, em vez de falhar em silêncio', async () => {
    detectMock.mockResolvedValue(cursorMissing);

    render(<Host />);

    expect(await screen.findByText(/agente não encontrado nesta máquina/i)).toBeInTheDocument();
    expect(screen.getByText(/escolha no picker oferece uma instalação gerenciada/i)).toBeInTheDocument();
    expect(screen.getByText(new RegExp(cursorMissing.searched[1].replace(/\\/g, '\\\\'), 'i'))).toBeInTheDocument();
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('');
  });

  it('anuncia a ausência para leitor de telas, com o que resolver', async () => {
    detectMock.mockResolvedValue(cursorMissing);

    render(<Host />);

    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith(
        expect.stringMatching(/instale o cli do agente ou informe o comando manualmente/i),
        'assertive',
      );
    });
  });

  it('não alarma quem edita um provedor que já tem comando salvo', async () => {
    // Ali a procura é informativa: o comando salvo é a escolha de quem
    // configurou, e um alarme assertivo interromperia a leitura para descrever
    // um problema que não existe.
    detectMock.mockResolvedValue(cursorMissing);

    render(<Host initialCommand="/opt/cursor/agente" autoFill={false} />);

    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());
    expect(detectMock).not.toHaveBeenCalled();
    expect(announceMock).not.toHaveBeenCalled();
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('/opt/cursor/agente');
  });

  it('não oferece detecção manual para substituir comando salvo', async () => {
    detectMock.mockResolvedValue(cursorMissing);

    render(<Host initialCommand="/opt/cursor/agente" autoFill={false} />);
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());

    expect(detectMock).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /detectar/i })).not.toBeInTheDocument();
  });

  it('sem agente escolhido não oferece procurar nem manda instalar nada', async () => {
    // É o estado de quem acabou de escolher o tipo e ainda não abriu o
    // catálogo. Um botão de procurar ali não teria o que procurar, e o bloco
    // "instale o CLI" mandaria resolver uma ausência que ninguém constatou.
    render(<Host agentId="" />);

    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toBeInTheDocument());
    expect(detectMock).not.toHaveBeenCalled();
    expect(screen.queryByRole('button', { name: /detectar e preencher comando/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/instale o cli do agente/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/agente não encontrado nesta máquina/i)).not.toBeInTheDocument();
  });

  it('desescolher o agente durante a procura não deixa a tela procurando para sempre', async () => {
    // A procura em voo é aposentada pela troca, e quem a aposenta tem de
    // apagar o que ela escreveu na tela: o "procurando agente" é dela, e a
    // resposta que o desligaria nunca mais vai ser considerada.
    const responder = deteccaoControlada();
    const user = userEvent.setup();

    render(<HostQuePodeDesescolherOAgente />);
    // O texto aparece no botão e no estado, e é o segundo que importa aqui: o
    // botão some junto com o agente, e o estado é o que ficaria na tela.
    await waitFor(() => expect(screen.getAllByText(/procurando o agente/i).length).toBeGreaterThan(0));

    await user.click(screen.getByRole('button', { name: /desescolher agente/i }));

    await waitFor(() => expect(screen.queryAllByText(/procurando o agente/i)).toHaveLength(0));

    // E a resposta atrasada continua sem valer: ela fala de uma pergunta que
    // ninguém faz mais.
    responder(cursorFound);
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(''));
  });

  it('agente que o app não sabe procurar diz isso, em vez de oferecer o botão', async () => {
    // São 38 agentes no catálogo e detecção escrita à mão para dois (AEP-0086
    // D1). Para os outros, um botão de procurar só teria uma resposta possível,
    // e chamar de "não encontrado" uma procura que nunca aconteceu seria mentir
    // sobre a máquina de quem tem o agente instalado.
    detectMock.mockResolvedValue({ detectable: false, found: false, searched: [] });

    render(<Host agentId="gemini-cli" />);

    expect(await screen.findByText(/não sabe procurar este agente no disco/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /detectar e preencher comando/i })).not.toBeInTheDocument();
    // Testar continua valendo: o comando apontado à mão é o que se confere.
    expect(screen.getByRole('button', { name: /testar agente/i })).toBeInTheDocument();
    expect(announceMock).not.toHaveBeenCalled();
  });

  it('anuncia a falha quando a própria procura quebra', async () => {
    detectMock.mockRejectedValue(new Error('acesso negado ao diretório'));

    render(<Host />);

    expect(await screen.findByText(/acesso negado ao diretório/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('acesso negado ao diretório', 'assertive');
  });

  it('campo voltar a ficar vazio tenta a detecção novamente', async () => {
    detectMock.mockRejectedValueOnce(new Error('acesso negado ao diretório'));
    const user = userEvent.setup();

    render(<Host />);
    await screen.findByText(/acesso negado ao diretório/i);

    detectMock.mockResolvedValue(cursorFound);
    const command = screen.getByLabelText(/comando do agente/i);
    await user.type(command, 'x');
    await user.clear(command);

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
  });
});

describe('agentLoginCommand', () => {
  it('troca o subcomando do protocolo pelo do login', () => {
    expect(agentLoginCommand('cursor-agent', ['acp'])).toBe('cursor-agent login');
  });

  it('mantém o caminho do agente do Windows, com o index.js', () => {
    expect(agentLoginCommand(cursorFound.command, cursorFound.args)).toBe(
      `${cursorFound.command} ${cursorFound.args[0]} login`,
    );
  });

  it('põe aspas em caminho com espaço, para a linha poder ser copiada', () => {
    expect(agentLoginCommand('C:\\Program Files\\node\\node.exe', ['C:\\cli\\index.js', 'acp'])).toBe(
      '"C:\\Program Files\\node\\node.exe" C:\\cli\\index.js login',
    );
  });

  it('sem comando não inventa comando nenhum', () => {
    expect(agentLoginCommand('   ', [])).toBe('');
  });

  it('sem o subcomando do protocolo não há o que trocar', () => {
    // É o adaptador npm do Claude Code: `node ...\index.js` não vira
    // `node ...\index.js login`, porque o adaptador não tem login.
    expect(agentLoginCommand('node', ['C:\\npm\\claude-agent-acp\\dist\\index.js'])).toBe('');
  });
});

describe('AgentProviderFields — teste do agente', () => {
  it('testa o comando configurado e diz que ele atende', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({
      state: 'online',
      agent_name: 'Cursor',
      agent_version: '2026.07.23',
      latency_ms: 120,
      work_dir: cursorFound.work_dir,
    });
    const user = userEvent.setup();

    render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/cursor respondeu e aceitou abrir sessão/i)).toBeInTheDocument();
    expect(testMock).toHaveBeenCalledWith(cursorFound.command, cursorFound.args);
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/respondeu e aceitou abrir sessão/i),
      'polite',
    );
  });

  it('estado sem login explica o login do CLI, mostra o comando e anuncia', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({
      state: 'unauthenticated',
      agent_name: 'Cursor',
      latency_ms: 90,
      error: 'abrir sessão no agente ACP: agente ACP não autenticado',
      login_methods: [{ id: 'cursor_login', name: 'Entrar no Cursor' }],
    });
    const user = userEvent.setup();

    render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/instalado, mas não está autenticado/i)).toBeInTheDocument();
    expect(screen.getByText(/abra um terminal e rode o comando abaixo/i)).toBeInTheDocument();
    // O comando mostrado é o do agente configurado, com `login` no lugar do
    // `acp`. Um `cursor-agent login` fixo não existiria nesta máquina Windows,
    // onde o agente é o `node.exe` com o `index.js`.
    expect(screen.getByText(`${cursorFound.command} ${cursorFound.args[0]} login`)).toBeInTheDocument();
    expect(screen.getByText(/entrar no cursor/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/instalado, mas não está autenticado/i),
      'assertive',
    );
  });

  it('o comando que o próprio agente informou prevalece sobre o palpite da tela', async () => {
    // O agente sobe com `--acp` e o palpite da tela produziria
    // `copilot --acp login`, que não existe. Ele publica o comando certo no
    // handshake, e é esse que a pessoa tem de ver.
    detectMock.mockResolvedValue({ detectable: false, found: false, searched: [] });
    testMock.mockResolvedValue({
      state: 'unauthenticated',
      agent_name: 'GitHub Copilot CLI',
      login_command: 'copilot login',
      login_methods: [
        { id: 'copilot-login', name: 'Entrar no Copilot', command: 'copilot login' },
      ],
    });
    const user = userEvent.setup();

    render(<Host initialCommand="copilot" autoFill={false} agentId="github-copilot-cli" />);
    await user.type(screen.getByLabelText(/argumentos/i), '--acp');
    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/o próprio agente informou como autenticar/i)).toBeInTheDocument();
    expect(screen.getByText('copilot login')).toBeInTheDocument();
    expect(screen.queryByText(/--acp login/)).not.toBeInTheDocument();
  });

  it('o que o agente escreveu sobre o login aparece como ele escreveu', async () => {
    // O OpenCode não publica comando: ele explica em texto que o login é
    // `opencode auth login`. Resumir isso num rótulo jogaria fora a única
    // instrução que existe.
    detectMock.mockResolvedValue({ detectable: false, found: false, searched: [] });
    testMock.mockResolvedValue({
      state: 'unauthenticated',
      agent_name: 'OpenCode',
      login_methods: [
        {
          id: 'opencode-login',
          name: 'Entrar no OpenCode',
          description: 'Rode `opencode auth login` no terminal',
        },
      ],
    });
    const user = userEvent.setup();

    render(<Host initialCommand="opencode" autoFill={false} agentId="opencode" />);
    await user.type(screen.getByLabelText(/argumentos/i), 'acp');
    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/instalado, mas não está autenticado/i)).toBeInTheDocument();
    expect(screen.getByText(/o agente explicou como autenticar/i)).toBeInTheDocument();
    expect(
      screen.getByText(/entrar no opencode: rode `opencode auth login` no terminal/i),
    ).toBeInTheDocument();
    // O palpite da tela seria `opencode login`, que não existe. Mostrá-lo ao
    // lado da instrução do agente daria duas ordens contraditórias, e a que
    // tem cara de comando pronto é a errada.
    expect(screen.queryByText('opencode login')).not.toBeInTheDocument();
  });

  it('agente cujo login é outro programa mostra o comando que a detecção deu', async () => {
    // No Claude Code o que sobe o ACP é um adaptador npm, que não tem login
    // nenhum: quem autentica é o CLI `claude`. Derivar o login do comando
    // configurado, como no Cursor, mandaria a pessoa a um comando inexistente.
    const claudeCode = {
      detectable: true,
      found: true,
      command: 'C:\\Program Files\\nodejs\\node.exe',
      args: ['C:\\Program Files\\nodejs\\node_modules\\@agentclientprotocol\\claude-agent-acp\\dist\\index.js'],
      version: '0.65.0',
      source: 'C:\\Program Files\\nodejs\\node_modules\\@agentclientprotocol\\claude-agent-acp\\dist\\index.js',
      searched: [],
      work_dir: 'C:\\Users\\ana\\projetos\\assistente',
      login_command: 'claude',
    };
    detectMock.mockResolvedValue(claudeCode);
    // O adaptador não anuncia método de login nenhum (`authMethods` vazio), e é
    // por isso que a instrução da tela precisa bastar por si.
    testMock.mockResolvedValue({
      state: 'unauthenticated',
      agent_name: 'Claude Agent',
      agent_version: '0.65.0',
      latency_ms: 140,
    });
    const user = userEvent.setup();

    render(<Host agentId="claude-acp" />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(claudeCode.command);
    });

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/instalado, mas não está autenticado/i)).toBeInTheDocument();
    expect(screen.getByText(/abra um terminal e rode o comando abaixo/i)).toBeInTheDocument();
    expect(screen.getByText('claude')).toBeInTheDocument();
    expect(screen.queryByText(/index\.js login/)).not.toBeInTheDocument();
  });

  it('o login do agente anterior some assim que o agente muda', async () => {
    // A procura do agente novo demora, e nesse intervalo os campos continuam na
    // tela: manter o comando de login da procura anterior mandaria autenticar o
    // Claude Code num provedor do Cursor.
    detectMock.mockResolvedValue({
      detectable: true,
      found: true,
      command: 'node',
      args: ['claude-agent-acp/dist/index.js'],
      searched: [],
      login_command: 'claude',
    });
    testMock.mockResolvedValue({ state: 'unauthenticated', agent_name: 'Claude Agent' });
    const user = userEvent.setup();

    render(<HostQueTrocaDeAgente />);
    await waitFor(() => expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('node'));
    await user.click(screen.getByRole('button', { name: /testar agente/i }));
    expect(await screen.findByText('claude')).toBeInTheDocument();

    // A procura do Cursor fica em voo: é exatamente a janela em que a tela
    // ainda teria o resultado do Claude Code em mãos.
    deteccaoControlada();
    await user.click(screen.getByRole('button', { name: /trocar para cursor/i }));

    await waitFor(() => expect(screen.queryByText('claude')).not.toBeInTheDocument());
  });

  it('sem saber o comando de login, pede a procura em vez de chutar um', async () => {
    // Comando configurado à mão, procura que não achou nada: ninguém tem o
    // comando de login a oferecer, e derivá-lo do adaptador npm mandaria a
    // pessoa a um `...\index.js login` que não existe.
    detectMock.mockResolvedValue({ detectable: true, found: false, searched: [] });
    testMock.mockResolvedValue({ state: 'unauthenticated', agent_name: 'Claude Agent' });
    const user = userEvent.setup();

    render(<Host initialCommand="node" autoFill={false} agentId="claude-acp" />);
    await user.type(screen.getByLabelText(/argumentos/i), 'C:\\npm\\claude-agent-acp\\dist\\index.js');
    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/instalado, mas não está autenticado/i)).toBeInTheDocument();
    expect(screen.getByText(/não dá para saber daqui qual comando autentica/i)).toBeInTheDocument();
    expect(screen.queryByText(/index\.js login/)).not.toBeInTheDocument();
    expect(screen.queryByText(/cursor-agent login/)).not.toBeInTheDocument();
  });

  it('agente que não responde manda conferir comando e instalação, com o detalhe', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({
      state: 'offline',
      latency_ms: 30,
      error: 'executável não encontrado',
    });
    const user = userEvent.setup();

    render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/confira o comando e a instalação/i)).toBeInTheDocument();
    expect(screen.getByText(/executável não encontrado/i)).toBeInTheDocument();
    expect(screen.queryByText(/login$/)).not.toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/confira o comando e a instalação/i),
      'assertive',
    );
  });

  it('resultado não sobrevive à mudança do comando testado', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({ state: 'online', agent_name: 'Cursor', latency_ms: 10 });
    const user = userEvent.setup();

    render(<Host />);
    const commandInput = await screen.findByLabelText(/comando do agente/i);
    await waitFor(() => expect(commandInput).toHaveValue(cursorFound.command));

    await user.click(screen.getByRole('button', { name: /testar agente/i }));
    expect(await screen.findByText(/respondeu e aceitou abrir sessão/i)).toBeInTheDocument();

    await user.type(commandInput, '-outro');

    expect(screen.queryByText(/respondeu e aceitou abrir sessão/i)).not.toBeInTheDocument();
  });

  it('não anuncia resultado de configuração que a pessoa já trocou', async () => {
    // Os campos seguem editáveis enquanto a sonda roda. A tela já escondia o
    // resultado de outro comando, mas o anúncio saía: quem usa leitor de telas
    // ouviria "conectado" sobre o comando anterior.
    detectMock.mockResolvedValue(cursorFound);
    let responderTeste: (health: unknown) => void = () => {};
    testMock.mockReturnValue(new Promise((resolve) => { responderTeste = resolve; }));
    const user = userEvent.setup();

    render(<Host />);
    const commandInput = await screen.findByLabelText(/comando do agente/i);
    await waitFor(() => expect(commandInput).toHaveValue(cursorFound.command));

    await user.click(screen.getByRole('button', { name: /testar agente/i }));
    await user.type(commandInput, '-outro');
    announceMock.mockClear();

    await act(async () => {
      responderTeste({ state: 'online', agent_name: 'Cursor', latency_ms: 10 });
    });

    expect(announceMock).not.toHaveBeenCalled();
    expect(screen.queryByText(/respondeu e aceitou abrir sessão/i)).not.toBeInTheDocument();
  });

  it('sem comando, nem chama o backend: pede o comando e anuncia', async () => {
    detectMock.mockResolvedValue(cursorMissing);
    const user = userEvent.setup();

    render(<Host />);
    await screen.findByText(/agente não encontrado nesta máquina/i);

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/informe o comando do agente para testar/i)).toBeInTheDocument();
    expect(testMock).not.toHaveBeenCalled();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/informe o comando do agente para testar/i),
      'assertive',
    );
  });

  it('falha da própria sondagem aparece e é anunciada', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockRejectedValue(new Error('serviço de agentes de código não inicializado'));
    const user = userEvent.setup();

    render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });

    await user.click(screen.getByRole('button', { name: /testar agente/i }));

    expect(await screen.findByText(/serviço de agentes de código não inicializado/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('serviço de agentes de código não inicializado', 'assertive');
  });
});

// O agente que pede a credencial numa variável de ambiente diz o nome dela no
// handshake. A tela oferece esse nome preenchido; quem configura confirma, e é
// só a referência ao cofre que fica guardada (AEP-0086 D12).
describe('AgentProviderFields — credencial do cofre', () => {
  const cofreComOpenAI = [
    { pattern: 'api.openai.com', type: 'bearer', masked: 'sk-...4f2a', managed: false },
    { pattern: 'api.anthropic.com', type: 'bearer', masked: 'sk-ant-...9c1', managed: false },
  ];

  const agenteQuePedeChave = {
    state: 'unauthenticated',
    agent_name: 'Agente',
    login_methods: [
      {
        id: 'api_key',
        name: 'Chave de API',
        credential_provider: 'openai',
        env_vars: [
          { name: 'OPENAI_API_KEY', label: 'Chave da OpenAI', secret: true },
          { name: 'OPENAI_BASE_URL', optional: true },
        ],
      },
    ],
  };

  /** Deixa o componente pronto e testado, que é quando as sugestões chegam. */
  async function comAgenteTestado(props: Parameters<typeof Host>[0] = {}) {
    detectMock.mockResolvedValue(cursorFound);
    const user = userEvent.setup();
    const rendered = render(<Host {...props} />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    await user.click(screen.getByRole('button', { name: /testar agente/i }));
    return { user, ...rendered };
  }

  it('diz o que entregar a credencial ao agente implica, antes de entregar', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    await comAgenteTestado();

    const bloco = screen.getByRole('group', { name: /credencial do cofre para o agente/i });
    // O aviso é sobre o que a decisão custa: o agente é programa de terceiro e
    // recebe o valor inteiro. Ele fica ligado ao botão que liga a passagem,
    // para quem navega por teclado ouvi-lo ao chegar nele.
    expect(bloco).toHaveTextContent(/programa de terceiros/i);
    const botao = screen.getByRole('button', { name: /ligar a credencial/i });
    const aviso = document.getElementById(botao.getAttribute('aria-describedby') || '');
    expect(aviso).toHaveTextContent(/pode usar essa credencial como quiser/i);
  });

  it('oferece a variável que o agente pediu e a entrada do cofre que combina', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    await comAgenteTestado();

    // A variável vem do agente, e a tela diz de onde ela veio: quem configura
    // precisa saber que aquilo é informação publicada, e não palpite do app.
    await waitFor(() => {
      expect(screen.getByLabelText(/variável de ambiente/i)).toHaveValue('OPENAI_API_KEY');
    });
    expect(screen.getByText(/o próprio agente informou o nome desta variável/i)).toBeInTheDocument();
    // A entrada do cofre é só pré-escolhida a partir do emissor que o agente
    // nomeou; ligar sozinho mandaria a chave sem ninguém ter dito que podia.
    expect(screen.getByLabelText(/entrada do cofre/i)).toHaveValue('api.openai.com');
  });

  it('a variável que não é segredo não é oferecida ao cofre', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue({
      ...agenteQuePedeChave,
      login_methods: [
        {
          ...agenteQuePedeChave.login_methods[0],
          env_vars: [{ name: 'OPENAI_BASE_URL', optional: true }],
        },
      ],
    });

    await comAgenteTestado();

    // Uma URL de base não sai do cofre: oferecê-la aqui usaria o caminho das
    // chaves para o que nem segredo é.
    await waitFor(() => {
      expect(screen.getByText(/instalado, mas não está autenticado/i)).toBeInTheDocument();
    });
    expect(screen.getByLabelText(/variável de ambiente/i)).toHaveValue('');
  });

  it('liga o par, anuncia quando ele passa a valer e o entrega ao formulário', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    const { user } = await comAgenteTestado();
    await waitFor(() => {
      expect(screen.getByLabelText(/variável de ambiente/i)).toHaveValue('OPENAI_API_KEY');
    });

    await user.click(screen.getByRole('button', { name: /ligar a credencial/i }));

    expect(screen.getByTestId('cofre-do-pai')).toHaveTextContent(
      JSON.stringify({ OPENAI_API_KEY: 'api.openai.com' }),
    );
    expect(screen.getByText(/OPENAI_API_KEY recebe a credencial de api\.openai\.com/)).toBeInTheDocument();
    // Vale no próximo start: o processo que já está de pé subiu com o ambiente
    // de antes, e não dizer isso deixaria alguém esperando efeito imediato.
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/próxima vez que o agente subir/i),
      'polite',
    );
  });

  it('recusa nome de variável que não sobrevive à passagem, sem chamar o backend', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    const { user } = await comAgenteTestado();
    const campo = await screen.findByLabelText(/variável de ambiente/i);
    await user.clear(campo);
    // Espaço no nome não atravessa: o par que o sistema operacional monta se
    // parte. Dizer isso aqui, ao lado do campo, é melhor do que deixar o erro
    // voltar do salvamento junto com tudo o mais que o formulário mandou.
    await user.type(campo, 'CHAVE DA OPENAI');
    await user.click(screen.getByRole('button', { name: /ligar a credencial/i }));

    expect(screen.getByText(/sem espaços e sem o sinal de igual/i)).toBeInTheDocument();
    expect(screen.getByTestId('cofre-do-pai')).toHaveTextContent('{}');
  });

  it('desliga a passagem tirando o par que estava ligado', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    const { user } = await comAgenteTestado({
      initialCredentialEnv: { OPENAI_API_KEY: 'api.openai.com' },
    });

    await user.click(
      await screen.findByRole('button', { name: /remover a credencial da variável OPENAI_API_KEY/i }),
    );

    expect(screen.getByTestId('cofre-do-pai')).toHaveTextContent('{}');
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/não recebe mais credencial do cofre/i),
      'polite',
    );
  });

  it('sem credencial cadastrada, diz onde cadastrar em vez de oferecer um seletor vazio', async () => {
    listCredentialsMock.mockResolvedValue([]);
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    expect(await screen.findByText(/não há credenciais cadastradas/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /ligar a credencial/i })).not.toBeInTheDocument();
  });

  it('cofre que não respondeu diz o motivo e não deixa ninguém preso num seletor sem opção', async () => {
    listCredentialsMock.mockRejectedValue(new Error('cofre trancado'));
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    // Sem a lista não há entrada para escolher, e um seletor vazio ao lado de um
    // botão que só sabe recusar deixaria a pessoa tentando o impossível.
    expect(await screen.findByText(/cofre trancado/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /ligar a credencial/i })).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/entrada do cofre/i)).not.toBeInTheDocument();
  });

  it('a sugestão do agente não volta por cima do campo que a pessoa apagou', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    const { user } = await comAgenteTestado();
    const campo = await screen.findByLabelText(/variável de ambiente/i);
    await waitFor(() => expect(campo).toHaveValue('OPENAI_API_KEY'));

    await user.clear(campo);
    // Trocar a entrada repinta o bloco. A sugestão já foi aplicada uma vez, e
    // reaplicá-la aqui desfaria o que a pessoa acabou de apagar.
    await user.selectOptions(screen.getByLabelText(/entrada do cofre/i), 'api.anthropic.com');

    expect(campo).toHaveValue('');
  });

  it('não tem violação de acessibilidade com a passagem ligada', async () => {
    listCredentialsMock.mockResolvedValue(cofreComOpenAI);
    testMock.mockResolvedValue(agenteQuePedeChave);

    const { container } = await comAgenteTestado({
      initialCredentialEnv: { OPENAI_API_KEY: 'api.openai.com' },
    });
    await screen.findByText(/OPENAI_API_KEY recebe a credencial de api\.openai\.com/);

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe('AgentProviderFields — acessibilidade', () => {
  it('não tem violação de acessibilidade com o agente encontrado', async () => {
    detectMock.mockResolvedValue(cursorFound);

    const { container } = render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });

    expect(await axe(container)).toHaveNoViolations();
  });

  it('não tem violação de acessibilidade no estado sem agente', async () => {
    detectMock.mockResolvedValue(cursorMissing);

    const { container } = render(<Host />);
    await screen.findByText(/agente não encontrado nesta máquina/i);

    expect(await axe(container)).toHaveNoViolations();
  });

  it('não tem violação de acessibilidade no estado sem login', async () => {
    detectMock.mockResolvedValue(cursorFound);
    testMock.mockResolvedValue({
      state: 'unauthenticated',
      agent_name: 'Cursor',
      login_methods: [{ id: 'cursor_login', name: 'Entrar no Cursor' }],
    });
    const user = userEvent.setup();

    const { container } = render(<Host />);
    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    await user.click(screen.getByRole('button', { name: /testar agente/i }));
    await screen.findByText(/instalado, mas não está autenticado/i);

    expect(await axe(container)).toHaveNoViolations();
  });
});
