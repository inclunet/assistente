/**
 * Chat Module - Serviços de chat
 * 
 * ARQUITETURA V2:
 *   - createChatStores: Factory de stores isoladas (top-level, compatível com Svelte)
 *   - MessageController: Controller stateless para operações de chat
 *   - Utilities: Funções helper para processamento de mensagens
 * 
 * Uso:
 *   const stores = createChatStores();
 *   const controller = new MessageController(stores, 'my-instance-id');
 *   await controller.init();
 *   controller.bindBackendEvents();
 * 
 *   // Em componente Svelte:
 *   $: console.log('Messages:', $stores.messages);
 */

// === STORES ===
export { createChatStores } from './stores.js';

// === CONTROLLER ===
export { MessageController } from './message-controller.js';

// === UTILITIES ===
export { 
  parseToolCalls, 
  formatAgentName, 
  convertMessageNode,
  extractMessagesFromThreads,
  normalizeThreads
} from './utils.js';



