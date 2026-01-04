// ====================
// Modal Components
// ====================
// 
// Modais acessíveis com:
// - Focus trap (mantém foco dentro do modal)
// - Portal (renderiza no body, evita problemas de z-index e ARIA)
// - Escape para fechar
// - Restauração de foco ao elemento anterior
// - Auto-focus inteligente
// - Backdrop clicável
// 
// Componentes:
//   - Modal: Modal genérico com título e conteúdo customizável
//   - ImageModal: Modal otimizado para visualização de imagens
//   - ConfigModal: Modal de configurações com Aplicar/Salvar/Cancelar
// 
// Uso:
//   import { Modal, ImageModal, ConfigModal } from './components/modal';
//

export { default as Modal } from './Modal.svelte';
export { default as ImageModal } from './ImageModal.svelte';
export { default as ConfigModal } from './ConfigModal.svelte';


