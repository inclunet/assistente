/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { act, renderHook, waitFor } from '@testing-library/react';

import { useProfileDependencies } from './useProfileDependencies';

const getAvailableToolsMock = vi.fn();
const getAllowlistsMock = vi.fn();
const getSkillsMock = vi.fn();
const getContextProvidersMock = vi.fn();

const handlers = new Map<string, () => void>();
const unsubscribeMock = vi.fn();

vi.mock('@wailsjs/go/wailsapi/Tools', () => ({
  GetAvailableTools: (...args: unknown[]) => getAvailableToolsMock(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Allowlists', () => ({
  GetAllowlists: (...args: unknown[]) => getAllowlistsMock(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({
  GetContextProviders: (...args: unknown[]) => getContextProvidersMock(...args),
}));

vi.mock('@wailsjs/go/wailsapi/Skills', () => ({
  GetSelectableSkills: (...args: unknown[]) => getSkillsMock(...args),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (event: string, handler: () => void) => {
    handlers.set(event, handler);
    return () => {
      handlers.delete(event);
      unsubscribeMock(event);
    };
  },
}));

describe('useProfileDependencies', () => {
  beforeEach(() => {
    getAvailableToolsMock.mockReset();
    getAllowlistsMock.mockReset();
    getSkillsMock.mockReset();
    getContextProvidersMock.mockReset();
    getAvailableToolsMock.mockResolvedValue([{ name: 'Tool 1' }]);
    getAllowlistsMock.mockResolvedValue([{ slug: 'al-1', name: 'Allowlist 1', ruleCount: 2 }]);
    getSkillsMock.mockResolvedValue([{ slug: 'skill-1', name: 'Skill 1', description: '' }]);
    getContextProvidersMock.mockResolvedValue([{ name: 'memory', display_name: 'Memory' }]);
    handlers.clear();
    unsubscribeMock.mockClear();
  });

  it('carrega tools, skills, allowlists e context providers', async () => {
    const { result } = renderHook(() => useProfileDependencies());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tools).toHaveLength(1);
    expect(result.current.skills).toHaveLength(1);
    expect(result.current.allowlists).toHaveLength(1);
    expect(result.current.contextProviders).toHaveLength(1);
  });

  it('mantém dados disponíveis quando uma dependência falha', async () => {
    getContextProvidersMock.mockRejectedValueOnce(new Error('context providers unavailable'));

    const { result } = renderHook(() => useProfileDependencies());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.tools).toHaveLength(1);
    expect(result.current.skills).toHaveLength(1);
    expect(result.current.allowlists).toHaveLength(1);
    expect(result.current.contextProviders).toEqual([]);
  });

  it('atualiza tools quando recebe evento MCP', async () => {
    getAvailableToolsMock.mockResolvedValueOnce([{ name: 'Tool 1' }]);

    const { result } = renderHook(() => useProfileDependencies());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    getAvailableToolsMock.mockResolvedValueOnce([{ name: 'Tool 2' }]);

    handlers.get('mcp:tools_changed')?.();

    await waitFor(() => {
      expect(result.current.tools[0]?.name).toBe('Tool 2');
    });
  });

  it('atualiza a seleção do editor aberto ao ativar e desativar uma skill de plugin', async () => {
    const { result } = renderHook(() => useProfileDependencies());
    await waitFor(() => expect(result.current.loading).toBe(false));
    const local = result.current.skills[0];
    const plugin = { slug: 'plugin:nota', name: 'Nota' };

    getSkillsMock.mockResolvedValueOnce([local, plugin]);
    await act(async () => handlers.get('plugins:skills_changed')?.());
    expect(result.current.skills).toEqual([local, plugin]);

    getSkillsMock.mockResolvedValueOnce([local]);
    await act(async () => handlers.get('plugins:skills_changed')?.());
    expect(result.current.skills).toEqual([local]);
  });

  it('ignora a listagem inicial quando ela termina após uma atualização', async () => {
    let resolveInitial!: (skills: unknown[]) => void;
    getSkillsMock.mockReturnValueOnce(new Promise(resolve => { resolveInitial = resolve; }));
    const { result } = renderHook(() => useProfileDependencies());

    getSkillsMock.mockResolvedValueOnce([]);
    await act(async () => handlers.get('plugins:skills_changed')?.());
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => resolveInitial([{ slug: 'plugin:desativada' }]));
    expect(result.current.skills).toEqual([]);
  });

  it('desinscreve ao desmontar', () => {
    const { unmount } = renderHook(() => useProfileDependencies());
    unmount();
    expect(unsubscribeMock).toHaveBeenCalledWith('mcp:tools_changed');
    expect(unsubscribeMock).toHaveBeenCalledWith('plugins:skills_changed');
    expect(handlers.size).toBe(0);
  });
});
