export type MarkdownFenceMarker = {
  char: '`' | '~';
  length: number;
};

const FENCE_START_RE = /^(\s*)(`{3,}|~{3,})/;

export function getMarkdownFenceMarker(line: string): MarkdownFenceMarker | null {
  const match = line.match(FENCE_START_RE);
  if (!match) return null;
  const marker = match[2] || '';
  const char = marker[0] as '`' | '~';
  return { char, length: marker.length };
}

export function isClosingMarkdownFence(line: string, fence: MarkdownFenceMarker): boolean {
  const trimmed = line.trimStart();
  const re = new RegExp(`^${fence.char === '`' ? '`' : '~'}{${fence.length},}\\s*$`);
  return re.test(trimmed);
}
