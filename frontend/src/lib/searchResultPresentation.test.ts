import { describe, expect, it } from 'vitest';
import { parseSearchResultPresentation } from './searchResultPresentation';

describe('parseSearchResultPresentation', () => {
  it('aceita apenas o contrato versionado de resultados nativos', () => {
    expect(parseSearchResultPresentation(JSON.stringify({
      search_result_presentation: {
        version: 1, total: 1, truncated: false,
        items: [{ kind: 'file', title: 'app.ts', target: { kind: 'file', path: 'C:/workspace/app.ts' } }],
      },
    }))).toEqual({
      total: 1, truncated: false,
      items: [{ kind: 'file', title: 'app.ts', target: { kind: 'file', path: 'C:/workspace/app.ts' } }],
    });
  });

  it('não infere resultados de metadata arbitrário ou targets inseguros', () => {
    expect(parseSearchResultPresentation('{"items":["https://example.com"]}')).toBeUndefined();
    expect(parseSearchResultPresentation(JSON.stringify({
      search_result_presentation: {
        version: 1, total: 1, items: [{ kind: 'url', title: 'arquivo', target: { kind: 'url', url: 'file:///secret' } }],
      },
    }))).toEqual({ total: 1, truncated: false, items: [{ kind: 'url', title: 'arquivo' }] });
  });
});
