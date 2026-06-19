import { useState, useEffect } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { GetAvailableTools, GetAllowlists, GetContextProviders, GetSkills } from '@wailsjs/go/app/App';
import type { controllers, allowlist, contextprovider, skills } from '../../wailsjs/go/models';

export interface ProfileDependencies {
  tools: controllers.ToolInfo[];
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
 * Também escuta eventos MCP para atualizar ferramentas dinamicamente.
 */
export function useProfileDependencies(): ProfileDependencies {
  const [tools, setTools] = useState<controllers.ToolInfo[]>([]);
  const [skills, setSkills] = useState<skills.SkillInfo[]>([]);
  const [allowlists, setAllowlists] = useState<allowlist.AllowlistInfo[]>([]);
  const [contextProviders, setContextProviders] = useState<contextprovider.ProviderMetadata[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadDependencies = async () => {
      setLoading(true);
      try {
        const [toolsData, allowlistsData, skillsData, contextProvidersData] = await Promise.allSettled([
          GetAvailableTools(),
          GetAllowlists(),
          GetSkills(),
          GetContextProviders(),
        ]);
        setTools(fulfilledOrEmpty(toolsData));
        setAllowlists(fulfilledOrEmpty(allowlistsData));
        setSkills(fulfilledOrEmpty(skillsData));
        setContextProviders(fulfilledOrEmpty(contextProvidersData));
      } finally {
        setLoading(false);
      }
    };

    loadDependencies();

    // Escuta mudanças nas tools MCP para atualizar a lista de ferramentas
    const unsubMCP = EventsOn('mcp:tools_changed', () => {
      GetAvailableTools().then(setTools).catch(() => {});
    });

    return () => {
      unsubMCP();
    };
  }, []);

  return { tools, skills, allowlists, contextProviders, loading };
}
