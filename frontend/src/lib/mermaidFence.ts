export type MermaidFence = {
  index: number;
  fenceStartOffset: number;
  fenceEndOffset: number;
  codeStartOffset: number;
  codeEndOffset: number;
  code: string;
};

export function findMermaidFenceByIndex(markdown: string, targetIndex: number): MermaidFence | null {
  if (targetIndex < 0) return null;

  const startRe = /^```[ \t]*mermaid[ \t]*$/gim;
  const endRe = /^```[ \t]*$/gim;

  let currentIndex = -1;
  let startMatch: RegExpExecArray | null;

  while ((startMatch = startRe.exec(markdown))) {
    currentIndex++;

    const fenceStartOffset = startMatch.index;
    const startLineEnd = markdown.indexOf('\n', fenceStartOffset);
    if (startLineEnd === -1) return null;

    const codeStartOffset = startLineEnd + 1;

    endRe.lastIndex = codeStartOffset;
    const endMatch = endRe.exec(markdown);
    if (!endMatch) return null;

    const fenceEndLineStart = endMatch.index;
    const fenceEndLineEnd = markdown.indexOf('\n', fenceEndLineStart);
    const fenceEndOffset = fenceEndLineEnd === -1 ? markdown.length : fenceEndLineEnd + 1;

    const codeRaw = markdown.slice(codeStartOffset, fenceEndLineStart);
    const code = codeRaw.endsWith('\n') ? codeRaw.slice(0, -1) : codeRaw;

    if (currentIndex === targetIndex) {
      return {
        index: currentIndex,
        fenceStartOffset,
        fenceEndOffset,
        codeStartOffset,
        codeEndOffset: fenceEndLineStart,
        code,
      };
    }

    // Continua após o fence para evitar match dentro do mesmo bloco
    startRe.lastIndex = fenceEndOffset;
  }

  return null;
}

export function replaceMermaidFenceCode(markdown: string, fence: MermaidFence, newCode: string): string {
  const normalized = newCode.replace(/\r\n/g, '\n').replace(/\r/g, '\n');
  const withTrailingNewline = normalized.length === 0 ? '' : normalized.endsWith('\n') ? normalized : normalized + '\n';
  return markdown.slice(0, fence.codeStartOffset) + withTrailingNewline + markdown.slice(fence.codeEndOffset);
}

export function removeMermaidFence(markdown: string, fence: MermaidFence): string {
  const before = markdown.slice(0, fence.fenceStartOffset);
  const after = markdown.slice(fence.fenceEndOffset);
  return (before + after).replace(/\n{3,}/g, '\n\n');
}
