import { useCallback, useEffect, useMemo, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import type { NavigateFunction } from 'react-router-dom';
import MarkdownIt from 'markdown-it';
import DOMPurify from 'dompurify';

import {
  extractRevealSlideAttributes,
  parseRevealMarkdown,
  stripRevealDirectives,
  type RevealSlide,
} from '../../lib/revealMarkdown';
import { markdownItDeepLink } from '../../lib/markdownItDeepLink';
import {
  getMarkdownFenceMarker,
  isClosingMarkdownFence,
  type MarkdownFenceMarker,
} from '../../lib/markdownFence';
import { isSafeLinkHref } from '../../lib/safeLink';
import { executeDeepLink, isDeepLink, parseDeepLink } from '../../lib/deepLinks';
import 'reveal.js/reveal.css';
import './RevealRenderer.css';

type RevealApi = {
  initialize: () => Promise<void> | void;
  destroy: () => void;
  sync: () => void;
  configure: (options: Record<string, unknown>) => void;
};

type RevealCtor = new (root: HTMLElement, options?: Record<string, unknown>) => RevealApi;
type MermaidApi = typeof import('mermaid')['default'];

interface RevealRendererProps {
  markdown: string;
  documentTitle?: string;
  fullscreenRequestNonce?: number;
  tabNavigation?: 'disabled' | 'enabled';
}

const md = new MarkdownIt({
  html: true,
  xhtmlOut: false,
  breaks: false,
  linkify: true,
  typographer: true,
});

md.use(markdownItDeepLink);

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
  ALLOWED_ATTR: ['href', 'src', 'alt', 'title', 'class', 'target', 'rel', 'tabindex', 'data-deep-link', 'role', 'aria-label'],
  ALLOWED_URI_REGEXP: /^(?:(?:(?:f|ht)tps?|mailto|tel|callto|sms|cid|xmpp|assistente):|[^a-z]|[a-z+.\-]+(?:[^a-z+.\-:]|$))/i,
};

const URL_DATA_ATTR_RE = /^data-background-(?:image|video|iframe)$/i;
const NOTE_RE = /^\s*Note:\s*$/i;

function trimBoundaryNewlines(value: string): string {
  return String(value || '').replace(/^(?:\r?\n)+|(?:\r?\n)+$/g, '');
}

function splitSpeakerNotes(markdown: string): { body: string; notes: string } {
  const text = String(markdown || '');
  const lines = text.match(/[^\n]*(?:\n|$)/g) ?? [''];
  const normalizedLines = lines.filter((line, index) => !(line === '' && index === lines.length - 1));
  let cursor = 0;
  let fence: MarkdownFenceMarker | null = null;

  for (const line of normalizedLines) {
    const lineWithoutNewline = line.replace(/\r?\n$/, '');
    if (fence) {
      if (isClosingMarkdownFence(lineWithoutNewline, fence)) fence = null;
    } else {
      const nextFence = getMarkdownFenceMarker(lineWithoutNewline);
      if (nextFence) {
        fence = nextFence;
      } else if (NOTE_RE.test(lineWithoutNewline)) {
        return {
          body: trimBoundaryNewlines(text.slice(0, cursor)),
          notes: trimBoundaryNewlines(text.slice(cursor + line.length)),
        };
      }
    }
    cursor += line.length;
  }

  return { body: markdown, notes: '' };
}

function renderMarkdownHtml(markdown: string): string {
  return DOMPurify.sanitize(md.render(markdown || ''), purifyConfig);
}

function enhanceLinkSecurity(html: string, tabNavigation: 'disabled' | 'enabled'): string {
  if (typeof document === 'undefined') return html;
  const template = document.createElement('template');
  template.innerHTML = html;
  template.content.querySelectorAll('a').forEach((link) => {
    const href = link.getAttribute('href') || '';
    if (isDeepLink(href)) {
      link.setAttribute('tabindex', tabNavigation === 'enabled' ? '0' : '-1');
      link.removeAttribute('target');
      link.removeAttribute('rel');
      return;
    }

    link.setAttribute('tabindex', tabNavigation === 'enabled' ? '0' : '-1');
    link.setAttribute('target', '_blank');
    link.setAttribute('rel', 'noopener noreferrer');
  });
  return template.innerHTML;
}

function enhanceImageAccessibility(html: string): string {
  if (typeof document === 'undefined') return html;
  const template = document.createElement('template');
  template.innerHTML = html;
  template.content.querySelectorAll('img').forEach((img) => {
    const alt = img.getAttribute('alt')?.trim();
    if (!alt) return;
    img.setAttribute('role', 'img');
    img.setAttribute('aria-label', alt);
    if (!img.getAttribute('title')) {
      img.setAttribute('title', alt);
    }
  });
  return template.innerHTML;
}

function revealDataProps(data: Record<string, string>) {
  return Object.fromEntries(
    Object.entries(data).filter(([key, value]) => {
      if (!/^data-[\w-]+$/.test(key)) return false;
      if (!URL_DATA_ATTR_RE.test(key)) return true;
      return isSafeLinkHref(value) && !/^mailto:/i.test(String(value || '').trim());
    })
  );
}

