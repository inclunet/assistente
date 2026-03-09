import { useState, useEffect } from 'react';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { GetAvailableTools, GetAllowlists, GetSkills } from '@wailsjs/go/main/App';
import { main, allowlist, skills } from '../../wailsjs/go/models';

export interface ProfileDependencies {
  tools: main.ToolInfo[];
  skills: skills.SkillInfo[];
  allowlists: allowlist.AllowlistInfo[];
  loading: boolean;
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
  const [tools, setTools] = useState<main.ToolInfo[]>([]);
  const [skills, setSkills] = useState<skills.SkillInfo[]>([]);
  const [allowlists, setAllowlists] = useState<allowlist.AllowlistInfo[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadDependencies = async () => {
      setLoading(true);
      try {
        const [toolsData, allowlistsData, skillsData] = await Promise.all([
          GetAvailableTools(),
          GetAllowlists(),
          GetSkills(),
        ]);
        setTools(toolsData || []);
        setAllowlists(allowlistsData || []);
        setSkills(skillsData || []);
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

  return { tools, skills, allowlists, loading };
}
