import type { EditorInsertFormat, EditorMode } from '../store/editorStore';

function looksLikeHtml(input: string): boolean {
  const text = String(input ?? '');
  if (!text) return false;

  // Heurística: se tem fence de código, assume Markdown (pode conter tags como texto).
  if (text.includes('```')) return false;

  // Tags comuns (inclui <br>, <p>, <div>, etc.)
  return /<\s*\/?\s*[a-zA-Z][^>]*>/.test(text);
}

function looksLikeEncodedHtmlEntities(input: string): boolean {
  const text = String(input ?? '');
  if (!text) return false;

  // Heurística: entidades que parecem tags (ex.: &lt;p&gt;...
  // Se estiver dentro de fence de código, tratamos como markdown (não converte).
  if (text.includes('```')) return false;

  return /&lt;\s*\/?\s*[a-zA-Z][^&]*&gt;/.test(text);
}

function decodeHtmlEntities(input: string): string {
  // Decodificação simples (suficiente para conteúdo vindo do chat)
  return String(input ?? '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&amp;/g, '&')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&#x([0-9a-fA-F]+);/g, (_, hex) => {
      try {
        return String.fromCodePoint(parseInt(hex, 16));
      } catch {
        return _;
      }
    })
    .replace(/&#(\d+);/g, (_, num) => {
      try {
        return String.fromCodePoint(parseInt(num, 10));
      } catch {
        return _;
      }
    });
}

function stripTagsPreservingBreaks(html: string): string {
  let text = String(html ?? '');

  // Normaliza quebras.
  text = text.replace(/\r\n?/g, '\n');

  // Quebras explícitas.
  text = text.replace(/<\s*br\s*\/?\s*>/gi, '\n');

  // Fecha blocos com quebras.
  text = text.replace(/<\s*\/\s*(p|div|section|article|header|footer|h[1-6]|li|tr)\s*>/gi, '\n');

  // Tags de lista: prefixo de item (simplificado).
  text = text.replace(/<\s*li\b[^>]*>/gi, '- ');

  // Remove tags restantes.
  text = text.replace(/<[^>]+>/g, '');

  // Decodifica entidades após remover tags.
  text = decodeHtmlEntities(text);

  // Limpa espaços.
  text = text.replace(/\n{3,}/g, '\n\n').replace(/[ \t]{2,}/g, ' ');
  return text.trim();
}

function htmlToPlainText(html: string): string {
  return stripTagsPreservingBreaks(html);
}

function extractLanguageFromCodeAttrs(attrs: string): string {
  const m = /language-([a-zA-Z0-9_-]+)/.exec(attrs);
  return m?.[1] ? String(m[1]) : '';
}

function unwrapPreCode(html: string): null | { language: string; text: string } {
  const src = String(html ?? '');
  const m = /<\s*pre\b[^>]*>\s*<\s*code\b([^>]*)>([\s\S]*?)<\s*\/\s*code\s*>\s*<\s*\/\s*pre\s*>/i.exec(src);
  if (!m) return null;
  const attrs = String(m[1] ?? '');
  const inner = String(m[2] ?? '');
  const language = extractLanguageFromCodeAttrs(attrs);
  const text = decodeHtmlEntities(inner).replace(/\r\n?/g, '\n').trim();
  return { language, text };
}

