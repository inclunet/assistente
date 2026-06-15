import { type RefObject } from 'react';
import { useTranslation } from 'react-i18next';
import { CompassOutlined, FileOutlined, PlusOutlined, SlidersOutlined } from '@ant-design/icons';

import { Toolbar, ToolbarButton, type ToolbarAction } from '../ui/Toolbar';
import type { MenuItem } from '../menu';
import type { EditorDocument } from '../../store/editorStore';
import type { TipTapEditor } from '../../pages/editorTypes';

export interface EditorToolbarProps {
  activeTab: EditorDocument | null;
  isAsking: boolean;
  richEditorRef: RefObject<TipTapEditor | null>;
  actions: ToolbarAction[];
  onOpenMenu: (anchor: HTMLElement, ariaLabel: string, items: MenuItem[]) => void;
  fileMenuItems: MenuItem[];
  formatMenuItems: MenuItem[];
  insertMenuItems: MenuItem[];
  modeMenuItems: MenuItem[];
}

/** Barra de ferramentas do editor (Arquivo, Formatar, Inserir, Modo + ações). */
export function EditorToolbar({
  activeTab,
  isAsking,
  richEditorRef,
  actions,
  onOpenMenu,
  fileMenuItems,
  formatMenuItems,
  insertMenuItems,
  modeMenuItems,
}: EditorToolbarProps) {
  const { t } = useTranslation();

  return (
    <Toolbar
      className="editor-page__toolbar ws-content-toolbar"
      left={<div className="editor-page__title">{activeTab?.title || t('editor.fallback.title')}</div>}
      right={
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <ToolbarButton
            label={t('editor.buttons.file')}
            icon={<FileOutlined />}
            onClick={(e) => onOpenMenu(e.currentTarget, 'Menu Arquivo', fileMenuItems)}
            aria-haspopup="menu"
          />

          <ToolbarButton
            label={t('editor.buttons.format')}
            icon={<SlidersOutlined />}
            disabled={!activeTab || isAsking || activeTab.mode !== 'rich' || !richEditorRef.current}
            onClick={(e) => onOpenMenu(e.currentTarget, 'Menu Formatar', formatMenuItems)}
            aria-haspopup="menu"
          />

          <ToolbarButton
            label={t('editor.buttons.insert')}
            icon={<PlusOutlined />}
            disabled={!activeTab || isAsking || activeTab.mode === 'view'}
            onClick={(e) => onOpenMenu(e.currentTarget, 'Menu Inserir', insertMenuItems)}
            aria-haspopup="menu"
          />

          <ToolbarButton
            label={t('editor.buttons.mode')}
            icon={<CompassOutlined />}
            disabled={!activeTab || isAsking}
            onClick={(e) => onOpenMenu(e.currentTarget, 'Menu Modo', modeMenuItems)}
            aria-haspopup="menu"
          />
        </div>
      }
      actions={actions}
      ariaLabel={t('editor.aria.toolbar')}
    />
  );
}
