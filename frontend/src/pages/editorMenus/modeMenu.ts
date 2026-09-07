import i18next from 'i18next';
import type { MenuItem } from '../../components/menu';
import type { EditorMode } from '../../store/editorStore';
import type { ModeMenuContext } from './menuContext';

export function buildModeMenuItemsForContextMenu(args: { ctx: ModeMenuContext }): MenuItem[] {
  const { ctx } = args;

  const canChange = !!ctx.activeTab && !ctx.isAsking;
  const current = ctx.activeTab?.mode || 'markdown';

  const mk = (mode: EditorMode, label: string, shortcut: string): MenuItem => ({
    id: `mode-${mode}`,
    label,
    shortcut,
    icon: current === mode ? '✓' : ' ',
    disabled: !canChange,
    action: () => ctx.setActiveTabMode(mode),
  });

  return [
    mk('markdown', i18next.t('editor.modes.markdown'), 'Alt+1'),
    mk('rich', i18next.t('editor.modes.rich'), 'Alt+2'),
    mk('view', i18next.t('editor.modes.view'), 'Alt+3'),
  ];
}
