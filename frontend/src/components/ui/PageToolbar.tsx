import { ReactNode } from 'react';
import { Toolbar, ToolbarAction } from './Toolbar';
import './PageToolbar.css';

export interface PageToolbarProps {
  title: string;
  actions?: ToolbarAction[];
  searchValue?: string;
  onSearchChange?: (value: string) => void;
  searchPlaceholder?: string;
  children?: ReactNode;
  onFocusGrid?: (() => void) | null;
}

export function PageToolbar({
  title,
  actions = [],
  searchValue = '',
  onSearchChange,
  searchPlaceholder = 'Buscar...',
  children,
  onFocusGrid,
}: PageToolbarProps) {
  return (
    <Toolbar
      left={<h1 className="page-toolbar__title">{title}</h1>}
      center={children}
      searchValue={searchValue}
      onSearchChange={onSearchChange}
      searchPlaceholder={searchPlaceholder}
      actions={actions}
      onFocusGrid={onFocusGrid}
    />
  );
}
