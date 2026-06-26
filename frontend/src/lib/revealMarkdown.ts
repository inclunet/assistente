export type RevealDetectionKind = 'markdown' | 'reveal';

export type RevealDetection = {
  kind: RevealDetectionKind;
  confidence: 'none' | 'probable' | 'strong' | 'manual';
  reason?: 'manual' | 'slideAttribute' | 'multipleSeparators' | 'notesWithSeparators';
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
const ATTRIBUTE_RE = /([a-zA-Z_:][\w:.-]*)\s*=\s*(?:"([^"]*)"|'([^']*)')/g;
const HORIZONTAL_SEPARATOR_RE = /^\s*---\s*$/;
const VERTICAL_SEPARATOR_RE = /^\s*----\s*$/;
const NOTE_RE = /^\s*Note:\s*$/im;

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

function stripYamlFrontmatter(markdown: string): string {
  const text = String(markdown || '');
  const match = text.match(/^\s*---\s*\r?\n[\s\S]*?\r?\n---\s*(?:\r?\n|$)/);
  if (!match) return text;
  return text.slice(match[0].length);
}

export function detectRevealMarkdown(markdown: string, manualMode?: 'markdown' | 'reveal'): RevealDetection {
  if (manualMode === 'reveal') {
    return { kind: 'reveal', confidence: 'manual', reason: 'manual' };
  }
  if (manualMode === 'markdown') {
    return { kind: 'markdown', confidence: 'manual', reason: 'manual' };
  }

  const text = String(markdown || '');
  if (SLIDE_ATTRIBUTE_RE.test(text)) {
    return { kind: 'reveal', confidence: 'strong', reason: 'slideAttribute' };
  }

  const analysisText = stripYamlFrontmatter(text);
  const parts = splitRevealSlides(analysisText);
  const nonEmptySlides = parts.filter((slide) => hasMeaningfulContent(slide.markdown));
  const separatorCount = Math.max(0, parts.length - 1);

  if (separatorCount >= 2 && nonEmptySlides.length >= 3) {
    return { kind: 'reveal', confidence: 'strong', reason: 'multipleSeparators' };
  }

  if (separatorCount >= 1 && nonEmptySlides.length >= 2 && NOTE_RE.test(text)) {
    return { kind: 'reveal', confidence: 'probable', reason: 'notesWithSeparators' };
  }

  return { kind: 'markdown', confidence: 'none' };
}

export function splitRevealSlides(markdown: string): RevealSlide[] {
  const text = String(markdown || '');
  const lines = text.match(/[^\n]*(?:\n|$)/g) ?? [''];
  const normalizedLines = lines.filter((line, index) => !(line === '' && index === lines.length - 1));

  const slides: RevealSlide[] = [];
  let currentStart = 0;
  let currentSeparator: RevealSlide['separatorBefore'] = '';
  let cursor = 0;
  let slideIndex = 0;

  for (const line of normalizedLines) {
    const lineWithoutNewline = line.replace(/\r?\n$/, '');
    if (isSlideSeparator(lineWithoutNewline)) {
      const endOffset = cursor;
      slides.push({
        index: slideIndex,
        level: currentSeparator.trim() === '----' ? 'vertical' : 'horizontal',
        markdown: text.slice(currentStart, endOffset).trim(),
        separatorBefore: currentSeparator,
        startOffset: currentStart,
        endOffset,
      });
      slideIndex += 1;
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

