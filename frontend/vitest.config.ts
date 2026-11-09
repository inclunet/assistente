import { configDefaults, defineConfig } from 'vitest/config';
import path from 'path';

/**
 * Vite plugin that replaces @ant-design/icons with a lightweight stub.
 * The real barrel re-exports ~700 icon components and takes 60s+ to
 * transform on a cold vitest cache, causing test timeouts in CI.
 */
function antDesignIconsStub() {
  const VIRTUAL_ID = '\0@ant-design/icons-stub';
  return {
    name: 'ant-design-icons-stub',
    enforce: 'pre' as const,
    resolveId(id: string) {
      if (id === '@ant-design/icons' || id.startsWith('@ant-design/icons/')) {
        return VIRTUAL_ID;
      }
    },
    load(id: string) {
      if (id !== VIRTUAL_ID) return undefined;
      return [
        'const S = () => null;',
        'export default {};',
        // Named exports for every icon used in the codebase.
        // When a new icon is added, append it here.
        ...[
          'ApartmentOutlined', 'ApiOutlined', 'AppstoreOutlined', 'ArrowLeftOutlined', 'AudioOutlined',
          'BarChartOutlined', 'BugOutlined', 'BulbOutlined',
          'CalendarOutlined', 'CaretRightOutlined', 'CheckCircleOutlined',
          'CheckOutlined', 'CheckSquareOutlined', 'ClearOutlined',
          'CloseCircleOutlined', 'CloseOutlined', 'CodeOutlined',
          'CompassOutlined', 'CompressOutlined', 'ConsoleSqlOutlined', 'CopyOutlined',
          'CustomerServiceOutlined',
          'DeleteOutlined', 'DownOutlined',
          'EditOutlined', 'ExclamationCircleOutlined', 'ExclamationOutlined', 'ExportOutlined',
          'EyeInvisibleOutlined', 'EyeOutlined',
          'FileOutlined', 'FilePdfOutlined', 'FileTextOutlined', 'FilterOutlined',
          'FullscreenOutlined',
          'FolderOpenOutlined', 'FolderOutlined',
          'HistoryOutlined',
          'ImportOutlined', 'InfoCircleOutlined', 'InfoOutlined',
          'InteractionOutlined',
          'KeyOutlined',
          'LeftOutlined', 'LinkOutlined', 'LoadingOutlined', 'LockOutlined',
          'MenuOutlined', 'MessageOutlined', 'MobileOutlined',
          'PaperClipOutlined', 'PauseCircleOutlined', 'PlayCircleOutlined',
          'PlusOutlined',
          'QuestionCircleOutlined',
          'ReadOutlined', 'ReloadOutlined', 'RightOutlined', 'RobotOutlined', 'UndoOutlined',
          'SafetyOutlined', 'SaveOutlined', 'SendOutlined', 'SettingOutlined',
          'SlidersOutlined', 'SoundOutlined', 'StarFilled', 'StarOutlined',
          'StopOutlined',
          'ThunderboltOutlined', 'ToolOutlined',
          'UnorderedListOutlined', 'UpOutlined', 'UserSwitchOutlined',
          'WarningOutlined',
          'ZoomInOutlined', 'ZoomOutOutlined',
        ].map(name => `export const ${name} = S;`),
      ].join('\n');
    },
  };
}

// Somente provas de lógica sem DOM. Novos testes continuam no ambiente de UI
// até que suas dependências sejam qualificadas para Node.
const nodeTestFiles = [
  'src/components/ui/tabs/keyboard.test.ts',
  'src/hooks/useDefaultFocus.test.ts',
  'src/lib/chatMessageAriaLabel.test.ts',
  'src/lib/chatSurface.test.ts',
  'src/lib/commandAppPage.test.ts',
  'src/lib/commandBridge.test.ts',
  'src/lib/commandChatSurface.test.ts',
  'src/lib/commandEditorTransfer.test.ts',
  'src/lib/commandLocalPaletteConditions.test.ts',
  'src/lib/commandTaskListSurface.test.ts',
  'src/lib/commandTerminalSurface.test.ts',
  'src/lib/dateUtils.test.ts',
  'src/lib/editorDialogLabels.test.ts',
  'src/lib/editorInsertNormalize.test.ts',
  'src/lib/editorPatch.test.ts',
  'src/lib/editorPatchApply.test.ts',
  'src/lib/editorSendMenu.test.ts',
  'src/lib/externalUICommandDispatcher.test.ts',
  'src/lib/formValidation.test.ts',
  'src/lib/linkPaste.test.ts',
  'src/lib/markdownTables.test.ts',
  'src/lib/mermaidFence.test.ts',
  'src/lib/messageContentLimit.test.ts',
  'src/lib/monacoInsertHeuristics.test.ts',
  'src/lib/profileDescription.test.ts',
  'src/lib/questionnaireClosed.test.ts',
  'src/lib/revealMarkdown.test.ts',
  'src/lib/richTextPatchApply.test.ts',
  'src/lib/richTextSelectionValidation.test.ts',
  'src/lib/safeLink.test.ts',
  'src/lib/searchResultPresentation.test.ts',
  'src/lib/serialSaveQueue.test.ts',
  'src/lib/speechEvents.test.ts',
  'src/lib/stripMarkdown.test.ts',
  'src/lib/taskListConfigConflict.test.ts',
  'src/lib/toolDetailSanitization.test.ts',
  'src/lib/toolPresentation.test.ts',
  'src/lib/workspaceSnapshot.test.ts',
  'src/pages/editorInlineChatContext.test.ts',
  'src/pages/editorTextRange.test.ts',
  'src/services/chatProgressAnnouncer.test.ts',
  'src/services/chatSessionRegistry.test.ts',
  'src/services/chatTurnQueue.test.ts',
  'src/services/externalUIConnection.test.ts',
  'src/services/voiceAccessibility/liveRegionAudit.test.ts',
  'src/services/voiceAccessibility/types.test.ts',
  'src/test/zustandSelectors.test.ts',
  'src/utils/format.test.ts',
  'src/utils/logger.test.ts',
  'src/utils/path.test.ts',
  'src/utils/toolPolicyMatcher.test.ts',
  'src/utils/toolSource.test.ts',
];

export default defineConfig({
  plugins: [antDesignIconsStub()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': path.resolve(__dirname, './wailsjs'),
    },
  },
  test: {
    clearMocks: true,
    globals: true,
    testTimeout: 30000,
    projects: [
      {
        extends: true,
        test: {
          name: 'logic',
          environment: 'node',
          include: nodeTestFiles,
        },
      },
      {
        extends: true,
        test: {
          name: 'ui',
          environment: 'jsdom',
          include: ['src/**/*.{test,spec}.{ts,tsx}'],
          exclude: [...configDefaults.exclude, ...nodeTestFiles],
          setupFiles: ['src/vitest.setup.ts', 'src/test/a11y-setup.ts'],
        },
      },
    ],
    server: {
      deps: {
        // Force @ant-design/icons through the Vite pipeline so the
        // antDesignIconsStub plugin can intercept it. Without this,
        // vitest externalizes node_modules and uses native Node.js
        // import, bypassing plugins entirely.
        inline: ['@ant-design/icons'],
      },
    },
  },
});
