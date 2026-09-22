import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const testDir = dirname(fileURLToPath(import.meta.url));
const srcRoot = join(testDir, '..', '..', 'src');

const liveRegionPatterns = [
  /\baria-live\s*=/g,
  /\brole\s*=\s*(?:"(?:status|alert|log)"|'(?:status|alert|log)'|\{[^}]*["'](?:status|alert|log)["'][^}]*\})/g,
];

function listFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const fullPath = join(dir, entry);
    const stats = statSync(fullPath);
    if (stats.isDirectory()) return listFiles(fullPath);
    if (!/\.(tsx|ts)$/.test(entry) || /\.test\./.test(entry)) return [];
    return [fullPath];
  });
}

describe('debug temporário do audit (REMOVER)', () => {
  it('lista arquivos com padrão de live region', () => {
    const hits: string[] = [];
    for (const file of listFiles(srcRoot)) {
      const source = readFileSync(file, 'utf8');
      for (const pattern of liveRegionPatterns) {
        pattern.lastIndex = 0;
        for (const match of source.matchAll(pattern)) {
          const line = source.slice(0, match.index ?? 0).split(/\r?\n/).length;
          hits.push(`${relative(srcRoot, file).split(sep).join('/')}:${line}: ${match[0].replace(/\s+/g, ' ').trim()}`);
        }
      }
    }
    // eslint-disable-next-line no-console
    console.log('AUDIT-DEBUG-HITS:', JSON.stringify(hits));
    expect(true).toBe(true);
  });
});
