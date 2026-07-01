import { describe, expect, it } from 'vitest';
import { readdirSync, readFileSync, statSync } from 'node:fs';
import { dirname, join, relative, sep } from 'node:path';
import { fileURLToPath } from 'node:url';

const srcRoot = join(dirname(fileURLToPath(import.meta.url)), '..', '..');
const allowedLiveRegionFiles = new Set([
  join('components', 'ui', 'ScreenReaderAnnouncer.tsx'),
]);

const liveRegionPatterns = [
  /\baria-live\s*=/g,
  /\brole\s*=\s*(?:"(?:status|alert|log)"|'(?:status|alert|log)'|\{[^}]*["'](?:status|alert|log)["'][^}]*\})/g,
];

function containsLiveRegionPattern(source: string): boolean {
  return liveRegionPatterns.some((pattern) => {
    pattern.lastIndex = 0;
    return pattern.test(source);
  });
}

function lineNumberAt(source: string, index: number): number {
  return source.slice(0, index).split(/\r?\n/).length;
}

function findLiveRegionViolations(source: string, relativePath: string): string[] {
  return liveRegionPatterns.flatMap((pattern) => {
    pattern.lastIndex = 0;
    return Array.from(source.matchAll(pattern), (match) => {
      const lineNumber = lineNumberAt(source, match.index ?? 0);
      const snippet = match[0].replace(/\s+/g, ' ').trim();
      return `${relativePath}:${lineNumber}: ${snippet}`;
    });
  });
}

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

      return findLiveRegionViolations(
        readFileSync(file, 'utf8'),
        relativePath.split(sep).join('/')
      );
    });

    expect(violations).toEqual([]);
  });

  it('detecta roles de live region em expressoes JSX', () => {
    expect(containsLiveRegionPattern('<div role={condition ? \'alert\' : undefined} />')).toBe(true);
    expect(containsLiveRegionPattern('<section role={isBusy ? "status" : "region"} />')).toBe(true);
    expect(containsLiveRegionPattern('<div role={isLog ? \'log\' : undefined} />')).toBe(true);
    expect(containsLiveRegionPattern(`<div
      role={condition
        ? 'alert'
        : undefined}
    />`)).toBe(true);
    expect(containsLiveRegionPattern('<div role={condition ? \'region\' : undefined} />')).toBe(false);
  });
});
