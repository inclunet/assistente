import { describe, expect, it } from 'vitest';
import { boundedSurfaceSnapshotValue, buildChatSurfaceParams, type SurfaceContext } from './chatSurface';

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
          surfaceType: 'editor',
          surfaceId: 'draft-1',
          title: 'README',
          snapshotVersion: 'editor:draft-1:1',
          selection: {
            kind: 'text',
            text: 'hello',
            isEmpty: false,
            explicit: true,
          },
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
    expect(JSON.parse(String(params.surfaceContextJson))).toMatchObject({
      surfaceType: 'editor',
      surfaceId: 'draft-1',
      title: 'README',
      selection: {
        kind: 'text',
        text: 'hello',
        isEmpty: false,
        explicit: true,
      },
      snapshotVersion: 'editor:draft-1:1',
    });
  });

  it('omite jsons quando state/context estão vazios', () => {
    const params = buildChatSurfaceParams(
      {
        type: 'terminal',
        state: {},
      },
      {
        context: {} as SurfaceContext,
      },
    );

    expect(params.tabType).toBe('terminal');
    expect(params.surfaceStateJson).toBeUndefined();
    expect(params.surfaceContextJson).toBeUndefined();
    expect(params.activeFilePath).toBeUndefined();
  });

  it('serializa envelope SurfaceContext canônico', () => {
    const params = buildChatSurfaceParams(
      { id: 'tab-1', type: 'terminal', title: 'Terminal' },
      {
        context: {
          surfaceType: 'terminal',
          surfaceId: 'term-1',
          mode: 'shell',
          snapshotVersion: 'terminal:term-1:42',
          content: { kind: 'terminal_output', recentOutput: 'ok' },
        },
      },
    );

    expect(JSON.parse(String(params.surfaceContextJson))).toMatchObject({
      surfaceType: 'terminal',
      surfaceId: 'term-1',
      snapshotVersion: 'terminal:term-1:42',
      content: { kind: 'terminal_output', recentOutput: 'ok' },
    });
  });

  it('descarta payload incompleto em vez de adaptá-lo', () => {
    const params = buildChatSurfaceParams(
      { type: 'editor', state: { draftId: 'draft-1' } },
      {
        context: {
          selectedText: 'hello',
          selectionEmpty: true,
        } as unknown as SurfaceContext,
      },
    );

    expect(params.surfaceContextJson).toBeUndefined();
  });

  it('limita valores usados como seed de snapshot', () => {
    expect(boundedSurfaceSnapshotValue('abcdef', 6)).toBe('abcdef');
    expect(boundedSurfaceSnapshotValue('abcdef', 3)).toBe('abc:len=6');
  });
});
