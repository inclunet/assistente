import i18next from 'i18next';

export const EDITOR_SLIDE_COMMAND_IDS = [
  'editor.slide.insert.basic',
  'editor.slide.insert.title',
  'editor.slide.insert.two_columns',
  'editor.slide.insert.image_right',
  'editor.slide.insert.image_left',
  'editor.slide.insert.section',
  'editor.slide.insert.agenda',
  'editor.slide.insert.quote',
  'editor.slide.insert.comparison',
  'editor.slide.insert.code',
  'editor.slide.insert.diagram',
] as const;

export type EditorSlideCommandID = typeof EDITOR_SLIDE_COMMAND_IDS[number];

export function isEditorSlideCommand(id: string): id is EditorSlideCommandID {
  return (EDITOR_SLIDE_COMMAND_IDS as readonly string[]).includes(id);
}

export function buildEditorSlideTemplate(id: string): string | undefined {
  switch (id) {
    case 'editor.slide.insert.basic':
      return `<!-- .slide: class="content-slide" -->

## ${i18next.t('editor.presentation.newSlideTitle')}`;
    case 'editor.slide.insert.title':
      return `<!-- .slide: class="title-slide" -->

# ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.subtitlePlaceholder')}`;
    case 'editor.slide.insert.two_columns':
      return `<!-- .slide: class="two-columns" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

### ${i18next.t('editor.presentation.insert.firstColumn')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}

### ${i18next.t('editor.presentation.insert.secondColumn')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}`;
    case 'editor.slide.insert.image_right':
      return `<!-- .slide: class="image-right" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.textPlaceholder')}

![${i18next.t('editor.presentation.insert.altPlaceholder')}](assets/image.png)`;
    case 'editor.slide.insert.image_left':
      return `<!-- .slide: class="image-left" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.textPlaceholder')}

![${i18next.t('editor.presentation.insert.altPlaceholder')}](assets/image.png)`;
    case 'editor.slide.insert.section':
      return `<!-- .slide: class="section-slide" -->

# ${i18next.t('editor.presentation.insert.titlePlaceholder')}

${i18next.t('editor.presentation.insert.subtitlePlaceholder')}`;
    case 'editor.slide.insert.agenda':
      return `<!-- .slide: class="agenda-slide" -->

## ${i18next.t('editor.presentation.insert.agenda')}

1. ${i18next.t('editor.presentation.insert.firstTopic')}
2. ${i18next.t('editor.presentation.insert.secondTopic')}
3. ${i18next.t('editor.presentation.insert.thirdTopic')}`;
    case 'editor.slide.insert.quote':
      return `<!-- .slide: class="quote-slide" -->

> ${i18next.t('editor.presentation.insert.quotePlaceholder')}

- ${i18next.t('editor.presentation.insert.authorPlaceholder')}`;
    case 'editor.slide.insert.comparison':
      return `<!-- .slide: class="comparison-slide two-columns" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

### ${i18next.t('editor.presentation.insert.before')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}

### ${i18next.t('editor.presentation.insert.after')}

- ${i18next.t('editor.presentation.insert.itemPlaceholder')}`;
    case 'editor.slide.insert.code':
      return `<!-- .slide: class="code-slide" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

\`\`\`ts
// ${i18next.t('editor.presentation.insert.codePlaceholder')}
function example() {
  return true;
}
\`\`\``;
    case 'editor.slide.insert.diagram':
      return `<!-- .slide: class="diagram-slide" -->

## ${i18next.t('editor.presentation.insert.titlePlaceholder')}

\`\`\`mermaid
flowchart TD
  A[${i18next.t('editor.presentation.insert.diagramStart')}] --> B[${i18next.t('editor.presentation.insert.diagramEnd')}]
\`\`\``;
    default:
      return undefined;
  }
}

export function appendEditorSlideMarkdown(current: string, content: string): string {
  const currentWithoutTrailingNewlines = String(current ?? '').replace(/[\r\n]+$/, '');
  const trimmedContent = String(content ?? '').trim();
  const hasTrailingSlideSeparator = /(^|\r?\n)\s*-{3,4}\s*$/.test(currentWithoutTrailingNewlines);
  const separator = currentWithoutTrailingNewlines.trim()
    ? hasTrailingSlideSeparator
      ? '\n\n'
      : '\n\n---\n\n'
    : '';
  return `${currentWithoutTrailingNewlines}${separator}${trimmedContent}\n`;
}
