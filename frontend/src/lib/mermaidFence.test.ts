import { describe, expect, it } from 'vitest';
import { findMermaidFenceByIndex, replaceMermaidFenceCode, removeMermaidFence } from './mermaidFence';

describe('mermaidFence', () => {
  it('encontra fence por indice', () => {
    const md = '```mermaid\nA-->B\n```\n\n```mermaid\nC-->D\n```\n';
    const fence = findMermaidFenceByIndex(md, 1);

    expect(fence?.code).toBe('C-->D');
  });

  it('substitui codigo do fence', () => {
    const md = '```mermaid\nA-->B\n```\n';
    const fence = findMermaidFenceByIndex(md, 0)!;
    const next = replaceMermaidFenceCode(md, fence, 'X');

    expect(next).toContain('```mermaid');
    expect(next).toContain('X');
  });

  it('remove fence', () => {
    const md = 'Antes\n\n```mermaid\nA-->B\n```\n\nDepois';
    const fence = findMermaidFenceByIndex(md, 0)!;
    const next = removeMermaidFence(md, fence);

    expect(next).toContain('Antes');
    expect(next).toContain('Depois');
    expect(next).not.toContain('mermaid');
  });
});
