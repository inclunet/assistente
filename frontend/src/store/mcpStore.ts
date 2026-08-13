import { logger } from '../utils/logger';
import { create } from 'zustand';
import {
  ListMCPServers,
  ConnectMCPServer,
  DisconnectMCPServer,
  ReconnectMCPServer,
  SaveMCPServer,
  DeleteMCPServer,
  GetMCPServerTools,
  GetMCPServerConfig,
} from '@wailsjs/go/wailsapi/MCP';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { mcp } from '../../wailsjs/go/models';

type ServerInfo = mcp.ServerInfo;
type ServerConfig = mcp.ServerConfig;
type MCPToolInfo = mcp.MCPToolInfo;

interface MCPState {
  servers: ServerInfo[];
  isLoading: boolean;
  activeServerSlug: string | null;
  editingConfig: ServerConfig | null;

  // Actions
  loadServers: () => Promise<void>;
  connect: (slug: string) => Promise<void>;
  disconnect: (slug: string) => Promise<void>;
  reconnect: (slug: string) => Promise<void>;
  save: (slug: string, config: ServerConfig) => Promise<void>;
  remove: (slug: string) => Promise<void>;
  getTools: (slug: string) => Promise<MCPToolInfo[]>;
  getConfig: (slug: string) => Promise<ServerConfig | null>;
  setActiveServer: (slug: string | null) => void;
  setEditingConfig: (config: ServerConfig | null) => void;
  setupEventListeners: () => () => void;
}

export const useMCPStore = create<MCPState>((set, get) => ({
  servers: [],
  isLoading: false,
  activeServerSlug: null,
  editingConfig: null,

  loadServers: async () => {
    set({ isLoading: true });
    try {
      const servers = await ListMCPServers();
      set({ servers: servers || [], isLoading: false });
    } catch (err) {
      logger.error('[MCP] Erro ao carregar servidores:', err);
      set({ isLoading: false });
    }
  },

  connect: async (slug: string) => {
    try {
      await ConnectMCPServer(slug);
      // Recarrega para obter status atualizado
      await get().loadServers();
    } catch (err) {
      logger.error(`[MCP] Erro ao conectar '${slug}':`, err);
      await get().loadServers();
    }
  },

  disconnect: async (slug: string) => {
    try {
      await DisconnectMCPServer(slug);
      await get().loadServers();
    } catch (err) {
      logger.error(`[MCP] Erro ao desconectar '${slug}':`, err);
      await get().loadServers();
    }
  },

  reconnect: async (slug: string) => {
    try {
      await ReconnectMCPServer(slug);
      await get().loadServers();
    } catch (err) {
      logger.error(`[MCP] Erro ao reconectar '${slug}':`, err);
      await get().loadServers();
    }
  },

  save: async (slug: string, config: ServerConfig) => {
    try {
      await SaveMCPServer(slug, config);
      await get().loadServers();
    } catch (err) {
      logger.error(`[MCP] Erro ao salvar '${slug}':`, err);
      throw err;
    }
  },

  remove: async (slug: string) => {
    try {
      await DeleteMCPServer(slug);
      const state = get();
      if (state.activeServerSlug === slug) {
        set({ activeServerSlug: null });
      }
      await get().loadServers();
    } catch (err) {
      logger.error(`[MCP] Erro ao remover '${slug}':`, err);
      throw err;
    }
  },

  getTools: async (slug: string) => {
    try {
      return await GetMCPServerTools(slug) || [];
    } catch (err) {
      logger.error(`[MCP] Erro ao obter tools de '${slug}':`, err);
      return [];
    }
  },

  getConfig: async (slug: string) => {
    try {
      return await GetMCPServerConfig(slug);
    } catch (err) {
      logger.error(`[MCP] Erro ao obter config de '${slug}':`, err);
      return null;
    }
  },

  setActiveServer: (slug: string | null) => {
    set({ activeServerSlug: slug });
  },

  setEditingConfig: (config: ServerConfig | null) => {
    set({ editingConfig: config });
  },

  setupEventListeners: () => {
    const unsubs: Array<() => void> = [];

    // Servidor conectado
    unsubs.push(EventsOn('mcp:server_connected', () => {
      get().loadServers();
    }));

    // Servidor desconectado
    unsubs.push(EventsOn('mcp:server_disconnected', () => {
      get().loadServers();
    }));

    // Erro no servidor
    unsubs.push(EventsOn('mcp:server_error', () => {
      get().loadServers();
    }));

    // Config alterada
    unsubs.push(EventsOn('mcp:config_changed', () => {
      get().loadServers();
    }));

    // Servidor conectando
    unsubs.push(EventsOn('mcp:server_connecting', () => {
      get().loadServers();
    }));

    // Servidor detectado como não-saudável (health check ou tool call falhou)
    unsubs.push(EventsOn('mcp:server_unhealthy', () => {
      get().loadServers();
    }));

    // Tools/resources/prompts mudaram (refresh periódico ou reconexão)
    unsubs.push(EventsOn('mcp:tools_changed', () => {
      get().loadServers();
    }));

    return () => {
      unsubs.forEach(fn => fn());
    };
  },
}));
