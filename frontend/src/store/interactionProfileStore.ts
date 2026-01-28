/**
 * Interaction Profile Store
 * 
 * Gerencia os perfis de interação por voz.
 * Os perfis são globais (não vinculados a conversas específicas).
 * Cada perfil pode ter múltiplos triggers (hotkey, wakeword, button, vad).
 */

import { create } from 'zustand';
import { database } from '../../wailsjs/go/models';
import {
  GetInteractionProfiles,
  GetInteractionProfile,
  GetActiveInteractionProfile,
  CreateInteractionProfile,
  UpdateInteractionProfile,
  DeleteInteractionProfile,
  SetDefaultInteractionProfile,
  SetActiveInteractionProfile,
  GetTriggersByProfile,
  CreateInteractionTrigger,
  UpdateInteractionTrigger,
  DeleteInteractionTrigger,
} from '../../wailsjs/go/main/App';

// Re-exporta os tipos do Wails para uso externo
export type InteractionProfile = database.InteractionProfile;
export type InteractionTrigger = database.InteractionTrigger;

// Tipos de trigger
export type TriggerType = 'hotkey' | 'button_ptt' | 'button_toggle' | 'wakeword' | 'vad';

// Tipos de provider
export type STTProvider = 'webspeech' | 'whisper_api' | 'vosk';
export type WakeWordProvider = 'vosk' | 'webspeech';

// Constantes de tipos de trigger
export const TRIGGER_TYPES: { value: TriggerType; label: string; description: string }[] = [
  { value: 'hotkey', label: 'Hotkey', description: 'Atalho de teclado (toggle)' },
  { value: 'button_ptt', label: 'Botão PTT', description: 'Segura para gravar' },
  { value: 'button_toggle', label: 'Botão Toggle', description: 'Clica para alternar' },
  { value: 'wakeword', label: 'Wakeword', description: 'Palavra de ativação' },
  { value: 'vad', label: 'VAD', description: 'Detecção contínua de voz' },
];

// Constantes de providers STT
export const STT_PROVIDERS: { value: STTProvider; label: string; description: string }[] = [
  { value: 'webspeech', label: 'WebSpeech', description: 'API do navegador (online, gratuito)' },
  { value: 'whisper_api', label: 'Whisper API', description: 'OpenAI (online, melhor qualidade)' },
  { value: 'vosk', label: 'Vosk', description: 'Offline (privado, qualidade básica)' },
];

export interface InteractionProfileState {
  // Dados
  profiles: InteractionProfile[];
  activeProfileId: number | null;
  
  // Estados de UI
  isLoading: boolean;
  error: string | null;
  
  // Estados de runtime
  isListening: boolean; // wakeword/vad escutando
  isRecording: boolean;
  isProcessing: boolean;
  currentVolume: number;

  // Actions - Profile CRUD
  loadProfiles: () => Promise<void>;
  loadProfile: (id: number) => Promise<InteractionProfile | null>;
  createProfile: (profile: Partial<InteractionProfile>) => Promise<InteractionProfile | null>;
  updateProfile: (id: number, profile: Partial<InteractionProfile>) => Promise<InteractionProfile | null>;
  deleteProfile: (id: number) => Promise<boolean>;
  setDefaultProfile: (id: number) => Promise<boolean>;

  // Actions - Trigger CRUD
  loadTriggers: (profileId: number) => Promise<InteractionTrigger[]>;
  createTrigger: (trigger: Partial<InteractionTrigger>) => Promise<InteractionTrigger | null>;
  updateTrigger: (id: number, trigger: Partial<InteractionTrigger>) => Promise<InteractionTrigger | null>;
  deleteTrigger: (id: number) => Promise<boolean>;

  // Actions - Runtime
  setActiveProfile: (id: number) => Promise<void>;
  setListening: (listening: boolean) => void;
  setRecording: (recording: boolean) => void;
  setProcessing: (processing: boolean) => void;
  setVolume: (volume: number) => void;

  // Helpers
  getActiveProfile: () => InteractionProfile | null;
  getProfileTriggers: (profileId: number) => InteractionTrigger[];
  reset: () => void;
}

