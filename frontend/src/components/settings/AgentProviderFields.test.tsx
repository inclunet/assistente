import { useState } from 'react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import { AgentProviderFields } from './AgentProviderFields';

const announceMock = vi.hoisted(() => vi.fn());
const detectMock = vi.hoisted(() => vi.fn());

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
    <AgentProviderFields
      agentKind="cursor"
      command={command}
      args={args}
      onCommandChange={setCommand}
      onArgsChange={setArgs}
      autoFill={autoFill}
    />
  );
};

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

  it('anuncia a falha quando a própria procura quebra', async () => {
    detectMock.mockRejectedValue(new Error('acesso negado ao diretório'));

    render(<Host />);

    expect(await screen.findByText(/acesso negado ao diretório/i)).toBeInTheDocument();
    expect(announceMock).toHaveBeenCalledWith('acesso negado ao diretório', 'assertive');
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
});
