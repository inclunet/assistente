import { describe, expect, it } from 'vitest';
import { buildChatSurfaceParams } from './chatSurface';

describe('buildChatSurfaceParams', () => {
  it('serializa state e context e preserva activeFilePath do editor', () => {
    const params = buildChatSurfaceParams(
      {
        type: 'editor',
        title: 'README',
        state: {
          filePath: '/tmp/readme.md',
          draftId: 'draft-1',
        },
      },
      {
        profileSlug: 'editor-texto',
        context: {
          selectedText: 'hello',
          selectionEmpty: false,
        },
      },
    );

    expect(params).toMatchObject({
      profileSlug: 'editor-texto',
      tabType: 'editor',
      activeFilePath: '/tmp/readme.md',
    });
    expect(JSON.parse(String(params.surfaceStateJson))).toEqual({
      filePath: '/tmp/readme.md',
      draftId: 'draft-1',
    });
    expect(JSON.parse(String(params.surfaceContextJson))).toEqual({
      selectedText: 'hello',
      selectionEmpty: false,
    });
  });

  it('omite jsons quando state/context estão vazios', () => {
    const params = buildChatSurfaceParams(
      {
        type: 'terminal',
        state: {},
      },
      {
        context: {},
      },
    );

    expect(params.tabType).toBe('terminal');
    expect(params.surfaceStateJson).toBeUndefined();
    expect(params.surfaceContextJson).toBeUndefined();
    expect(params.activeFilePath).toBeUndefined();
  });
});
