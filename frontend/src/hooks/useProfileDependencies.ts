import { useState, useEffect } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { GetAllowlists } from '@wailsjs/go/wailsapi/Allowlists';
import { GetContextProviders } from '@wailsjs/go/wailsapi/Profiles';
import { GetSelectableSkills } from '@wailsjs/go/wailsapi/Skills';
import { GetAvailableTools } from '@wailsjs/go/wailsapi/Tools';
import type { apidto, allowlist, contextprovider, skills } from '../../wailsjs/go/models';

export interface ProfileDependencies {
  tools: apidto.ToolInfo[];
  skills: skills.SkillInfo[];
  allowlists: allowlist.AllowlistInfo[];
  contextProviders: contextprovider.ProviderMetadata[];
  loading: boolean;
}

function fulfilledOrEmpty<T>(result: PromiseSettledResult<T[]>): T[] {
  return result.status === 'fulfilled' ? (result.value || []) : [];
}

/**
 * Hook para carregar dependências necessárias para editar perfis:
 * - Ferramentas disponíveis (MCP + builtin)
 * - Skills disponíveis
 * - Allowlists disponíveis
 *
 * Atualiza ferramentas MCP e skills selecionáveis enquanto o editor estiver aberto.
 */
export function useProfileDependencies(): ProfileDependencies {
  const [tools, setTools] = useState<apidto.ToolInfo[]>([]);
  const [skills, setSkills] = useState<skills.SkillInfo[]>([]);
  const [allowlists, setAllowlists] = useState<allowlist.AllowlistInfo[]>([]);
  const [contextProviders, setContextProviders] = useState<contextprovider.ProviderMetadata[]>([]);
  const [loadingDependencies, setLoadingDependencies] = useState(true);
  const [loadingSkills, setLoadingSkills] = useState(true);

  useEffect(() => {
    let mounted = true;
    let skillsRequest = 0;
    const loadSkills = async () => {
      const request = ++skillsRequest;
      let selectable: skills.SkillInfo[];
      try {
        selectable = (await GetSelectableSkills()) || [];
      } catch {
        selectable = [];
      }
      if (mounted && request === skillsRequest) {
        setSkills(selectable);
        setLoadingSkills(false);
      }
    };

    const unsubSkills = EventsOn('plugins:skills_changed', loadSkills);
    void loadSkills();

    void Promise.allSettled([GetAvailableTools(), GetAllowlists(), GetContextProviders()])
      .then(([toolsData, allowlistsData, providersData]) => {
        if (!mounted) return;
        setTools(fulfilledOrEmpty(toolsData));
        setAllowlists(fulfilledOrEmpty(allowlistsData));
        setContextProviders(fulfilledOrEmpty(providersData));
        setLoadingDependencies(false);
      });

    const unsubMCP = EventsOn('mcp:tools_changed', () => {
      GetAvailableTools().then(setTools).catch(() => {});
    });

    return () => {
      mounted = false;
      unsubSkills();
      unsubMCP();
    };
  }, []);

  return { tools, skills, allowlists, contextProviders, loading: loadingDependencies || loadingSkills };
}
