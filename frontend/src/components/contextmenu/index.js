// ====================
// Context Menu Components
// ====================
// 
// Sistema de menu de contexto acessível com suporte a:
// - Navegação completa por teclado (↑↓←→, Home, End, Escape)
// - Submenus aninhados
// - Type-ahead (primeira letra)
// - Posicionamento automático
// - Restauração de foco
// - Anúncios para leitores de tela
// - Separadores, ícones, atalhos, estilo "danger"
// 
// Uso:
//   import { ContextMenu, ContextMenuTrigger } from './components/contextmenu';
//

export { default as ContextMenu } from './ContextMenu.svelte';
export { default as ContextMenuTrigger } from './ContextMenuTrigger.svelte';

// Re-export registerCloseAll para uso avançado
export { registerCloseAll } from './ContextMenu.svelte';


