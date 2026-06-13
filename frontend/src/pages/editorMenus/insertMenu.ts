import i18next from 'i18next';
import type { MenuItem } from '../../components/menu';
import { createRichMenuActions } from './richMenuActions';
import type { InsertMenuContext } from './menuContext';

export function buildInsertMenuItemsForContextMenu(args: { ctx: InsertMenuContext }): MenuItem[] {
  const { ctx } = args;
  const { activeTab, isAsking, applyInsertRequest, focusEditorSoon, richEditorRef, addToast } = ctx;

  const canInsert = !!activeTab && !isAsking && activeTab.mode !== 'view';

  const richActions = createRichMenuActions({
    activeTab,
    isAsking,
    richEditorRef,
    requireTabMode: 'rich',
    addToast,
  });

  const insertMarkdownSnippet = async (content: string) => {
    if (!activeTab) return;
    await applyInsertRequest({
      id: `ui-editor-insert-${Date.now()}`,
      target: 'document',
      targetDocumentId: activeTab.id,
      format: 'markdown',
      content,
      focus: true,
    });
    focusEditorSoon();
  };

  const insertMarkdownTable = async (rows: number, cols: number) => {
    const header = Array.from({ length: cols }, (_, i) => `C${i + 1}`);
    const headerRow = `| ${header.join(' | ')} |`;
    const sepRow = `|${Array.from({ length: cols }, () => '---').join('|')}|`;
    const bodyRows = Array.from({ length: Math.max(1, rows - 1) }, () => {
      return `| ${Array.from({ length: cols }, () => ' ').join(' | ')} |`;
    }).join('\n');

    await insertMarkdownSnippet([headerRow, sepRow, bodyRows].filter(Boolean).join('\n') + '\n');
  };

  const insertRichTable = (rows: number, cols: number, withHeaderRow: boolean) => {
    richActions.run((rich) => {
      rich.chain?.().focus?.().insertTable?.({ rows, cols, withHeaderRow })?.run?.();
    });
  };

  const makeTableMenu = (): MenuItem => {
    const rowChoices = [2, 3, 4, 5, 6];
    const colChoices = [2, 3, 4, 5, 6];

    return {
      id: 'ins-table',
      label: 'Tabela',
      icon: '▦',
      disabled: !canInsert,
      submenu: rowChoices.map((r) => ({
        id: `ins-table-r${r}`,
        label: `${r} linhas`,
        submenu: colChoices.map((c) => ({
          id: `ins-table-r${r}-c${c}`,
          label: `${c} colunas`,
          submenu:
            activeTab?.mode === 'rich'
              ? [
                  {
                    id: `ins-table-r${r}-c${c}-hdr-on`,
                    label: 'Com cabeçalho',
                    action: () => insertRichTable(r, c, true),
                  },
                  {
                    id: `ins-table-r${r}-c${c}-hdr-off`,
                    label: 'Sem cabeçalho',
                    action: () => insertRichTable(r, c, false),
                  },
                ]
              : [
                  {
                    id: `ins-table-r${r}-c${c}-md`,
                    label: 'Inserir (Markdown)',
                    action: () => {
                      void insertMarkdownTable(r, c);
                    },
                  },
                  {
                    id: `ins-table-r${r}-c${c}-hdr-note`,
                    label: 'Cabeçalho: Markdown sempre usa a 1ª linha',
                    disabled: true,
                  },
                ],
        })),
      })),
    };
  };

  return [
    {
      id: 'ins-mermaid',
      label: 'Diagrama Mermaid',
      icon: '🧩',
      disabled: !canInsert,
      action: () => {
        if (!activeTab) return;
        if (activeTab.mode === 'markdown') {
          void insertMarkdownSnippet('```mermaid\nflowchart TD\n  A[Início] --> B[Fim]\n```\n');
          return;
        }

        const template = 'flowchart TD\n  A[Início] --> B[Fim]';
        const didRun = richActions.run((rich) => {
          rich
            .chain?.()
            .focus?.()
            .setCodeBlock?.({ language: 'mermaid' })
            ?.insertContent?.(template)
            ?.run?.();
        });
        if (didRun) {
          addToast(i18next.t('editor.toast.mermaidInserted'), 'success');
        } else {
          // Quando não está pronto, mostra toast informativo.
          richActions.getRichOrToast();
        }
      },
    },
    {
      id: 'ins-codeblock',
      label: 'Bloco de código',
      icon: '{ }',
      disabled: !canInsert,
      action: () => {
        if (!activeTab) return;
        if (activeTab.mode === 'markdown') {
          void insertMarkdownSnippet('```\n\n```\n');
          return;
        }
        richActions.run((rich) => {
          rich.chain?.().focus?.().setCodeBlock?.({ language: '' })?.run?.();
        });
      },
    },
    makeTableMenu(),
    {
      id: 'ins-lists',
      label: 'Listas',
      icon: '•',
      disabled: !canInsert,
      submenu: [
        {
          id: 'ins-bullets',
          label: 'Marcadores',
          shortcut: '•',
          disabled: !canInsert,
          action: () => {
            if (!activeTab) return;
            if (activeTab.mode === 'markdown') {
              void insertMarkdownSnippet('- item\n- item\n');
              return;
            }
            richActions.run((rich) => {
              rich.chain?.().focus?.().toggleBulletList?.().run?.();
            });
          },
        },
        {
          id: 'ins-numbers',
          label: 'Numerada',
          shortcut: '1.',
          disabled: !canInsert,
          action: () => {
            if (!activeTab) return;
            if (activeTab.mode === 'markdown') {
              void insertMarkdownSnippet('1. item\n2. item\n');
              return;
            }
            richActions.run((rich) => {
              rich.chain?.().focus?.().toggleOrderedList?.().run?.();
            });
          },
        },
      ],
    },
    {
      id: 'ins-blockquote',
      label: 'Citação',
      icon: '❝',
      disabled: !canInsert,
      action: () => {
        if (!activeTab) return;
        if (activeTab.mode === 'markdown') {
          void insertMarkdownSnippet('> ');
          return;
        }
        richActions.run((rich) => {
          rich.chain?.().focus?.().toggleBlockquote?.().run?.();
        });
      },
    },
  ];
}
