export function looksLikeGfmPipeTable(markdown: string): boolean {
  const text = String(markdown ?? '');
  if (!text.trim()) return false;

  const lines = text.split(/\r?\n/).map((l) => l.trim());

  // Procura um header row e um separator row em sequência.
  for (let i = 0; i < lines.length - 1; i += 1) {
    const header = lines[i];
    const sep = lines[i + 1];

    const isRow = (line: string) => line.startsWith('|') && line.endsWith('|') && line.includes('|');

    if (!isRow(header) || !isRow(sep)) continue;

    // Separador típico: | --- | --- | (aceita alinhamentos :---, ---:, :---:)
    const cells = sep
      .slice(1, -1)
      .split('|')
      .map((c) => c.trim());

    if (cells.length < 2) continue;

    const isSepCell = (c: string) => /^:?-{3,}:?$/.test(c);
    if (!cells.every(isSepCell)) continue;

    return true;
  }

  return false;
}
