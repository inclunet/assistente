import MarkdownIt from 'markdown-it';

const md = new MarkdownIt({
  html: false,
  xhtmlOut: false,
  breaks: false,
  linkify: true,
  typographer: true,
});

export function markdownToHtml(markdown: string): string {
  if (!markdown) return '';
  return md.render(markdown);
}
