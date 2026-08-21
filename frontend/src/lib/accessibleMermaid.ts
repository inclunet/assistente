import {
  createNavigator,
  getMessages,
  type DiagramNavigator,
  type HighlightContext,
  type HighlightRenderer,
  type HighlightTarget,
} from '@inclunet/mermaid-a11y/core';
import {
  renderAccessibleDiagram,
  type MermaidConfig,
  type MermaidLike,
} from '@inclunet/mermaid-a11y/mermaid';
import { announce } from '../hooks/useAnnouncer';
import { playBumpSound } from '../services/audioFeedback';
import './accessibleMermaid.css';

type MermaidModule = typeof import('mermaid');
export type MermaidApi = MermaidModule['default'];

export interface AccessibleMermaidOptions {
  chart: string;
  container: HTMLElement;
  mermaid: MermaidLike;
  locale?: string;
  navigationEnabled: boolean;
  ariaLabel: string;
  config?: MermaidConfig;
}

export interface AccessibleMermaidResult {
  diagramType: string;
  navigable: boolean;
  svg: SVGSVGElement;
  cleanup(): void;
}

export interface MermaidErrorContentOptions {
  container: HTMLElement;
  ariaLabel: string;
  title: string;
  message: string;
  detailsLabel: string;
  errorText: string;
  detailsTabbable: boolean;
}

export interface MermaidErrorContent {
  details: HTMLDetailsElement;
  errorPre: HTMLPreElement;
}

let mermaidPromise: Promise<MermaidApi> | null = null;
let instructionsSequence = 0;

export function loadMermaid(): Promise<MermaidApi> {
  if (!mermaidPromise) {
    mermaidPromise = import('mermaid').then((module) => {
      const api = module.default;
      api.initialize({
        startOnLoad: false,
        theme: 'dark',
        securityLevel: 'strict',
      });
      return api;
    });
  }
  return mermaidPromise;
}

export async function renderAccessibleMermaid(
  options: AccessibleMermaidOptions,
): Promise<AccessibleMermaidResult> {
  const {
    chart,
    container,
    mermaid,
    locale,
    navigationEnabled,
    ariaLabel,
    config = {
      theme: 'dark',
      securityLevel: 'strict',
    },
  } = options;

  const rendered = await renderAccessibleDiagram({
    chart,
    config,
    mermaid,
    host: container,
  });

  container.setAttribute('role', 'group');
  container.setAttribute('aria-label', rendered.graph.title || ariaLabel);
  container.dataset.mermaidDiagramType = rendered.diagramType;

  const navigable = navigationEnabled && rendered.graph.nodes.length > 0;
  if (!navigable) {
    container.tabIndex = -1;
    return {
      ...rendered,
      navigable: false,
      cleanup() {
        container.replaceChildren();
      },
    };
  }

  const instructions = document.createElement('span');
  instructions.id = `mermaid-a11y-instructions-${++instructionsSequence}`;
  instructions.className = 'sr-only';
  instructions.textContent = getMessages(locale).keyboardInstructions;

  // Ponte temporária para mermaid-a11y#1. O nó não é uma live region e não
  // participa da árvore acessível; toda fala continua arbitrada pelo AEP-0058.
  const announcementSink = document.createElement('span');
  announcementSink.hidden = true;
  announcementSink.setAttribute('aria-hidden', 'true');

  container.append(instructions, announcementSink);

  const observer = new MutationObserver(() => {
    const message = announcementSink.textContent?.trim();
    if (message && container.contains(document.activeElement)) {
      announce(message);
    }
  });
  observer.observe(announcementSink, {
    childList: true,
    characterData: true,
    subtree: true,
  });

  const navigator: DiagramNavigator = createNavigator({
    graph: rendered.graph,
    svg: rendered.svg,
    container,
    liveRegion: announcementSink,
    instructionsId: instructions.id,
    locale,
    onBoundary: playBumpSound,
    highlightRenderer: createThemeHighlightRenderer(),
  });
  navigator.attach();

  return {
    ...rendered,
    navigable: true,
    cleanup() {
      observer.disconnect();
      navigator.detach();
      container.replaceChildren();
    },
  };
}

