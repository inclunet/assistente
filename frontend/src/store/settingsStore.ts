import { create } from 'zustand';
import { persist } from 'zustand/middleware';
import { STT_WEBSPEECH } from '../components/pickers/STTProviderPicker';

export interface AppConfig {
  apiKey: string;
  baseURL: string;
  defaultModel: string;
  temperature: number;
  maxTokens: number;
  streamEnabled: boolean;
  theme: 'light' | 'dark' | 'system';
  language: string;
  voice?: string;
  voiceProfileId?: number;
  // Configurações separadas para assistente e usuário
  useAriaLiveForAgent?: boolean;  // Se deve usar aria-live para mensagens do assistente
  useAriaLiveForUser?: boolean;   // Se deve usar aria-live para mensagens do usuário
  ttsEnabledForUser?: boolean;    // Se TTS está habilitado para ler mensagens enviadas pelo usuário
  sttProvider?: string;
}

interface SettingsState {
  config: AppConfig | null;
  isLoading: boolean;
  error: string | null;
  
  // Actions
  setConfig: (config: AppConfig) => void;
  updateConfig: (partial: Partial<AppConfig>) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const defaultConfig: AppConfig = {
  apiKey: '',
  baseURL: 'https://api.openai.com/v1',
  defaultModel: 'gpt-4',
  temperature: 0.7,
  maxTokens: 2000,
  streamEnabled: true,
  theme: 'system',
  language: 'pt-BR',
  voice: undefined, // Gerenciado pelo perfil de voz
  voiceProfileId: undefined, // Usa perfil padrão do banco
  useAriaLiveForAgent: true, // Usa aria-live para assistente por padrão (TTS desativado)
  useAriaLiveForUser: true,  // Usa aria-live para usuário por padrão (TTS desativado)
  ttsEnabledForUser: false,  // TTS do usuário desativado por padrão
  sttProvider: STT_WEBSPEECH, // WebSpeech por padrão
};

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      config: null,
      isLoading: false,
      error: null,

      setConfig: (config) => set({ config, error: null }),
      
      updateConfig: (partial) =>
        set((state) => ({
          config: state.config ? { ...state.config, ...partial } : null,
        })),
      
      setLoading: (isLoading) => set({ isLoading }),
      
      setError: (error) => set({ error, isLoading: false }),
      
      reset: () => set({ config: defaultConfig, error: null, isLoading: false }),
    }),
    {
      name: 'assistente-settings',
      partialize: (state) => ({ config: state.config }),
    }
  )
);
