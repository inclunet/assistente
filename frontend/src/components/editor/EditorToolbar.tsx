import { type RefObject, useId } from 'react';
import { useTranslation } from 'react-i18next';
import { CompassOutlined, DownOutlined, FileOutlined, FullscreenOutlined, PlusOutlined, SlidersOutlined } from '@ant-design/icons';

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
  revealSlidePicker?: {
    enabled: boolean;
    slideCount: number;
    currentSlideIndex: number;
    slideLabels?: Array<string | undefined>;
    onSelectSlide: (index: number) => void;
    onCreateSlide: () => void;
  };
  revealFullscreen?: {
    enabled: boolean;
    onRequest: () => void;
  };
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
  revealSlidePicker,
  revealFullscreen,
}: EditorToolbarProps) {
  const { t } = useTranslation();
  const slidePickerMenuLabelId = useId();
  const showRevealSlidePicker = !!revealSlidePicker?.enabled && revealSlidePicker.slideCount > 0;
  const showRevealFullscreen = !!revealFullscreen?.enabled;
  const getRevealSlideLabel = (index: number) =>
    revealSlidePicker?.slideLabels?.[index] || t('editor.presentation.slideOption', { index: index + 1 });
  const revealSlideMenuItems: MenuItem[] = showRevealSlidePicker
    ? [
        ...Array.from({ length: revealSlidePicker.slideCount }, (_, index) => ({
          id: `reveal-slide-${index}`,
          label: getRevealSlideLabel(index),
          checked: index === revealSlidePicker.currentSlideIndex,
          action: () => revealSlidePicker.onSelectSlide(index),
        })),
        { id: 'reveal-slide-separator', separator: true },
        {
          id: 'reveal-slide-new',
          label: t('editor.presentation.newSlide'),
          action: revealSlidePicker.onCreateSlide,
        },
      ]
    : [];
  const revealSlidePickerControl = showRevealSlidePicker ? (
    <div className="editor-page__toolbar-presentation" role="group" aria-labelledby={slidePickerMenuLabelId}>
      <span id={slidePickerMenuLabelId} className="editor-page__toolbar-presentation-label">
        {t('editor.presentation.slidePickerLabel')}
      </span>
      <ToolbarButton
        label={getRevealSlideLabel(revealSlidePicker.currentSlideIndex)}
        endIcon={<DownOutlined />}
        disabled={isAsking}
        onClick={(e) => onOpenMenu(e.currentTarget, t('editor.presentation.goToSlide'), revealSlideMenuItems)}
        aria-haspopup="menu"
      />
    </div>
  ) : null;

  return (
    <Toolbar
      className="editor-page__toolbar ws-content-toolbar"
      left={
        <div className="editor-page__toolbar-left">
          <div className="editor-page__title">{activeTab?.title || t('editor.fallback.title')}</div>
        </div>
      }
      right={
        <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
          <ToolbarButton
            label={t('editor.buttons.file')}
            icon={<FileOutlined />}
            onClick={(e) => onOpenMenu(e.currentTarget, t('editor.aria.fileMenu'), fileMenuItems)}
            aria-haspopup="menu"
          />

          <ToolbarButton
            label={t('editor.buttons.format')}
            icon={<SlidersOutlined />}
            disabled={!activeTab || isAsking || activeTab.mode !== 'rich' || !richEditorRef.current}
            onClick={(e) => onOpenMenu(e.currentTarget, t('editor.aria.formatMenu'), formatMenuItems)}
            aria-haspopup="menu"
          />

          <ToolbarButton
            label={t('editor.buttons.insert')}
            icon={<PlusOutlined />}
            disabled={!activeTab || isAsking || activeTab.mode === 'view'}
            onClick={(e) => onOpenMenu(e.currentTarget, t('editor.aria.insertMenu'), insertMenuItems)}
            aria-haspopup="menu"
          />

          {revealSlidePickerControl}

          {showRevealFullscreen ? (
            <ToolbarButton
              label={t('editor.presentation.fullscreen')}
              icon={<FullscreenOutlined />}
              shortcut="F5"
              disabled={isAsking}
              onClick={revealFullscreen.onRequest}
            />
          ) : null}

          <ToolbarButton
            label={t('editor.buttons.mode')}
            icon={<CompassOutlined />}
            disabled={!activeTab || isAsking}
            onClick={(e) => onOpenMenu(e.currentTarget, t('editor.aria.modeMenu'), modeMenuItems)}
            aria-haspopup="menu"
          />
        </div>
      }
      actions={actions}
      ariaLabel={t('editor.aria.toolbar')}
    />
  );
}