function renderSlide(
  slide: RevealSlide,
  getSlideLabel: (slide: RevealSlide) => string,
  tabNavigation: 'disabled' | 'enabled',
) {
  const attrs = extractRevealSlideAttributes(slide.markdown);
  const markdownWithoutDirectives = stripRevealDirectives(slide.markdown);
  const { body, notes } = splitSpeakerNotes(markdownWithoutDirectives);
  const html = enhanceLinkSecurity(
    enhanceImageAccessibility(renderMarkdownHtml(body)),
    tabNavigation,
  );
  const notesHtml = notes
    ? enhanceLinkSecurity(renderMarkdownHtml(notes), tabNavigation)
    : '';
  const label = getSlideLabel(slide);

  return (
    <section
      key={slide.index}
      aria-label={label}
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

function renderSlides(
  slides: RevealSlide[],
  getSlideLabel: (slide: RevealSlide) => string,
  tabNavigation: 'disabled' | 'enabled',
) {
  const rendered = [];
  for (let index = 0; index < slides.length; index += 1) {
    const slide = slides[index];
    if (slide.level === 'vertical') {
      const verticalSlides: RevealSlide[] = [];
      let nextIndex = index;
      while (nextIndex < slides.length && slides[nextIndex].level === 'vertical') {
        verticalSlides.push(slides[nextIndex]);
        nextIndex += 1;
      }
      rendered.push(
        <section key={`orphan-vertical-stack-${slide.index}`}>
          {verticalSlides.map((verticalSlide) => (
            renderSlide(verticalSlide, getSlideLabel, tabNavigation)
          ))}
        </section>
      );
      index = nextIndex - 1;
      continue;
    }

    const verticalSlides: RevealSlide[] = [];
    let nextIndex = index + 1;
    while (nextIndex < slides.length && slides[nextIndex].level === 'vertical') {
      verticalSlides.push(slides[nextIndex]);
      nextIndex += 1;
    }

    if (verticalSlides.length > 0) {
      const stackSlides = slide.markdown.trim() ? [slide, ...verticalSlides] : verticalSlides;
      rendered.push(
        <section key={`vertical-stack-${slide.index}`}>
          {stackSlides.map((stackSlide) => renderSlide(stackSlide, getSlideLabel, tabNavigation))}
        </section>
      );
      index = nextIndex - 1;
      continue;
    }

    rendered.push(renderSlide(slide, getSlideLabel, tabNavigation));
  }

  return rendered;
}

const navigateWithinApp: NavigateFunction = (to) => {
  if (typeof window === 'undefined') return;

  if (typeof to === 'number') {
    window.history.go(to);
    return;
  }

  const path = typeof to === 'string'
    ? to
    : `${to.pathname || ''}${to.search || ''}${to.hash || ''}`;
  window.history.pushState(null, '', path || '/');
  window.dispatchEvent(new PopStateEvent('popstate'));
};

export function RevealRenderer({
  markdown,
  documentTitle,
  fullscreenRequestNonce = 0,
  tabNavigation = 'disabled',
}: RevealRendererProps) {
  const { t } = useTranslation();
  const rootRef = useRef<HTMLDivElement | null>(null);
  const revealApiRef = useRef<RevealApi | null>(null);
  const tabNavigationRef = useRef(tabNavigation);
  const mermaidApiRef = useRef<MermaidApi | null>(null);
  const lastFullscreenRequestNonceRef = useRef(fullscreenRequestNonce);
  const deck = useMemo(() => parseRevealMarkdown(markdown, 'reveal'), [markdown]);
  const slides = deck.slides;
  const deckTitle = useMemo(
    () => deck.title || String(documentTitle || '').trim() || undefined,
    [deck.title, documentTitle]
  );
  tabNavigationRef.current = tabNavigation;
  const getSlideLabel = useCallback(
    (slide: RevealSlide) => slide.label || t('editor.presentation.slideOption', { index: slide.index + 1 }),
    [t]
  );

  const handleDeepLinkClick = useCallback(
    (event: MouseEvent) => {
      const target = event.target instanceof Element
        ? event.target.closest('a[href]') as HTMLAnchorElement | null
        : null;
      const uri = target?.getAttribute('data-deep-link') || target?.getAttribute('href') || '';
      if (!isDeepLink(uri)) return;

      const action = parseDeepLink(uri);
      if (!action) return;

      event.preventDefault();
      event.stopPropagation();
      void executeDeepLink(action, { navigate: navigateWithinApp });
    },
    []
  );

  const handleDeepLinkKeydown = useCallback(
    (event: KeyboardEvent) => {
      if (event.key !== 'Enter' && event.key !== ' ') return;

      const target = event.target instanceof Element
        ? event.target.closest('a[href]') as HTMLAnchorElement | null
        : null;
      const uri = target?.getAttribute('data-deep-link') || target?.getAttribute('href') || '';
      if (!isDeepLink(uri)) return;

      const action = parseDeepLink(uri);
      if (!action) return;

      event.preventDefault();
      event.stopPropagation();
      void executeDeepLink(action, { navigate: navigateWithinApp });
    },
    []
  );

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
        keyboard: tabNavigationRef.current !== 'enabled',
        center: true,
      });
      await api.initialize();
      if (disposed) {
        api.destroy();
        return;
      }
      revealApiRef.current = api;
      try {
        api.configure({ keyboard: tabNavigationRef.current !== 'enabled' });
        api.sync();
      } catch {
        // Best-effort: o efeito de slides também tenta sincronizar depois.
      }
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
    const root = rootRef.current;
    if (!root) return;

    root.addEventListener('click', handleDeepLinkClick as EventListener);
    root.addEventListener('keydown', handleDeepLinkKeydown as EventListener);

    return () => {
      root.removeEventListener('click', handleDeepLinkClick as EventListener);
      root.removeEventListener('keydown', handleDeepLinkKeydown as EventListener);
    };
  }, [handleDeepLinkClick, handleDeepLinkKeydown]);

  useEffect(() => {
    try {
      revealApiRef.current?.configure({ keyboard: tabNavigation !== 'enabled' });
      revealApiRef.current?.sync();
    } catch {
      // Best-effort: o renderer continua mostrando o HTML mesmo se o sync falhar.
    }
  }, [slides, tabNavigation]);

  useEffect(() => {
    if (fullscreenRequestNonce === lastFullscreenRequestNonceRef.current) return;
    lastFullscreenRequestNonceRef.current = fullscreenRequestNonce;
    const root = rootRef.current;
    if (!root) return;
    const fullscreenRoot = root;

    async function toggleFullscreen() {
      try {
        if (document.fullscreenElement) {
          await document.exitFullscreen?.();
          return;
        }
        await fullscreenRoot.requestFullscreen?.();
        revealApiRef.current?.sync();
      } catch {
        // Fullscreen pode não estar disponível em todos os WebViews; a apresentação permanece embutida.
      }
    }

    void toggleFullscreen();
  }, [fullscreenRequestNonce]);

  useEffect(() => {
    let disposed = false;
    const renderedWrappers: HTMLElement[] = [];

    async function renderMermaid() {
      if (!rootRef.current) return;
      const mermaidBlocks = Array.from(rootRef.current.querySelectorAll('code.language-mermaid')) as HTMLElement[];
      if (mermaidBlocks.length === 0) return;

      if (!mermaidApiRef.current) {
        const mod = await import('mermaid');
        const api = (mod.default ?? mod) as MermaidApi;
        api.initialize({ startOnLoad: false, theme: 'dark', securityLevel: 'strict' });
        mermaidApiRef.current = api;
      }

      const mermaid = mermaidApiRef.current;
      if (!mermaid || disposed) return;

      for (let index = 0; index < mermaidBlocks.length; index += 1) {
        const codeBlock = mermaidBlocks[index];
        const pre = codeBlock.parentElement as HTMLPreElement | null;
        if (!pre || pre.dataset.mermaidRendered) continue;

        const mermaidCode = (codeBlock.textContent || '').trimEnd();
        const wrapper = document.createElement('div');
        wrapper.className = 'mermaid-diagram';
        wrapper.setAttribute('role', 'group');
        wrapper.setAttribute('aria-label', t('editor.presentation.mermaidDiagramLabel'));
        wrapper.dataset.mermaidIndex = String(index);
        wrapper.dataset.mermaidCode = mermaidCode;
        wrapper.tabIndex = -1;

        try {
          const { svg } = await mermaid.render(`reveal-mermaid-${Date.now()}-${index}`, mermaidCode);
          if (disposed) return;
          wrapper.innerHTML = svg;
        } catch {
          if (disposed) return;
          wrapper.classList.add('mermaid-diagram--error');
          wrapper.textContent = t('editor.presentation.mermaidRenderError');
        }

        pre.parentNode?.insertBefore(wrapper, pre);
        pre.style.display = 'none';
        pre.dataset.mermaidRendered = 'true';
        renderedWrappers.push(wrapper);
      }

      try {
        revealApiRef.current?.sync();
      } catch {
        // Best-effort: o deck continua utilizável mesmo se o sync pós-Mermaid falhar.
      }
    }

    void renderMermaid();

    return () => {
      disposed = true;
      renderedWrappers.forEach((wrapper) => {
        const pre = wrapper.nextElementSibling as HTMLPreElement | null;
        if (pre?.dataset.mermaidRendered) {
          pre.style.display = '';
          pre.removeAttribute('data-mermaid-rendered');
        }
        wrapper.remove();
      });
    };
  }, [slides, t, tabNavigation]);

  return (
    <div
      className="reveal-renderer"
      role="region"
      aria-label={
        deckTitle
          ? t('editor.presentation.ariaWithTitle', { title: deckTitle })
          : t('editor.presentation.aria')
      }
    >
      <div className="reveal-renderer__hint">
        {t('editor.presentation.hint')}
      </div>
      <div ref={rootRef} className="reveal">
        <div className="slides">
          {renderSlides(slides, getSlideLabel, tabNavigation)}
        </div>
      </div>
    </div>
  );
}

