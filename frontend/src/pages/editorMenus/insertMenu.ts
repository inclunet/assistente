import i18next from 'i18next';
import type { MenuItem } from '../../components/menu';
import { createRichMenuActions } from './richMenuActions';
import type { InsertMenuContext } from './menuContext';

export function buildInsertMenuItemsForContextMenu(args: { ctx: InsertMenuContext }): MenuItem[] {
  const { ctx } = args;
  const { activeTab, isAsking, applyInsertRequest, appendMarkdownToDocument, focusEditorSoon, richEditorRef, addToast } = ctx;

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

  const insertRevealSlide = async (content: string) => {
    appendMarkdownToDocument(content);
    focusEditorSoon();
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

  const makeSlideMenu = (): MenuItem => ({
    id: 'ins-slide',
    label: i18next.t('editor.presentation.insert.menu'),
    icon: '▭',
    disabled: !canInsert,
    submenu: [
      {
        id: 'ins-slide-basic',
        label: i18next.t('editor.presentation.insert.basic'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="content-slide" -->

## ${i18next.t('editor.presentation.newSlideTitle')}`);
        },
      },
      {
        id: 'ins-slide-title',
        label: i18next.t('editor.presentation.insert.title'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="title-slide" -->

# ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.subtitlePlaceholder')}`);
        },
      },
      {
        id: 'ins-slide-two-columns',
        label: i18next.t('editor.presentation.insert.twoColumns'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="two-columns" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

### ${i18next.t('editor.presentation.insert.firstColumn')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}

### ${i18next.t('editor.presentation.insert.secondColumn')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}`);
        },
      },
      {
        id: 'ins-slide-image-right',
        label: i18next.t('editor.presentation.insert.imageRight'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="image-right" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.textPlaceholder')}

![${i18next.t('editor.presentation.insert.altPlaceholder')}](assets/image.png)`);
        },
      },
      {
        id: 'ins-slide-image-left',
        label: i18next.t('editor.presentation.insert.imageLeft'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="image-left" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.textPlaceholder')}

![${i18next.t('editor.presentation.insert.altPlaceholder')}](assets/image.png)`);
        },
      },
      {
        id: 'ins-slide-section',
        label: i18next.t('editor.presentation.insert.section'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="section-slide" -->

# ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.subtitlePlaceholder')}`);
        },
      },
      {
        id: 'ins-slide-agenda',
        label: i18next.t('editor.presentation.insert.agenda'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="agenda-slide" -->

## ${i18next.t('editor.presentation.insert.agenda')}

1. ${i18next.t('editor.presentation.insert.firstTopic')}
2. ${i18next.t('editor.presentation.insert.secondTopic')}
3. ${i18next.t('editor.presentation.insert.thirdTopic')}`);
        },
      },
      {
        id: 'ins-slide-quote',
        label: i18next.t('editor.presentation.insert.quote'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="quote-slide" -->

> ${i18next.t('editor.presentation.insert.quotePlaceholder')}

- ${i18next.t('editor.presentation.insert.authorPlaceholder')}`);
        },
      },
      {
        id: 'ins-slide-comparison',
        label: i18next.t('editor.presentation.insert.comparison'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="comparison-slide two-columns" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

### ${i18next.t('editor.presentation.insert.before')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}

### ${i18next.t('editor.presentation.insert.after')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}`);
        },
      },
      {
        id: 'ins-slide-code',
        label: i18next.t('editor.presentation.insert.code'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="code-slide" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

\`\`\`ts
// ${i18next.t('editor.presentation.insert.codePlaceholder')}
function example() {
  return true;
}
\`\`\``);
        },
      },
      {
        id: 'ins-slide-diagram',
        label: i18next.t('editor.presentation.insert.diagram'),
        action: () => {
          void insertRevealSlide(`<!-- .slide: class="diagram-slide" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

\`\`\`mermaid
flowchart TD
  A[${i18next.t('editor.presentation.insert.diagramStart')}] --> B[${i18next.t('editor.presentation.insert.diagramEnd')}]
\`\`\``);
        },
      },
    ],
  });

  return [
    makeSlideMenu(),
    {
      id: 'ins-mermaid',
      label: i18next.t('editor.presentation.mermaidDiagramLabel'),
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