export const useInteractionProfileStore = create<InteractionProfileState>()(
  (set, get) => ({
      // Estado inicial
      profiles: [],
      activeProfileId: null,
      isLoading: false,
      error: null,
      isListening: false,
      isRecording: false,
      isProcessing: false,
      currentVolume: 0,

      // Carrega todos os perfis do backend (já inclui triggers)
      loadProfiles: async () => {
        const currentState = get();
        
        // Evita carregamentos duplicados
        if (currentState.isLoading) {
          console.log('[InteractionProfileStore] Já carregando, ignorando chamada duplicada');
          return;
        }
        
        set({ isLoading: true, error: null });
        
        try {
          const profiles = await GetInteractionProfiles();
          
          // Busca o perfil ativo do backend (persistido no banco)
          let activeProfile = await GetActiveInteractionProfile();
          
          // Se não há perfil ativo, usa o perfil padrão
          if (!activeProfile && profiles.length > 0) {
            const defaultProfile = profiles.find(p => p.is_default) || profiles[0];
            console.log('[InteractionProfileStore] Nenhum perfil ativo, usando padrão:', defaultProfile.name);
            activeProfile = defaultProfile;
            
            // Persiste no banco
            try {
              await SetActiveInteractionProfile(defaultProfile.id);
            } catch (err) {
              console.error('[InteractionProfileStore] Erro ao definir perfil padrão como ativo:', err);
            }
          }
          
          const activeId = activeProfile?.id || null;
          
          set({ 
            profiles, 
            activeProfileId: activeId,
            isLoading: false 
          });
          
          // Se há um perfil ativo e é diferente do atual, aplica-o (registra hotkeys no backend)
          if (activeId && activeId !== currentState.activeProfileId) {
            try {
              await SetActiveInteractionProfile(activeId);
              console.log('[InteractionProfileStore] Perfil ativo restaurado:', activeId);
            } catch (err) {
              console.error('[InteractionProfileStore] Erro ao restaurar perfil ativo:', err);
            }
          }
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao carregar perfis';
          set({ error: message, isLoading: false });
          console.error('[InteractionProfileStore] Erro:', error);
        }
      },

      // Carrega um perfil específico com triggers
      loadProfile: async (id) => {
        try {
          const profile = await GetInteractionProfile(id);
          
          // Atualiza na lista
          set(state => ({
            profiles: state.profiles.map(p => p.id === id ? profile : p),
          }));

          return profile;
        } catch (error) {
          console.error('[InteractionProfileStore] Erro ao carregar perfil:', error);
          return null;
        }
      },

      // Cria novo perfil
      createProfile: async (profile) => {
        set({ isLoading: true, error: null });
        
        try {
          const newProfile = new database.InteractionProfile({
            name: profile.name || 'Novo Perfil',
            description: profile.description || '',
            is_default: profile.is_default || false,
            stt_provider: profile.stt_provider || 'webspeech',
            language: profile.language || 'pt-BR',
            feedback_sounds: profile.feedback_sounds ?? true,
          });

          const created = await CreateInteractionProfile(newProfile);
          
          set(state => ({
            profiles: [...state.profiles, created],
            isLoading: false,
          }));

          return created;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao criar perfil';
          set({ error: message, isLoading: false });
          console.error('[InteractionProfileStore] Erro:', error);
          return null;
        }
      },

      // Atualiza perfil existente
      updateProfile: async (id, profile) => {
        set({ isLoading: true, error: null });
        
        try {
          const existing = get().profiles.find(p => p.id === id);
          if (!existing) {
            throw new Error('Perfil não encontrado');
          }

          const updatedProfile = new database.InteractionProfile({
            ...existing,
            ...profile,
            id,
            // Não envia triggers aqui - são gerenciados separadamente
            triggers: undefined,
          });

          const updated = await UpdateInteractionProfile(id, updatedProfile);
          
          set(state => ({
            profiles: state.profiles.map(p => p.id === id ? updated : p),
            isLoading: false,
          }));

          return updated;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao atualizar perfil';
          set({ error: message, isLoading: false });
          console.error('[InteractionProfileStore] Erro:', error);
          return null;
        }
      },

      // Deleta perfil
      deleteProfile: async (id) => {
        set({ isLoading: true, error: null });
        
        try {
          await DeleteInteractionProfile(id);
          
          set(state => {
            const newProfiles = state.profiles.filter(p => p.id !== id);
            const newActiveId = state.activeProfileId === id 
              ? (newProfiles.find(p => p.is_default)?.id || newProfiles[0]?.id || null)
              : state.activeProfileId;
            
            return {
              profiles: newProfiles,
              activeProfileId: newActiveId,
              isLoading: false,
            };
          });

          return true;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao deletar perfil';
          set({ error: message, isLoading: false });
          console.error('[InteractionProfileStore] Erro:', error);
          return false;
        }
      },

      // Define perfil como padrão
      setDefaultProfile: async (id) => {
        try {
          await SetDefaultInteractionProfile(id);
          
          // Recarrega os perfis para obter o estado atualizado
          const profiles = await GetInteractionProfiles();
          set({ profiles });

          return true;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao definir perfil padrão';
          set({ error: message });
          console.error('[InteractionProfileStore] Erro:', error);
          return false;
        }
      },

      // ==================== Trigger CRUD ====================

      // Carrega triggers de um perfil
      loadTriggers: async (profileId) => {
        try {
          const triggers = await GetTriggersByProfile(profileId);
          
          // Atualiza triggers no perfil correspondente
          set(state => ({
            profiles: state.profiles.map(p => {
              if (p.id === profileId) {
                // Atualiza triggers mantendo o objeto original
                p.triggers = triggers;
              }
              return p;
            }),
          }));

          return triggers;
        } catch (error) {
          console.error('[InteractionProfileStore] Erro ao carregar triggers:', error);
          return [];
        }
      },

      // Cria novo trigger
      createTrigger: async (trigger) => {
        try {
          const newTrigger = new database.InteractionTrigger({
            profile_id: trigger.profile_id,
            type: trigger.type || 'button_toggle',
            enabled: trigger.enabled ?? true,
            auto_stop: trigger.auto_stop ?? false,
            hotkey: trigger.hotkey || '',
            hotkey_global: trigger.hotkey_global ?? true,
            hotkey_bring_to_front: trigger.hotkey_bring_to_front ?? true,
            wakeword_keyword: trigger.wakeword_keyword || '',
            wakeword_provider: trigger.wakeword_provider || 'vosk',
            wakeword_sensitivity: trigger.wakeword_sensitivity ?? 0.5,
            vad_silence_threshold: trigger.vad_silence_threshold ?? 0.01,
            vad_silence_duration: trigger.vad_silence_duration ?? 1500,
            vad_activity_threshold: trigger.vad_activity_threshold ?? 0.02,
            vad_activity_duration: trigger.vad_activity_duration ?? 200,
          });

          const created = await CreateInteractionTrigger(newTrigger);
          
          // Recarrega o perfil para obter triggers atualizados
          if (created.profile_id) {
            await get().loadProfile(created.profile_id);
          }

          return created;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao criar trigger';
          set({ error: message });
          console.error('[InteractionProfileStore] Erro:', error);
          return null;
        }
      },

      // Atualiza trigger existente
      updateTrigger: async (id, trigger) => {
        try {
          // Busca trigger existente
          const profile = get().profiles.find(p => 
            p.triggers?.some(t => t.id === id)
          );
          const existing = profile?.triggers?.find(t => t.id === id);
          
          if (!existing) {
            throw new Error('Trigger não encontrado');
          }

          const updatedTrigger = new database.InteractionTrigger({
            ...existing,
            ...trigger,
            id,
          });

          const updated = await UpdateInteractionTrigger(id, updatedTrigger);
          
          // Recarrega o perfil para obter triggers atualizados
          if (updated.profile_id) {
            await get().loadProfile(updated.profile_id);
          }

          return updated;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao atualizar trigger';
          set({ error: message });
          console.error('[InteractionProfileStore] Erro:', error);
          return null;
        }
      },

      // Deleta trigger
      deleteTrigger: async (id) => {
        try {
          // Encontra o profile_id antes de deletar
          const profile = get().profiles.find(p => 
            p.triggers?.some(t => t.id === id)
          );
          const profileId = profile?.id;

          await DeleteInteractionTrigger(id);
          
          // Recarrega o perfil para obter triggers atualizados
          if (profileId) {
            await get().loadProfile(profileId);
          }

          return true;
        } catch (error) {
          const message = error instanceof Error ? error.message : 'Erro ao deletar trigger';
          set({ error: message });
          console.error('[InteractionProfileStore] Erro:', error);
          return false;
        }
      },

      // ==================== Runtime ====================

      // Define perfil ativo e registra hotkeys no backend
      setActiveProfile: async (id) => {
        const state = get();
        
        // Evita chamadas duplicadas para o mesmo perfil
        if (state.activeProfileId === id) {
          console.log('[InteractionProfileStore] Perfil já ativo:', id);
          return;
        }
        
        set({ activeProfileId: id });
        
        // Registra hotkeys do perfil no backend
        try {
          await SetActiveInteractionProfile(id);
          console.log('[InteractionProfileStore] Perfil ativado:', id);
        } catch (error) {
          console.error('[InteractionProfileStore] Erro ao ativar perfil no backend:', error);
          // Não reverte o estado local - hotkeys podem não estar disponíveis mas perfil ainda funciona
        }
      },

      setListening: (listening) => {
        set({ isListening: listening });
      },

      setRecording: (recording) => {
        set({ isRecording: recording });
      },

      setProcessing: (processing) => {
        set({ isProcessing: processing });
      },

      setVolume: (volume) => {
        set({ currentVolume: volume });
      },

      // ==================== Helpers ====================

      // Retorna o perfil ativo
      getActiveProfile: () => {
        const { profiles, activeProfileId } = get();
        return profiles.find(p => p.id === activeProfileId) || null;
      },

      // Retorna triggers de um perfil
      getProfileTriggers: (profileId) => {
        const profile = get().profiles.find(p => p.id === profileId);
        return profile?.triggers || [];
      },

      // Reset
      reset: () => {
        set({
          profiles: [],
          activeProfileId: null,
          isLoading: false,
          error: null,
          isListening: false,
          isRecording: false,
          isProcessing: false,
          currentVolume: 0,
        });
      },
    })
    // activeProfileId agora é persistido no banco de dados (não mais no localStorage)
);

export default useInteractionProfileStore;
