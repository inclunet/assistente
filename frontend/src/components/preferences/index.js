// ====================
// Preferences Components
// ====================
// 
// Componentes de preferências reutilizáveis:
// - ChatPreferences: Preferências do chat com guias (Chat, Voz, Transcrição)
// 
// Uso:
//   import { ChatPreferences } from './components/preferences';
//

export { default as ChatPreferences } from './ChatPreferences.svelte';

// Re-export de constantes úteis dos pickers
export { VOICE_DISABLED, STT_WEBSPEECH, STT_WHISPER } from '../pickers';

