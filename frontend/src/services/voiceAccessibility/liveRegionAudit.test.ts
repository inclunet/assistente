import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const allowedLiveRegionFiles = new Set([
  join('components', 'ui', 'ScreenReaderAnnouncer.tsx'),
]);

const liveRegionPatterns = [
  /\baria-live\s*=/,
  /\brole\s*=\s*(?:"(?:status|alert|log)"|'(?:status|alert|log)'|\{\s*["'](?:status|alert|log)["']\s*\})/,
];

function listProductionSourceFiles(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const fullPath = join(dir, entry);
    const stats = statSync(fullPath);

    if (stats.isDirectory()) {
      return listProductionSourceFiles(fullPath);
    }

    if (!/\.(tsx|ts)$/.test(entry) || /\.test\./.test(entry)) {
      return [];
    }

    return [fullPath];
  });
}

describe('AEP-0058 live region arbitration', () => {
  it('mantem live regions apenas no ScreenReaderAnnouncer global', () => {
    const violations = listProductionSourceFiles(srcRoot).flatMap((file) => {
      const relativePath = relative(srcRoot, file);
      if (allowedLiveRegionFiles.has(relativePath)) {
        return [];
      }

      return readFileSync(file, 'utf8')
        .split(/\r?\n/)
        .flatMap((line, index) => (
          liveRegionPatterns.some((pattern) => pattern.test(line))
            ? [`${relativePath.split(sep).join('/')}:${index + 1}: ${line.trim()}`]
            : []
        ));
    });

    expect(violations).toEqual([]);
  });
});
