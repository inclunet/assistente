// Biblioteca de menus (nomenclatura padrão)
// Base única: `Menu` implementa navegação por teclado, submenus e separadores.
// `ContextMenu` é uma variação semântica (mesma UI; default aria-label de contexto).

export {
  ContextMenu,
  Menu,
} from '../ui/menu';

export type {
  MenuItem,
  ContextMenuProps,
  MenuProps,
} from '../ui/menu';
