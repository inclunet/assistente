import { useState } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
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
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
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
  it('sem agente escolhido, diz isso e chama o botão de escolher', () => {
    render(<AgentPicker agentId="" onPick={() => {}} />);

    expect(screen.getByText(/nenhum agente escolhido/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /escolher agente no catálogo/i })).toBeInTheDocument();
  });

  it('escolher no catálogo devolve o agente e passa a mostrá-lo pelo nome', async () => {
    const onPick = vi.fn();
    const user = userEvent.setup();

    render(<Host onPick={onPick} />);
    await user.click(screen.getByRole('button', { name: /escolher agente no catálogo/i }));

    await user.click(await screen.findByRole('option', { name: /gemini cli/i }));

    expect(onPick).toHaveBeenCalledWith(expect.objectContaining({ id: 'gemini-cli' }));
    // O diálogo fecha e o escolhido fica escrito na tela, e não só na memória de
    // quem clicou.
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(screen.getByText(/gemini cli/i)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /trocar de agente/i })).toBeInTheDocument();
  });

  it('agente salvo aparece pelo nome do catálogo, e não pelo identificador cru', async () => {
    render(<AgentPicker agentId="cursor" onPick={() => {}} />);

    expect(await screen.findByText(/cursor/i)).toBeInTheDocument();
  });

  it('catálogo fora do ar não apaga o agente escolhido: sobra o identificador', async () => {
    // O nome é enfeite sobre o `id`. Sem rede, dizer "cursor" é pior do que
    // dizer "Cursor" e muito melhor do que deixar a linha em branco.
    catalogMock.mockRejectedValue(new Error('sem rede'));

    render(<AgentPicker agentId="cursor" onPick={() => {}} />);

    await waitFor(() => expect(catalogMock).toHaveBeenCalled());
    expect(screen.getByText(/cursor/i)).toBeInTheDocument();
  });

  it('o botão diz o que abrir o catálogo faz antes de alguém clicar nele', () => {
    render(<AgentPicker agentId="" onPick={() => {}} />);

    expect(screen.getByRole('button', { name: /escolher agente no catálogo/i })).toHaveAccessibleDescription(
      /trocar de agente limpa o comando/i,
    );
  });

  it('não tem violação de acessibilidade', async () => {
    const { container } = render(<AgentPicker agentId="cursor" onPick={() => {}} />);
    await waitFor(() => expect(catalogMock).toHaveBeenCalled());

    expect(await axe(container)).toHaveNoViolations();
  });
});
