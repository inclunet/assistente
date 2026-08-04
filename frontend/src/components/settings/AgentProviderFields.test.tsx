import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { AgentProviderFields } from './AgentProviderFields';

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
}));

const cursorFound = {
  found: true,
  command: 'C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\node.exe',
  args: ['C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\index.js', 'acp'],
  version: '2026.07.30-abc123',
  source: 'C:\\Users\\ana\\AppData\\Local\\cursor-agent\\versions\\2026.07.30-abc123\\index.js',
  searched: ['C:\\Users\\ana\\AppData\\Local\\cursor-agent'],
  work_dir: 'C:\\Users\\ana\\projetos\\assistente',
};

const cursorMissing = {
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
const Host = ({ initialCommand = '', autoFill = true }: { initialCommand?: string; autoFill?: boolean }) => {
  const [command, setCommand] = useState(initialCommand);
  const [args, setArgs] = useState<string[]>([]);
  return (
    <div>
      <span data-testid="args-atual">{args.join(',')}</span>
      <AgentProviderFields
        agentKind="cursor"
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
          agentKind="cursor"
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

    const workDir = await screen.findByLabelText(/diretório de trabalho/i);
    await waitFor(() => expect(workDir).toHaveValue(cursorFound.work_dir));
    expect(workDir).toHaveAttribute('readonly');
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

    await user.click(screen.getByRole('button', { name: /detectar instalação/i }));

    await waitFor(() => {
      expect(screen.getByLabelText(/comando do agente/i)).toHaveValue(cursorFound.command);
    });
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringContaining(cursorFound.command),
      'polite',
    );
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

    await user.click(screen.getByRole('button', { name: /detectar instalação/i }));

    await waitFor(() => {
      expect(announceMock).toHaveBeenCalledWith(
        expect.stringMatching(/instale o cli do agente ou informe o comando manualmente/i),
        'assertive',
      );
    });
  });

  it('anuncia a falha quando a própria procura quebra', async () => {
    detectMock.mockRejectedValue(new Error('acesso negado ao diretório'));

    render(<Host />);

    expect(await screen.findByText(/acesso negado ao diretório/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('acesso negado ao diretório', 'assertive');
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
    expect(screen.getByText('cursor-agent login')).toBeInTheDocument();
    expect(screen.getByText(/entrar no cursor/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith(
      expect.stringMatching(/instalado, mas não está autenticado/i),
      'assertive',
    );
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
    expect(screen.queryByText('cursor-agent login')).not.toBeInTheDocument();
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