export function createMermaidErrorContent(
  options: MermaidErrorContentOptions,
): MermaidErrorContent {
  const {
    container,
    ariaLabel,
    title,
    message,
    detailsLabel,
    errorText,
    detailsTabbable,
  } = options;

  container.replaceChildren();
  container.classList.add('mermaid-diagram--error');
  container.setAttribute('role', 'group');
  container.setAttribute('aria-label', ariaLabel);
  container.tabIndex = -1;

  const titleElement = document.createElement('div');
  titleElement.className = 'mermaid-diagram__error-title';
  titleElement.textContent = title;

  const messageElement = document.createElement('div');
  messageElement.className = 'mermaid-diagram__error-message';
  messageElement.textContent = message;

  const details = document.createElement('details');
  details.className = 'mermaid-diagram__error-details';

  const summary = document.createElement('summary');
  summary.textContent = detailsLabel;
  summary.tabIndex = detailsTabbable ? 0 : -1;

  const errorPre = document.createElement('pre');
  errorPre.className = 'mermaid-diagram__error-pre';
  errorPre.textContent = errorText;

  details.append(summary, errorPre);
  container.append(titleElement, messageElement, details);

  return { details, errorPre };
}

function createThemeHighlightRenderer(): HighlightRenderer {
  let layer: SVGGElement | null = null;

  return {
    attach(context: HighlightContext) {
      layer = document.createElementNS('http://www.w3.org/2000/svg', 'g');
      layer.dataset.mermaidA11yHighlightLayer = 'true';
      layer.setAttribute('aria-hidden', 'true');
      layer.setAttribute('pointer-events', 'none');
      context.svg.appendChild(layer);
    },
    update(target: HighlightTarget | null) {
      layer?.replaceChildren();
      if (layer && target) {
        paintThemeHighlight(layer, target.element);
      }
    },
    detach() {
      layer?.remove();
      layer = null;
    },
  };
}

function paintThemeHighlight(layer: SVGGElement, element: Element): void {
  const path = element.tagName.toLowerCase() === 'path'
    ? element as SVGPathElement
    : element.querySelector<SVGPathElement>('path');
  if (path) {
    layer.append(
      cloneHighlightPath(path, 'mermaid-a11y-highlight-halo'),
      cloneHighlightPath(path, 'mermaid-a11y-highlight-line'),
    );
    return;
  }

  const graphics = element as SVGGraphicsElement;
  if (typeof graphics.getBBox !== 'function') return;

  try {
    const box = graphics.getBBox();
    if (!box.width && !box.height) return;
    layer.append(
      createHighlightRect(box, 7, 'mermaid-a11y-highlight-halo'),
      createHighlightRect(box, 4, 'mermaid-a11y-highlight-line'),
    );
  } catch {
    // Alguns WebViews não implementam getBBox para elementos SVG ocultos.
  }
}

function cloneHighlightPath(path: SVGPathElement, className: string): SVGPathElement {
  const clone = path.cloneNode(true) as SVGPathElement;
  clone.removeAttribute('id');
  clone.removeAttribute('style');
  clone.removeAttribute('aria-label');
  clone.removeAttribute('role');
  clone.setAttribute('class', className);
  clone.setAttribute('fill', 'none');
  clone.setAttribute('pointer-events', 'none');
  clone.setAttribute('aria-hidden', 'true');
  return clone;
}

function createHighlightRect(box: DOMRect, padding: number, className: string): SVGRectElement {
  const rect = document.createElementNS('http://www.w3.org/2000/svg', 'rect');
  rect.setAttribute('class', className);
  rect.setAttribute('x', String(box.x - padding));
  rect.setAttribute('y', String(box.y - padding));
  rect.setAttribute('width', String(Math.max(box.width + padding * 2, 8)));
  rect.setAttribute('height', String(Math.max(box.height + padding * 2, 8)));
  rect.setAttribute('rx', '4');
  rect.setAttribute('fill', 'none');
  rect.setAttribute('aria-hidden', 'true');
  return rect;
}