function looksLikeMarkdownDoc(text: string): boolean {
  const t = String(text ?? '').trimStart();
  if (!t) return false;
  // Heurística: começa com heading/list/quote/fence
  if (/^#{1,6}\s+/.test(t)) return true;
  if (/^\s*[-*+]\s+/.test(t)) return true;
  if (/^\s*\d+\.\s+/.test(t)) return true;
  if (/^\s*>\s+/.test(t)) return true;
  if (/^```/.test(t)) return true;
  // Ou tem múltiplas linhas típicas de markdown
  if (/\n\s*\n/.test(t) && /\*\*|\[.+\]\(.+\)|`/.test(t)) return true;
  return false;
}

function htmlToMarkdown(html: string): string {
  let text = String(html ?? '');
  text = text.replace(/\r\n?/g, '\n');

  // Code blocks: <pre><code class="language-x">...</code></pre>
  text = text.replace(/<\s*pre\b[^>]*>\s*<\s*code\b([^>]*)>([\s\S]*?)<\s*\/\s*code\s*>\s*<\s*\/\s*pre\s*>/gi, (_m, attrs, inner) => {
    const lang = extractLanguageFromCodeAttrs(String(attrs ?? ''));
    const code = decodeHtmlEntities(String(inner ?? '')).replace(/\n{3,}/g, '\n\n').trim();
    const fence = '```';
    return `\n\n${fence}${lang ? lang : ''}\n${code}\n${fence}\n\n`;
  });

  // Links
  text = text.replace(/<\s*a\b[^>]*href\s*=\s*"([^"]+)"[^>]*>([\s\S]*?)<\s*\/\s*a\s*>/gi, (_m, href, inner) => {
    const label = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim() || String(href);
    return `[${label}](${String(href)})`;
  });

  // Headings
  text = text.replace(/<\s*h([1-6])\b[^>]*>([\s\S]*?)<\s*\/\s*h\1\s*>/gi, (_m, level, inner) => {
    const n = Math.min(6, Math.max(1, Number(level)));
    const content = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim();
    return `\n\n${'#'.repeat(n)} ${content}\n\n`;
  });

  // Inline formatting
  text = text.replace(/<\s*(strong|b)\b[^>]*>([\s\S]*?)<\s*\/\s*\1\s*>/gi, (_m, _tag, inner) => {
    const content = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim();
    return `**${content}**`;
  });
  text = text.replace(/<\s*(em|i)\b[^>]*>([\s\S]*?)<\s*\/\s*\1\s*>/gi, (_m, _tag, inner) => {
    const content = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim();
    return `*${content}*`;
  });
  text = text.replace(/<\s*code\b[^>]*>([\s\S]*?)<\s*\/\s*code\s*>/gi, (_m, inner) => {
    const content = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim();
    return content ? `\`${content}\`` : '';
  });

  // List items
  text = text.replace(/<\s*li\b[^>]*>([\s\S]*?)<\s*\/\s*li\s*>/gi, (_m, inner) => {
    const content = stripTagsPreservingBreaks(String(inner ?? '')).replace(/\n+/g, ' ').trim();
    return `\n- ${content}`;
  });

  // Quebras e blocos
  text = text.replace(/<\s*br\s*\/?\s*>/gi, '\n');
  text = text.replace(/<\s*\/\s*(p|div|section|article|header|footer)\s*>/gi, '\n\n');

  // Remove tags restantes e decodifica entidades
  text = text.replace(/<[^>]+>/g, '');
  text = decodeHtmlEntities(text);

  // Normaliza espaçamento
  text = text.replace(/\n{3,}/g, '\n\n');
  return text.trim();
}

export function normalizeEditorInsertContent(args: {
  content: string;
  format: EditorInsertFormat;
  targetMode: EditorMode;
}): { content: string; format: EditorInsertFormat } {
  const raw = String(args.content ?? '');
  const format = args.format;
  const targetMode = args.targetMode;

  if (!raw) return { content: raw, format };
  if (format === 'html') return { content: raw, format };

  // Alguns caminhos podem entregar HTML escapado como entidades (&lt;p&gt;...).
  // Decodificamos apenas se, após decodificar, isso realmente parecer HTML.
  let htmlCandidate = raw;
  if (looksLikeEncodedHtmlEntities(htmlCandidate)) {
    const decoded = decodeHtmlEntities(htmlCandidate);
    if (looksLikeHtml(decoded)) htmlCandidate = decoded;
  }

  // Caso especial: alguns caminhos entregam a mensagem inteira dentro de <pre><code>...
  // (ou o HTML renderizado do chat). Se o conteúdo interno parece Markdown, devemos
  // tratá-lo como Markdown, não como um code block.
  const unwrapped = unwrapPreCode(htmlCandidate);
  if (unwrapped && looksLikeMarkdownDoc(unwrapped.text)) {
    if (args.format === 'plain') {
      return { content: unwrapped.text, format: 'plain' };
    }
    // Mantém escolha do usuário: se pediu markdown, volta markdown.
    if (args.format === 'markdown') {
      return { content: unwrapped.text, format: 'markdown' };
    }
    // Se pediu html, deixamos passar.
  }

  if (!looksLikeHtml(htmlCandidate)) return { content: raw, format };

  // Se parece HTML, normaliza de acordo com destino.
  if (targetMode === 'rich') {
    // Se o usuário escolheu "Markdown" mas o conteúdo é HTML, melhor inserir como HTML
    // do que transformar em texto escapado.
    if (format === 'markdown') return { content: htmlCandidate, format: 'html' };
    if (format === 'plain') return { content: htmlToPlainText(htmlCandidate), format: 'plain' };
    return { content: raw, format };
  }

  // Markdown / View: inserimos texto no Monaco; View usa markdown por baixo.
  if (format === 'markdown') return { content: htmlToMarkdown(htmlCandidate), format: 'markdown' };
  if (format === 'plain') return { content: htmlToPlainText(htmlCandidate), format: 'plain' };

  return { content: raw, format };
}

export const __private__ = {
  looksLikeHtml,
  looksLikeEncodedHtmlEntities,
  unwrapPreCode,
  looksLikeMarkdownDoc,
  htmlToPlainText,
  htmlToMarkdown,
  decodeHtmlEntities,
};
