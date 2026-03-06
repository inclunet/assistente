import React from 'react';
import type { ContextMenuProps } from './types';
import { Menu } from './Menu';

// Variação semântica: ContextMenu deriva do Menu e mantém o default aria-label anterior.
export const ContextMenu: React.FC<ContextMenuProps> = (props) => {
  return <Menu {...props} ariaLabel={props.ariaLabel ?? 'Menu de contexto'} />;
};
