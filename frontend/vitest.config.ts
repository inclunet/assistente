import { defineConfig } from 'vitest/config';
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
          'EditOutlined', 'ExclamationOutlined', 'ExportOutlined',
          'EyeInvisibleOutlined', 'EyeOutlined',
          'FileOutlined', 'FilePdfOutlined', 'FileTextOutlined', 'FilterOutlined',
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
          'ReadOutlined', 'ReloadOutlined', 'RightOutlined', 'RobotOutlined',
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

export default defineConfig({
  plugins: [antDesignIconsStub()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@wailsjs': path.resolve(__dirname, './wailsjs'),
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,tsx}'],
    clearMocks: true,
    setupFiles: ['src/vitest.setup.ts', 'src/test/a11y-setup.ts'],
    globals: true,
    testTimeout: 30000,
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
