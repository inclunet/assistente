/**
 * Chat Navigation Context
 * 
 * Sistema de navegação interna do chat usando Svelte Context API.
 * Permite que componentes (mesmo em slots) se comuniquem sobre foco/navegação
 * sem depender de DOM global.
 * 
 * Uso:
 * - ChatContainer: setContext('chat-navigation', createNavigationStore())
 * - ChatInput/ChatHistory: getContext('chat-navigation')
 */

import { writable } from 'svelte/store';

export const CHAT_NAVIGATION_KEY = 'chat-navigation';

/**
 * Cria a store de navegação do chat
 * @returns {import('svelte/store').Writable}
 */
export function createNavigationStore() {
  const store = writable({
    // Alvo de foco: 'input' | 'firstMessage' | 'lastMessage' | null
    focusTarget: null,
    
    // Índice da mensagem focada (-1 = nenhuma)
    focusedIndex: -1,
    
    // Dados extras para o foco
    focusData: null,
  });
  
  return {
    subscribe: store.subscribe,
    set: store.set,
    update: store.update,
    
    /**
     * Solicita foco no campo de input
     */
    focusInput() {
      store.update(s => ({ ...s, focusTarget: 'input', focusData: null }));
    },
    
    /**
     * Solicita foco na primeira mensagem
     */
    focusFirstMessage() {
      store.update(s => ({ ...s, focusTarget: 'firstMessage', focusData: null }));
    },
    
    /**
     * Solicita foco na última mensagem
     */
    focusLastMessage() {
      store.update(s => ({ ...s, focusTarget: 'lastMessage', focusData: null }));
    },
    
    /**
     * Solicita foco em uma mensagem específica por índice
     * @param {number} index 
     */
    focusMessage(index) {
      store.update(s => ({ 
        ...s, 
        focusTarget: 'message', 
        focusedIndex: index,
        focusData: { index } 
      }));
    },
    
    /**
     * Limpa o alvo de foco (após processar)
     */
    clearFocusTarget() {
      store.update(s => ({ ...s, focusTarget: null, focusData: null }));
    },
    
    /**
     * Atualiza o índice focado (para tracking)
     * @param {number} index 
     */
    setFocusedIndex(index) {
      store.update(s => ({ ...s, focusedIndex: index }));
    },
  };
}

/**
 * Tipo da store de navegação (para TypeScript/JSDoc)
 * @typedef {ReturnType<typeof createNavigationStore>} NavigationStore
 */





