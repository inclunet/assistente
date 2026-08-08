import { StrictMode, useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { AgentProviderFields, agentLoginCommand } from './AgentProviderFields';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());
const testMock = vi.hoisted(() => vi.fn());

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
}: {
  initialCommand?: string;
  autoFill?: boolean;
  agentId?: string;
}) => {
  const [command, setCommand] = useState(initialCommand);
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <span data-testid="args-atual">{args.join(',')}</span>
      <AgentProviderFields
        agentId={agentId}
        command={command}
        args={args}
        onCommandChange={setCommand}
        onArgsChange={setArgs}
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

  it('diz o que cada botão faz antes de alguém clicar nele', async () => {
    // Lado a lado e sem descrição, os dois pareciam duas formas de conferir a
    // instalação — e só um deles sobrescreve o que está nos campos.
    detectMock.mockResolvedValue(cursorFound);

    render(<Host />);

    const detectar = await screen.findByRole('button', { name: /detectar e preencher comando/i });
    expect(detectar).toHaveAccessibleDescription(
      /preenche o comando e os argumentos acima, substituindo/i,
    );

    const testar = screen.getByRole('button', { name: /testar agente/i });
    expect(testar).toHaveAccessibleDescription(/informa se ele respondeu.*não altera os campos/i);
  });

  it('não sobrescreve o comando já salvo ao abrir a edição', async () => {
    detectMock.mockResolvedValue(cursorFound);

    render(<Host initialCommand="/usr/local/bin/cursor-agent" autoFill={false} />);

    await waitFor(() => expect(detectMock).toHaveBeenCalled());
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
    responder(cursorFound);

    await waitFor(() => expect(screen.getByText(new RegExp(`versão ${cursorFound.version}`, 'i'))).toBeInTheDocument());
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

  it('detecção pedida no botão aplica o comando encontrado e anuncia', async () => {
    detectMock.mockResolvedValue(cursorFound);
    const user = userEvent.setup();

    render(<Host initialCommand="cursor-agent-antigo" autoFill={false} />);
    await waitFor(() => expect(detectMock).toHaveBeenCalledTimes(1));

    await user.click(screen.getByRole('button', { name: /detectar e preencher comando/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringContaining(cursorFound.command),
      'polite',
    );
  });
});

// O app roda dentro de `React.StrictMode`, que em desenvolvimento monta,
// desmonta e remonta cada componente. A remontagem precisa deixar os campos tão
// vivos quanto na primeira vez: o que sobrevive à desmontagem simulada e não é
// refeito na volta cala a tela para sempre.
describe('AgentProviderFields — remontado pelo StrictMode', () => {
  it('a procura que responde depois da remontagem ainda preenche e destrava o botão', async () => {
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
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /detectar e preencher comando/i })).toBeEnabled(),
    );
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
    expect(screen.getByText(/instale o cli do agente/i)).toBeInTheDocument();
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

    expect(await screen.findByText(/agente não encontrado nesta máquina/i)).toBeInTheDocument();
    expect(announceMock).not.toHaveBeenCalled();
    // O comando salvo continua onde estava, e o texto explica o que a máquina tem.
    expect(screen.getByLabelText(/comando do agente/i)).toHaveValue('/opt/cursor/agente');
  });

  it('alarma na detecção pedida, mesmo com comando salvo', async () => {
    // Aqui a pessoa pediu a procura: o resultado é a resposta a uma ação dela.
    detectMock.mockResolvedValue(cursorMissing);
    const user = userEvent.setup();

    render(<Host initialCommand="/opt/cursor/agente" autoFill={false} />);
    await screen.findByText(/agente não encontrado nesta máquina/i);

    await user.click(screen.getByRole('button', { name: /detectar e preencher comando/i }));

    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith(
        expect.stringMatching(/instale o cli do agente ou informe o comando manualmente/i),
        'assertive',
      );
    });
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

  it('procura que quebrou continua tendo como ser tentada de novo', async () => {
    // A falha zera o que se sabia do agente, inclusive se o app sabe procurá-lo.
    // Esconder o botão nesse estado transformaria um erro passageiro em beco sem
    // saída: só recarregando a tela para tentar outra vez.
    detectMock.mockRejectedValueOnce(new Error('acesso negado ao diretório'));
    const user = userEvent.setup();

    render(<Host />);
    await screen.findByText(/acesso negado ao diretório/i);

    detectMock.mockResolvedValue(cursorFound);
    await user.click(screen.getByRole('button', { name: /detectar e preencher comando/i }));

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
