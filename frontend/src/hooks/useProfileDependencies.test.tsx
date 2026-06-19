/** @vitest-environment jsdom */
import { describe, it, expect, beforeEach, vi } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';

import { useProfileDependencies } from './useProfileDependencies';

const getAvailableToolsMock = vi.fn();
const getAllowlistsMock = vi.fn();
const getSkillsMock = vi.fn();
const getContextProvidersMock = vi.fn();

let eventsHandler: (() => void) | null = null;
const unsubscribeMock = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetAvailableTools: (...args: unknown[]) => getAvailableToolsMock(...args),
  GetAllowlists: (...args: unknown[]) => getAllowlistsMock(...args),
  GetSkills: (...args: unknown[]) => getSkillsMock(...args),
  GetContextProviders: (...args: unknown[]) => getContextProvidersMock(...args),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (_event: string, handler: () => void) => {
    eventsHandler = handler;
    return unsubscribeMock;
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
    eventsHandler = null;
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

    eventsHandler?.();

    await waitFor(() => {
      expect(result.current.tools[0]?.name).toBe('Tool 2');
    });
  });

  it('desinscreve ao desmontar', () => {
    const { unmount } = renderHook(() => useProfileDependencies());
    unmount();
    expect(unsubscribeMock).toHaveBeenCalledTimes(1);
  });
});
