import type { MenuItem } from '../../components/menu';
import type { EditorMode } from '../../store/editorStore';
import type { ModeMenuContext } from './menuContext';

export function buildModeMenuItemsForContextMenu(args: { ctx: ModeMenuContext }): MenuItem[] {
  const { ctx } = args;

  const canChange = !!ctx.activeTab && !ctx.isAsking;
  const current = ctx.activeTab?.mode || 'markdown';

  const mk = (mode: EditorMode, label: string): MenuItem => ({
    id: `mode-${mode}`,
    label,
    icon: current === mode ? '✓' : ' ',
    disabled: !canChange,
    action: () => ctx.setActiveTabMode(mode),
  });

  return [mk('markdown', 'Código'), mk('rich', 'Rico'), mk('view', 'Visualização')];
}
