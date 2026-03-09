import type { MenuItem } from '../../components/menu';
import { createRichMenuActions } from './richMenuActions';
import type { FormatMenuContext } from './menuContext';

export function buildFormatMenuItemsForContextMenu(args: { ctx: FormatMenuContext }): MenuItem[] {
  const { ctx } = args;
  const { activeTab, isAsking, richEditorRef, richEditorHandleRef } = ctx;

  const richActions = createRichMenuActions({
    activeTab,
    isAsking,
    richEditorRef,
    requireTabMode: 'rich',
  });

  const canFormat = richActions.canUseRich;
  const rich = richActions.rich as any;
  const run = richActions.run;
  const canRun = richActions.canRun;

  const headingSubmenu: MenuItem[] = [
    {
      id: 'fmt-p',
      label: 'Parágrafo',
      icon: 'P',
      disabled: !canFormat,
      action: () => run((r) => r.chain?.().focus?.().setParagraph?.().run?.()),
    },
    ...[1, 2, 3, 4, 5, 6].map((lvl) => ({
      id: `fmt-h${lvl}`,
      label: `Título ${lvl} (H${lvl})`,
      icon: `H${lvl}`,
      disabled: !canFormat,
      action: () => run((r) => r.chain?.().focus?.().setHeading?.({ level: lvl })?.run?.()),
    })),
  ];

  const inTable = !!rich?.isActive?.('table');
  const tableSubmenu: MenuItem[] = [
    {
      id: 'fmt-table-row-before',
      label: 'Adicionar linha acima',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().addRowBefore?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().addRowBefore?.().run?.()),
    },
    {
      id: 'fmt-table-row-after',
      label: 'Adicionar linha abaixo',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().addRowAfter?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().addRowAfter?.().run?.()),
    },
    {
      id: 'fmt-table-del-row',
      label: 'Remover linha',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().deleteRow?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().deleteRow?.().run?.()),
    },
    { id: 'fmt-table-sep-1', separator: true },
    {
      id: 'fmt-table-col-before',
      label: 'Adicionar coluna antes',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().addColumnBefore?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().addColumnBefore?.().run?.()),
    },
    {
      id: 'fmt-table-col-after',
      label: 'Adicionar coluna depois',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().addColumnAfter?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().addColumnAfter?.().run?.()),
    },
    {
      id: 'fmt-table-del-col',
      label: 'Remover coluna',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().deleteColumn?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().deleteColumn?.().run?.()),
    },
    { id: 'fmt-table-sep-2', separator: true },
    {
      id: 'fmt-table-toggle-header-row',
      label: 'Alternar cabeçalho (linha)',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().toggleHeaderRow?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().toggleHeaderRow?.().run?.()),
    },
    {
      id: 'fmt-table-toggle-header-col',
      label: 'Alternar cabeçalho (coluna)',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().toggleHeaderColumn?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().toggleHeaderColumn?.().run?.()),
    },
    {
      id: 'fmt-table-toggle-header-cell',
      label: 'Alternar cabeçalho (célula)',
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().toggleHeaderCell?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().toggleHeaderCell?.().run?.()),
    },
    { id: 'fmt-table-sep-3', separator: true },
    {
      id: 'fmt-table-merge',
      label: 'Mesclar células',
      disabled: !inTable || !canRun((r) => (r as any).can?.().chain?.().focus?.().mergeCells?.().run?.()),
      action: () => run((r) => (r as any).chain?.().focus?.().mergeCells?.().run?.()),
    },
    {
      id: 'fmt-table-split',
      label: 'Separar célula',
      disabled: !inTable || !canRun((r) => (r as any).can?.().chain?.().focus?.().splitCell?.().run?.()),
      action: () => run((r) => (r as any).chain?.().focus?.().splitCell?.().run?.()),
    },
    { id: 'fmt-table-sep-4', separator: true },
    {
      id: 'fmt-table-prev-cell',
      label: 'Ir para célula anterior',
      disabled: !inTable || !canRun((r) => (r as any).can?.().chain?.().focus?.().goToPreviousCell?.().run?.()),
      action: () => run((r) => (r as any).chain?.().focus?.().goToPreviousCell?.().run?.()),
    },
    {
      id: 'fmt-table-next-cell',
      label: 'Ir para próxima célula',
      disabled: !inTable || !canRun((r) => (r as any).can?.().chain?.().focus?.().goToNextCell?.().run?.()),
      action: () => run((r) => (r as any).chain?.().focus?.().goToNextCell?.().run?.()),
    },
    { id: 'fmt-table-sep-5', separator: true },
    {
      id: 'fmt-table-delete',
      label: 'Apagar tabela',
      danger: true,
      disabled: !inTable || !canRun((r) => r.can?.().chain?.().focus?.().deleteTable?.().run?.()),
      action: () => run((r) => r.chain?.().focus?.().deleteTable?.().run?.()),
    },
  ];

  return [
    {
      id: 'fmt-text',
      label: 'Texto',
      icon: 'A',
      disabled: !canFormat,
      submenu: [
        {
          id: 'fmt-bold',
          label: 'Negrito',
          shortcut: 'Ctrl+B',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleBold?.().run?.()),
        },
        {
          id: 'fmt-italic',
          label: 'Itálico',
          shortcut: 'Ctrl+I',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleItalic?.().run?.()),
        },
        {
          id: 'fmt-strike',
          label: 'Tachado',
          shortcut: 'Ctrl+Shift+X',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleStrike?.().run?.()),
        },
        { id: 'fmt-link-sep', separator: true },
        {
          id: 'fmt-link-set',
          label: 'Inserir/Editar link',
          shortcut: 'Ctrl+K',
          disabled: !canFormat || typeof richEditorHandleRef.current?.openLinkDialog !== 'function',
          action: () =>
            run(() => {
              const open = richEditorHandleRef.current?.openLinkDialog;
              if (open) void open();
            }),
        },
        {
          id: 'fmt-link-unset',
          label: 'Remover link',
          disabled: !canFormat || !rich?.isActive?.('link'),
          action: () => run((r) => r.chain?.().focus?.().extendMarkRange?.('link')?.unsetLink?.().run?.()),
        },
        { id: 'fmt-text-sep', separator: true },
        {
          id: 'fmt-clear-marks',
          label: 'Limpar formatação de texto',
          icon: '↺',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().unsetAllMarks?.().run?.()),
        },
      ],
    },
    {
      id: 'fmt-paragraph',
      label: 'Parágrafo e títulos',
      icon: '¶',
      disabled: !canFormat,
      submenu: headingSubmenu,
    },
    {
      id: 'fmt-blocks',
      label: 'Blocos',
      icon: '▤',
      disabled: !canFormat,
      submenu: [
        {
          id: 'fmt-bq',
          label: 'Citação',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleBlockquote?.().run?.()),
        },
        {
          id: 'fmt-code',
          label: 'Bloco de código',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleCodeBlock?.().run?.()),
        },
        { id: 'fmt-blocks-sep', separator: true },
        {
          id: 'fmt-ul',
          label: 'Lista com marcadores',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleBulletList?.().run?.()),
        },
        {
          id: 'fmt-ol',
          label: 'Lista numerada',
          disabled: !canFormat,
          action: () => run((r) => r.chain?.().focus?.().toggleOrderedList?.().run?.()),
        },
      ],
    },
    {
      id: 'fmt-table',
      label: 'Tabela',
      icon: '▦',
      disabled: !canFormat,
      submenu: tableSubmenu,
    },
  ];
}
