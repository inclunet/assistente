// ====================
// Chat Components
// ====================
// Sistema de componentes modulares para chat, independente de backend.
// 
// Uso:
//   import { ChatContainer, MessageNode, ChatInput } from './components/chat';
//

// ========================================
// Core Components - Visualização Pura
// ========================================

// Message components
export { default as MessageNode } from './core/message/MessageNode.svelte';
export { default as MessageContent } from './core/message/MessageContent.svelte';
export { default as MessageActions } from './core/message/MessageActions.svelte';
export { default as MessageAvatar } from './core/message/MessageAvatar.svelte';
export { default as MessageHeader } from './core/message/MessageHeader.svelte';
// MessageContextMenu removido - use o ContextMenu do projeto com getMessageMenuItems()
export { default as ThreadIndicator } from './core/message/ThreadIndicator.svelte';

// Input components
export { default as ChatInput } from './core/input/ChatInput.svelte';
export { default as MediaPreview } from './core/input/MediaPreview.svelte';
export { default as MediaPicker } from './core/input/MediaPicker.svelte';

// Input buttons (use no slot input-buttons)
export { default as SendButton } from './core/input/SendButton.svelte';
export { default as VoiceRecordButton } from './core/input/VoiceRecordButton.svelte';
export { default as MediaButton } from './core/input/MediaButton.svelte';

// Context API for internal navigation
export { 
  createNavigationStore, 
  CHAT_NAVIGATION_KEY 
} from './context/navigation.js';

// Feedback components
export { default as TypingIndicator } from './core/feedback/TypingIndicator.svelte';
export { default as EmptyState } from './core/feedback/EmptyState.svelte';
export { default as LoadingIndicator } from './core/feedback/LoadingIndicator.svelte';
export { default as StreamingIndicator } from './core/feedback/StreamingIndicator.svelte';

// Main components
export { default as ChatHistory } from './core/ChatHistory.svelte';

// NOTA: Modais e Toolbar foram removidos.
// Use seus próprios componentes de modal e toolbar.
// Veja o README para exemplos.

// ========================================
// Wrappers - Componentes com Lógica
// ========================================

export { default as ChatContainer } from './wrappers/ChatContainer.svelte';

// ========================================
// Theming - CSS Custom Properties
// ========================================
// 
// The theming system uses CSS custom properties (--chat-*).
// Import the base tokens and optionally a theme or adapter:
// 
//   // Base tokens (required)
//   import '@chat-components/styles/tokens.css';
//   
//   // Optional: Theme
//   import '@chat-components/styles/themes/dark.css';
//   
//   // Optional: Adapter for your design system
//   import '@chat-components/adapters/your-project.css';
// 
// See: styles/tokens.css for all available tokens.

// ========================================
// i18n - svelte-i18n
// ========================================
// 
// Import directly from 'svelte-i18n':
//   import { _, locale } from 'svelte-i18n';
// 
// Translations are in: src/lib/locales/*.json

// ========================================
// Utils
// ========================================

export { 
  formatTimestamp,
  truncateText,
  generateMessageId,
  isImageMedia,
  isAudioMedia,
  isDocumentMedia
} from './utils/helpers.js';

// Menu items generators - use com seu ContextMenu
export {
  getMessageMenuItems,
  getCodeMenuItems,
  getTableMenuItems,
  getImageMenuItems,
  getLinkMenuItems,
  getLinksMenuItem,
  getImagesMenuItem,
  getCodeBlocksMenuItem,
  getTablesMenuItem
} from './utils/menuItems.js';

