import type { MenuItem } from '../../components/menu';
import { createRichMenuActions } from './richMenuActions';
import type { RichEditorLike } from './richMenuActions';
import type { FormatMenuContext } from './menuContext';
import { captureEditorFormatting, requestEditorFormatCommand } from '../../lib/commandEditorFormatting';
import { canNavigateCell, requestEditorCellNavigationCommand } from '../../lib/commandEditorCellNavigation';
import { Editor } from '@tiptap/core';

export function buildFormatMenuItemsForContextMenu(args: { ctx: FormatMenuContext }): MenuItem[] {
  const { ctx } = args;
  const { activeTab, isAsking, richEditorRef } = ctx;

  const richActions = createRichMenuActions({
    activeTab,
    isAsking,
    richEditorRef,
    requireTabMode: 'rich',
  });
  const capturedEditor = richEditorRef.current instanceof Editor ? richEditorRef.current : undefined;

  const canFormat = richActions.canUseRich && capturedEditor !== undefined && !activeTab?.readOnly && !activeTab?.loadError;
  const rich = richActions.rich as RichEditorLike | null;
  type FormatCommandID = Parameters<typeof requestEditorFormatCommand>[0];
  type FormatCommandInput = Parameters<typeof requestEditorFormatCommand>[1];

  const canRunFormatCommand = (commandID: FormatCommandID) => {
    if (!canFormat) return false;
    const target = captureEditorFormatting(capturedEditor);
    try {
      return target?.canExecute(commandID) === true;
    } finally {
      target?.dispose();
    }
  };

  const requestFormatCommand = (commandID: FormatCommandID, input?: FormatCommandInput) => () => {
    if (!canFormat || !capturedEditor) return;
    requestEditorFormatCommand(commandID, input, capturedEditor);
  };
  const requestCellNavigation = (commandID: 'editor.table.cell.next' | 'editor.table.cell.previous') => {
    if (capturedEditor && canNavigateCell(capturedEditor, commandID)) {
      const target = captureEditorFormatting(capturedEditor);
      const valid = target?.isCurrent() === true;
      target?.dispose();
      if (valid) requestEditorCellNavigationCommand(commandID, capturedEditor);
    }
  };

  const headingLevels = [1, 2, 3, 4, 5, 6] as const;
  const headingCommands = {
    1: 'editor.format.heading.h1',
    2: 'editor.format.heading.h2',
    3: 'editor.format.heading.h3',
    4: 'editor.format.heading.h4',
    5: 'editor.format.heading.h5',
    6: 'editor.format.heading.h6',
  } as const;
  const headingSubmenu: MenuItem[] = [
    {
      id: 'fmt-p',
      label: 'Parágrafo',
      icon: 'P',
      disabled: !canRunFormatCommand('editor.format.paragraph'),
      action: requestFormatCommand('editor.format.paragraph'),
    },
    ...headingLevels.map((lvl) => ({
      id: `fmt-h${lvl}`,
      label: `Título ${lvl} (H${lvl})`,
      icon: `H${lvl}`,
      disabled: !canRunFormatCommand(headingCommands[lvl]),
      action: requestFormatCommand(headingCommands[lvl]),
    })),
  ];

  const inTable = canFormat && !!rich?.isActive?.('table');
  const tableSubmenu: MenuItem[] = [
    {
      id: 'fmt-table-row-before',
      label: 'Adicionar linha acima',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.row.before'),
      action: requestFormatCommand('editor.format.table.row.before'),
    },
    {
      id: 'fmt-table-row-after',
      label: 'Adicionar linha abaixo',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.row.after'),
      action: requestFormatCommand('editor.format.table.row.after'),
    },
    {
      id: 'fmt-table-del-row',
      label: 'Remover linha',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.row.delete'),
      action: requestFormatCommand('editor.format.table.row.delete'),
    },
    { id: 'fmt-table-sep-1', separator: true },
    {
      id: 'fmt-table-col-before',
      label: 'Adicionar coluna antes',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.column.before'),
      action: requestFormatCommand('editor.format.table.column.before'),
    },
    {
      id: 'fmt-table-col-after',
      label: 'Adicionar coluna depois',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.column.after'),
      action: requestFormatCommand('editor.format.table.column.after'),
    },
    {
      id: 'fmt-table-del-col',
      label: 'Remover coluna',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.column.delete'),
      action: requestFormatCommand('editor.format.table.column.delete'),
    },
    { id: 'fmt-table-sep-2', separator: true },
    {
      id: 'fmt-table-toggle-header-row',
      label: 'Alternar cabeçalho (linha)',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.header.row'),
      action: requestFormatCommand('editor.format.table.header.row'),
    },
    {
      id: 'fmt-table-toggle-header-col',
      label: 'Alternar cabeçalho (coluna)',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.header.column'),
      action: requestFormatCommand('editor.format.table.header.column'),
    },
    {
      id: 'fmt-table-toggle-header-cell',
      label: 'Alternar cabeçalho (célula)',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.header.cell'),
      action: requestFormatCommand('editor.format.table.header.cell'),
    },
    { id: 'fmt-table-sep-3', separator: true },
    {
      id: 'fmt-table-merge',
      label: 'Mesclar células',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.merge'),
      action: requestFormatCommand('editor.format.table.merge'),
    },
    {
      id: 'fmt-table-split',
      label: 'Separar célula',
      disabled: !inTable || !canRunFormatCommand('editor.format.table.split'),
      action: requestFormatCommand('editor.format.table.split'),
    },
    { id: 'fmt-table-sep-4', separator: true },
    {
      id: 'fmt-table-prev-cell',
      label: 'Ir para célula anterior',
      disabled: !inTable || !capturedEditor ||
        !canNavigateCell(capturedEditor, 'editor.table.cell.previous'),
      action: () => requestCellNavigation('editor.table.cell.previous'),
    },
    {
      id: 'fmt-table-next-cell',
      label: 'Ir para próxima célula',
      disabled: !inTable || !capturedEditor ||
        !canNavigateCell(capturedEditor, 'editor.table.cell.next'),
      action: () => requestCellNavigation('editor.table.cell.next'),
    },
    { id: 'fmt-table-sep-5', separator: true },
    {
      id: 'fmt-table-delete',
      label: 'Apagar tabela',
      danger: true,
      disabled: !inTable || !canRunFormatCommand('editor.format.table.delete'),
      action: requestFormatCommand('editor.format.table.delete'),
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
          disabled: !canRunFormatCommand('editor.format.bold'),
          action: () => requestEditorFormatCommand('editor.format.bold'),
        },
        {
          id: 'fmt-italic',
          label: 'Itálico',
          shortcut: 'Ctrl+I',
          disabled: !canRunFormatCommand('editor.format.italic'),
          action: () => requestEditorFormatCommand('editor.format.italic'),
        },
        {
          id: 'fmt-strike',
          label: 'Tachado',
          shortcut: 'Ctrl+Shift+X',
          disabled: !canRunFormatCommand('editor.format.strike'),
          action: () => requestEditorFormatCommand('editor.format.strike'),
        },
        { id: 'fmt-link-sep', separator: true },
        {
          id: 'fmt-link-set',
          label: 'Inserir/Editar link',
          disabled: !canRunFormatCommand('editor.format.link.set'),
          action: requestFormatCommand('editor.format.link.set'),
        },
        {
          id: 'fmt-link-unset',
          label: 'Remover link',
          disabled: !canFormat || !rich?.isActive?.('link') || !canRunFormatCommand('editor.format.link.remove'),
          action: requestFormatCommand('editor.format.link.remove'),
        },
        { id: 'fmt-text-sep', separator: true },
        {
          id: 'fmt-clear-marks',
          label: 'Limpar formatação de texto',
          icon: '↺',
          disabled: !canRunFormatCommand('editor.format.clear_marks'),
          action: requestFormatCommand('editor.format.clear_marks'),
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
          disabled: !canRunFormatCommand('editor.format.blockquote'),
          action: requestFormatCommand('editor.format.blockquote'),
        },
        {
          id: 'fmt-code',
          label: 'Bloco de código',
          disabled: !canRunFormatCommand('editor.format.code_block'),
          action: requestFormatCommand('editor.format.code_block'),
        },
        { id: 'fmt-blocks-sep', separator: true },
        {
          id: 'fmt-ul',
          label: 'Lista com marcadores',
          disabled: !canRunFormatCommand('editor.format.list.bullet'),
          action: requestFormatCommand('editor.format.list.bullet'),
        },
        {
          id: 'fmt-ol',
          label: 'Lista numerada',
          disabled: !canRunFormatCommand('editor.format.list.ordered'),
          action: requestFormatCommand('editor.format.list.ordered'),
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
