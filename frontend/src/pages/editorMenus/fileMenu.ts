import type { MenuItem } from '../../components/menu';
import type { FileMenuContext } from './menuContext';

export function buildFileMenuItemsForContextMenu(args: { ctx: FileMenuContext }): MenuItem[] {
  const { ctx } = args;

  return ctx.fileMenuItems.map((it) => ({
    id: `editor-toolbar-file-${it.value}`,
    label: it.label,
    shortcut: it.sublabel,
    disabled: !!it.disabled,
    action: () => {
      void ctx.onSelect(String(it.value || ''));
    },
    ariaLabel: it.sublabel ? `${it.label}, ${it.sublabel}` : it.label,
  }));
}
