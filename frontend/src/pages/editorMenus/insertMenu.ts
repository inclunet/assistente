import i18next from 'i18next';
import type { MenuItem } from '../../components/menu';
import type { InsertMenuContext } from './menuContext';
import { captureEditorFormatting, requestEditorFormatCommand, type EditorFormatCommandID } from '../../lib/commandEditorFormatting';
import type { EditorCommandInput } from '../../lib/commandEditorInputs';
import { Editor } from '@tiptap/core';

export function buildInsertMenuItemsForContextMenu({ ctx }: { ctx: InsertMenuContext }): MenuItem[] {
  const { activeTab, isAsking, richEditorRef } = ctx;
  const canInsert = !!activeTab && !isAsking && !activeTab.readOnly && !activeTab.loadError && activeTab.mode !== 'view';
  const capturedEditor = activeTab?.mode === 'rich'
    ? (richEditorRef.current instanceof Editor ? richEditorRef.current : undefined)
    : activeTab ?? undefined;
  const expected = (id: EditorFormatCommandID) => id.startsWith('editor.slide.') ? activeTab ?? undefined : capturedEditor;
  const canRequest = (id: EditorFormatCommandID) => {
    if (!canInsert || !expected(id)) return false;
    const target = captureEditorFormatting(expected(id));
    try { return target?.canExecute(id) === true; }
    finally { target?.dispose(); }
  };
  const request = (id: EditorFormatCommandID, input?: EditorCommandInput) => {
    if (canRequest(id)) requestEditorFormatCommand(id, input, expected(id));
  };
  const item = (id: string, command: EditorFormatCommandID, label: string, icon?: string): MenuItem => ({
    // Menus are built before effects register editor surfaces. Resolve the
    // capability when the menu reads it, not once during the mounting render.
    id, label, icon, get disabled() { return !canRequest(command); }, action: () => request(command),
  });
  const choices = [2, 3, 4, 5, 6];
  const table = 'editor.format.table.insert';
  const slides = [
    ['basic', 'basic'], ['title', 'title'], ['two_columns', 'twoColumns'],
    ['image_right', 'imageRight'], ['image_left', 'imageLeft'], ['section', 'section'],
    ['agenda', 'agenda'], ['quote', 'quote'], ['comparison', 'comparison'],
    ['code', 'code'], ['diagram', 'diagram'],
  ] as const;
  return [
    {
      id: 'ins-slide', label: i18next.t('editor.presentation.insert.menu'), icon: '▭', disabled: !canInsert,
      submenu: slides.map(([suffix, label]) => item(`ins-slide-${suffix.replace(/_/g, '-')}`,
        `editor.slide.insert.${suffix}`, i18next.t(`editor.presentation.insert.${label}`))),
    },
    item('ins-mermaid', 'editor.format.mermaid.insert', i18next.t('editor.presentation.mermaidDiagramLabel'), '🧩'),
    item('ins-codeblock', 'editor.format.code_block.insert', i18next.t('editor.insertCommands.codeBlock'), '{ }'),
    {
      id: 'ins-table', label: i18next.t('editor.commandTable.title'), icon: '▦', get disabled() { return !canRequest(table); },
      submenu: choices.map(rows => ({
        id: `ins-table-r${rows}`, label: i18next.t('editor.insertCommands.rows', { count: rows }),
        submenu: choices.map(cols => ({
          id: `ins-table-r${rows}-c${cols}`, label: i18next.t('editor.insertCommands.cols', { count: cols }),
          submenu: activeTab?.mode === 'rich'
            ? [true, false].map(withHeaderRow => ({
              id: `ins-table-r${rows}-c${cols}-hdr-${withHeaderRow ? 'on' : 'off'}`,
              label: i18next.t(withHeaderRow ? 'editor.insertCommands.headerOn' : 'editor.insertCommands.headerOff'),
              get disabled() { return !canRequest(table); }, action: () => request(table, { kind: 'table', rows, cols, withHeaderRow }),
            }))
            : [{
              id: `ins-table-r${rows}-c${cols}-md`, label: i18next.t('editor.insertCommands.markdown'),
              get disabled() { return !canRequest(table); }, action: () => request(table, { kind: 'table', rows, cols, withHeaderRow: true }),
            }, {
              id: `ins-table-r${rows}-c${cols}-hdr-note`, label: i18next.t('editor.commandTable.markdownDescription'), disabled: true,
            }],
        })),
      })),
    },
    {
      id: 'ins-lists', label: i18next.t('editor.insertCommands.lists'), icon: '•', disabled: !canInsert,
      submenu: [
        item('ins-bullets', 'editor.format.list.bullet', i18next.t('editor.insertCommands.bullets'), '•'),
        item('ins-numbers', 'editor.format.list.ordered', i18next.t('editor.insertCommands.numbers'), '1.'),
      ],
    },
    item('ins-blockquote', 'editor.format.blockquote', i18next.t('editor.insertCommands.quote'), '❝'),
  ];
}
