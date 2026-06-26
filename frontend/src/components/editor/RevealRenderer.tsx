import { useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';

import {
  extractRevealSlideAttributes,
  parseRevealMarkdown,
  stripRevealDirectives,
  type RevealSlide,
} from '../../lib/revealMarkdown';
import 'reveal.js/reveal.css';
import './RevealRenderer.css';

type RevealApi = {
  initialize: () => Promise<void> | void;
  destroy: () => void;
  sync: () => void;
};

type RevealCtor = new (root: HTMLElement, options?: Record<string, unknown>) => RevealApi;

interface RevealRendererProps {
  markdown: string;
}

const md = new MarkdownIt({
  html: true,
  xhtmlOut: false,
  breaks: false,
  linkify: true,
  typographer: true,
});

const purifyConfig = {
  ALLOWED_TAGS: [
    'p',
    'br',
    'strong',
    'em',
    'b',
    'i',
    'u',
    's',
    'del',
    'h1',
    'h2',
    'h3',
    'h4',
    'h5',
    'h6',
    'ul',
    'ol',
    'li',
    'a',
    'img',
    'pre',
    'code',
    'blockquote',
    'table',
    'thead',
    'tbody',
    'tr',
    'th',
    'td',
    'hr',
    'div',
    'span',
    'aside',
  ],
  ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'class', 'target', 'rel', 'tabindex', 'role', 'aria-label'],
  ALLOWED_URI_REGEXP: /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|assistente):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
};

function splitSpeakerNotes(markdown: string): { body: string; notes: string } {
  const match = String(markdown || '').match(/^\s*Note:\s*$/im);
  if (!match || match.index === undefined) return { body: markdown, notes: '' };
  return {
    body: markdown.slice(0, match.index).trim(),
    notes: markdown.slice(match.index + match[0].length).trim(),
  };
}

function renderMarkdownHtml(markdown: string): string {
  return DOMPurify.sanitize(md.render(markdown || ''), purifyConfig);
}

function revealDataProps(data: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(data).filter(([key]) => /^data-[\w-]+$/.test(key))
  );
}

function renderSlide(slide: RevealSlide) {
  const attrs = extractRevealSlideAttributes(slide.markdown);
  const markdownWithoutDirectives = stripRevealDirectives(slide.markdown);
  const { body, notes } = splitSpeakerNotes(markdownWithoutDirectives);
  const html = renderMarkdownHtml(body);
  const notesHtml = notes ? renderMarkdownHtml(notes) : '';

  return (
    <section
      key={slide.index}
      className={attrs.className}
      {...revealDataProps(attrs.data)}
      dangerouslySetInnerHTML={{
        __html: notesHtml
          ? `${html}<aside class="notes">${notesHtml}</aside>`
          : html,
      }}
    />
  );
}

export function RevealRenderer({ markdown }: RevealRendererProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement | null>(null);
  const revealApiRef = useRef<RevealApi | null>(null);
  const deck = useMemo(() => parseRevealMarkdown(markdown, 'reveal'), [markdown]);
  const slides = deck.slides;

  useEffect(() => {
    let disposed = false;

    async function start() {
      if (!rootRef.current) return;
      const mod = await import('reveal.js');
      if (disposed || !rootRef.current) return;
      const Reveal = (mod.default ?? mod) as unknown as RevealCtor;
      const api = new Reveal(rootRef.current, {
        embedded: true,
        hash: false,
        controls: true,
        progress: true,
        slideNumber: true,
        keyboard: true,
        center: true,
      });
      await api.initialize();
      if (disposed) {
        api.destroy();
        return;
      }
      revealApiRef.current = api;
    }

    void start();

    return () => {
      disposed = true;
      try {
        revealApiRef.current?.destroy();
      } catch {
        // Reveal pode lançar se o ciclo de vida for interrompido durante inicialização.
      }
      revealApiRef.current = null;
    };
  }, []);

  useEffect(() => {
    try {
      revealApiRef.current?.sync();
    } catch {
      // Best-effort: o renderer continua mostrando o HTML mesmo se o sync falhar.
    }
  }, [slides]);

  return (
    <div className="reveal-renderer" role="region" aria-label={t('editor.presentation.aria')}>
      <div className="reveal-renderer__hint">
        {t('editor.presentation.hint')}
      </div>
      <div ref={rootRef} className="reveal">
        <div className="slides">
          {slides.map(renderSlide)}
        </div>
      </div>
    </div>
  );
}

