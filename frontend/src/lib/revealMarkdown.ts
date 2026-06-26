export type RevealDetectionKind = 'markdown' | 'reveal';

export type RevealDetection = {
  kind: RevealDetectionKind;
  confidence: 'none' | 'probable' | 'strong' | 'manual';
  reason?: 'manual' | 'slideAttribute' | 'multipleSeparators';
};

export type RevealSlideLevel = 'horizontal' | 'vertical';

export type RevealSlide = {
  index: number;
  level: RevealSlideLevel;
  markdown: string;
  separatorBefore: string;
  startOffset: number;
  endOffset: number;
};

export type RevealSlideAttributes = {
  className?: string;
  data: Record<string, string>;
};

export type ParsedRevealDeck = {
  slides: RevealSlide[];
  detection: RevealDetection;
};

const SLIDE_ATTRIBUTE_RE = /<!--\s*\.(?:slide|element)\s*:/i;
const SLIDE_DIRECTIVE_RE = /<!--\s*\.slide\s*:\s*([^>]*?)-->/i;
const LEADING_SLIDE_DIRECTIVES_RE = /^(\s*<!--\s*\.slide\s*:[\s\S]*?-->\s*)+/i;
const ATTRIBUTE_RE = /([a-zA-Z_:][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
const HORIZONTAL_SEPARATOR_RE = /^\s*---\s*$/;
const VERTICAL_SEPARATOR_RE = /^\s*----\s*$/;
const FENCE_START_RE = /^(\s*)(`{3,}|~{3,})/;

function isHorizontalSeparator(line: string): boolean {
  return HORIZONTAL_SEPARATOR_RE.test(line);
}

function isVerticalSeparator(line: string): boolean {
  return VERTICAL_SEPARATOR_RE.test(line);
}

function isSlideSeparator(line: string): boolean {
  return isHorizontalSeparator(line) || isVerticalSeparator(line);
}

function hasMeaningfulContent(value: string): boolean {
  return value.trim().length > 0;
}

function getYamlFrontmatterRange(markdown: string): { start: number; end: number } | null {
  const text = String(markdown || '');
  const match = text.match(/^\s*---\s*\r?\n([\s\S]*?)\r?\n---\s*(?:\r?\n|$)/);
  if (!match) return null;
  const body = String(match[1] || '');
  if (!/^\s*[A-Za-z0-9_.-]+\s*:/m.test(body)) return null;
  return { start: 0, end: match[0].length };
}

function stripYamlFrontmatter(markdown: string): string {
  const text = String(markdown || '');
  const range = getYamlFrontmatterRange(text);
  if (!range) return text;
  return text.slice(range.end);
}

function getFenceMarker(line: string): { char: '`' | '~'; length: number } | null {
  const match = line.match(FENCE_START_RE);
  if (!match) return null;
  const marker = match[2] || '';
  const char = marker[0] as '`' | '~';
  return { char, length: marker.length };
}

function isClosingFence(line: string, fence: { char: '`' | '~'; length: number }): boolean {
  const trimmed = line.trimStart();
  const re = new RegExp(`^${fence.char === '`' ? '`' : '~'}{${fence.length},}\\s*$`);
  return re.test(trimmed);
}

function hasSlideAttributeOutsideFences(markdown: string): boolean {
  const lines = String(markdown || '').split(/\r?\n/);
  let fence: { char: '`' | '~'; length: number } | null = null;

  for (const line of lines) {
    if (fence) {
      if (isClosingFence(line, fence)) fence = null;
      continue;
    }

    const nextFence = getFenceMarker(line);
    if (nextFence) {
      fence = nextFence;
      continue;
    }

    if (SLIDE_ATTRIBUTE_RE.test(line)) return true;
  }

  return false;
}

export function detectRevealMarkdown(markdown: string, manualMode?: 'markdown' | 'reveal'): RevealDetection {
  if (manualMode === 'reveal') {
    return { kind: 'reveal', confidence: 'manual', reason: 'manual' };
  }
  if (manualMode === 'markdown') {
    return { kind: 'markdown', confidence: 'manual', reason: 'manual' };
  }

  const text = String(markdown || '');
  if (hasSlideAttributeOutsideFences(text)) {
    return { kind: 'reveal', confidence: 'strong', reason: 'slideAttribute' };
  }

  const analysisText = stripYamlFrontmatter(text);
  const parts = splitRevealSlides(analysisText);
  const nonEmptySlides = parts.filter((slide) => hasMeaningfulContent(slide.markdown));
  const separatorCount = Math.max(0, parts.length - 1);

  if (separatorCount >= 2 && nonEmptySlides.length >= 3) {
    return { kind: 'reveal', confidence: 'strong', reason: 'multipleSeparators' };
  }

  return { kind: 'markdown', confidence: 'none' };
}

export function splitRevealSlides(markdown: string): RevealSlide[] {
  const text = String(markdown || '');
  const lines = text.match(/[^\n]*(?:\n|$)/g) ?? [''];
  const normalizedLines = lines.filter((line, index) => !(line === '' && index === lines.length - 1));
  const frontmatterRange = getYamlFrontmatterRange(text);
  const contentStart = frontmatterRange?.end ?? 0;

  const slides: RevealSlide[] = [];
  let currentStart = contentStart;
  let currentSeparator: RevealSlide['separatorBefore'] = '';
  let cursor = 0;
  let slideIndex = 0;
  let fence: { char: '`' | '~'; length: number } | null = null;

  for (const line of normalizedLines) {
    if (cursor < contentStart) {
      cursor += line.length;
      continue;
    }

    const lineWithoutNewline = line.replace(/\r?\n$/, '');
    if (fence) {
      if (isClosingFence(lineWithoutNewline, fence)) fence = null;
    } else {
      const nextFence = getFenceMarker(lineWithoutNewline);
      if (nextFence) {
        fence = nextFence;
      }
    }

    if (!fence && isSlideSeparator(lineWithoutNewline)) {
      const endOffset = cursor;
      const currentMarkdown = text.slice(currentStart, endOffset).trim();
      if (currentMarkdown || currentSeparator) {
        slides.push({
          index: slideIndex,
          level: currentSeparator.trim() === '----' ? 'vertical' : 'horizontal',
          markdown: currentMarkdown,
          separatorBefore: currentSeparator,
          startOffset: currentStart,
          endOffset,
        });
        slideIndex += 1;
      }
      currentSeparator = lineWithoutNewline.trim();
      currentStart = cursor + line.length;
    }
    cursor += line.length;
  }

  slides.push({
    index: slideIndex,
    level: currentSeparator.trim() === '----' ? 'vertical' : 'horizontal',
    markdown: text.slice(currentStart).trim(),
    separatorBefore: currentSeparator,
    startOffset: currentStart,
    endOffset: text.length,
  });

  return slides;
}

export function parseRevealMarkdown(markdown: string, manualMode?: 'markdown' | 'reveal'): ParsedRevealDeck {
  const detection = detectRevealMarkdown(markdown, manualMode);
  return {
    detection,
    slides: detection.kind === 'reveal' ? splitRevealSlides(markdown) : [],
  };
}

export function replaceRevealSlide(markdown: string, slide: RevealSlide, nextSlideMarkdown: string): string {
  const text = String(markdown || '');
  const before = text.slice(0, slide.startOffset);
  const after = text.slice(slide.endOffset);
  const normalizedNext = String(nextSlideMarkdown || '').trim();
  const needsTrailingNewline = after.length > 0 && !normalizedNext.endsWith('\n') ? '\n' : '';
  return `${before}${normalizedNext}${needsTrailingNewline}${after}`;
}

export function extractRevealSlideAttributes(markdown: string): RevealSlideAttributes {
  const text = String(markdown || '');
  const match = text.match(SLIDE_DIRECTIVE_RE);
  if (!match) return { data: {} };

  const attrs = String(match[1] || '');
  const data: Record<string, string> = {};
  let className = '';

  for (const attrMatch of attrs.matchAll(ATTRIBUTE_RE)) {
    const rawName = String(attrMatch[1] || '').trim();
    const value = String(attrMatch[2] ?? attrMatch[3] ?? '').trim();
    if (!rawName || !value) continue;

    if (rawName === 'class') {
      className = value
        .split(/\s+/)
        .map((token) => token.replace(/[^\w-]/g, ''))
        .filter(Boolean)
        .join(' ');
      continue;
    }

    if (rawName.startsWith('data-')) {
      data[rawName] = value;
    }
  }

  return {
    className: className || undefined,
    data,
  };
}

export function stripRevealDirectives(markdown: string): string {
  return String(markdown || '').replace(SLIDE_DIRECTIVE_RE, '').trim();
}

export function getRevealSlideEditableMarkdown(markdown: string): string {
  return String(markdown || '').replace(LEADING_SLIDE_DIRECTIVES_RE, '').trim();
}

export function mergeRevealSlideEditableMarkdown(originalMarkdown: string, nextEditableMarkdown: string): string {
  const original = String(originalMarkdown || '');
  const match = original.match(LEADING_SLIDE_DIRECTIVES_RE);
  const prefix = match?.[0]?.trimEnd();
  const body = String(nextEditableMarkdown || '').trim();
  return prefix ? `${prefix}\n\n${body}`.trim() : body;
}

