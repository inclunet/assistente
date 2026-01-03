// ====================
// Grid Components
// ====================
// 
// Componentes de grid acessíveis com:
// - Navegação completa por teclado
// - Seleção única e múltipla
// - Suporte a ARIA (role="grid", aria-selected, etc.)
// - Theming via CSS custom properties
// 
// Componentes:
//   - DataGrid: Grid tabular com colunas, edição inline
//   - CardGrid: Grid de cards responsivo com slots
// 
// Utilitários:
//   - gridUtils: Funções de seleção e navegação compartilhadas
// 
// Uso:
//   import { DataGrid, CardGrid } from './components/grid';
//

export { default as DataGrid } from './DataGrid.svelte';
export { default as CardGrid } from './CardGrid.svelte';

// Utilitários (para uso avançado ou extensão)
export {
  createSelectionManager,
  calculateLinearNavigation,
  calculate2DNavigation,
  handleCommonGridKeys,
  getGridPosition,
  clamp
} from './gridUtils.js';



