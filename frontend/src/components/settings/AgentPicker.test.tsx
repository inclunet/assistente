import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import ptBR from '../../locales/pt-BR';
import { axe } from '../../test/a11yAxe';
import type { CatalogAgent } from './ACPAgentCatalog';
import { AgentPicker } from './AgentPicker';

const catalogMock = vi.hoisted(() => vi.fn());

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
  useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn() }),
}));

vi.mock('@wailsjs/go/wailsapi/ACPRegistry', () => ({
  GetACPCatalog: catalogMock,
  RefreshACPCatalog: catalogMock,
}));

const catalogo = {
  agents: [
    {
      id: 'cursor',
      name: 'Cursor',
      distributions: ['binary'],
      runtime_found: true,
      integrity: 'digest',
      state: 'installed',
    },
    {
      id: 'gemini-cli',
      name: 'Gemini CLI',
      distributions: ['npm'],
      runtime: 'node',
      runtime_found: true,
      integrity: 'none',
      state: 'no_detection',
    },
  ],
  age_seconds: 0,
  from_cache: false,
  stale: false,
};

/**
 * Hospeda o seletor com o mesmo estado que o formulário do provedor dá a ele: o
 * agente escolhido é do formulário, e o seletor mostra o que ele guardou.
 */
const Host = ({ onPick }: { onPick?: (agent: CatalogAgent) => void }) => {
  const [agentId, setAgentId] = useState('');
  return (
    <AgentPicker
      agentId={agentId}
      onPick={(agent) => {
        setAgentId(agent.id);
        onPick?.(agent);
      }}
    />
  );
};

beforeEach(() => {
  catalogMock.mockResolvedValue(catalogo);
});

afterEach(() => {
  vi.clearAllMocks();
});

describe('AgentPicker', () => {
  it('usa o mesmo combobox inline dos outros pickers', async () => {
    render(<AgentPicker agentId="" onPick={() => {}} />);

    const button = await screen.findByRole('button', { name: /agente acp/i });
    expect(button).toHaveAttribute('aria-haspopup', 'listbox');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('escolher no picker devolve o agente e passa a mostrá-lo pelo nome', async () => {
    const onPick = vi.fn();
    const user = userEvent.setup();

    render(<Host onPick={onPick} />);
    await user.click(await screen.findByRole('button', { name: /agente acp/i }));

    await user.click(await screen.findByRole('option', { name: /gemini cli/i }));

    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: 'gemini-cli' }));
    expect(screen.getByRole('button', { name: /agente acp, gemini cli/i })).toBeInTheDocument();
  });

  it('agente salvo aparece pelo nome do catálogo', async () => {
    render(<AgentPicker agentId="cursor" onPick={() => {}} />);

    expect(await screen.findByRole('button', { name: /agente acp, cursor/i })).toBeInTheDocument();
  });

  it('catálogo fora do ar mostra erro e permite tentar novamente', async () => {
    catalogMock.mockRejectedValue(new Error('sem rede'));

    render(<AgentPicker agentId="cursor" onPick={() => {}} />);

    expect(await screen.findByText(/sem rede/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /atualizar catálogo/i })).toBeInTheDocument();
  });

  it('filtra também pelos metadados que não cabem na opção visual', async () => {
    const user = userEvent.setup();
    catalogMock.mockResolvedValue({
      ...catalogo,
      agents: [{
        ...catalogo.agents[0],
        authors: ['Equipe Exemplo'],
      }],
    });
    render(<AgentPicker agentId="" onPick={() => {}} />);

    await user.click(await screen.findByRole('button', { name: /agente acp/i }));
    await user.type(screen.getByRole('combobox'), 'Equipe Exemplo');
    expect(screen.getByRole('option', { name: /cursor/i })).toBeInTheDocument();
  });

  it('não tem violação de acessibilidade', async () => {
    const { container } = render(<AgentPicker agentId="cursor" onPick={() => {}} />);
    await screen.findByRole('button', { name: /agente acp, cursor/i });

    expect(await axe(container)).toHaveNoViolations();
  });
});
