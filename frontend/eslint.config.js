import tsParser from '@typescript-eslint/parser';
import tsPlugin from '@typescript-eslint/eslint-plugin';
import reactHooks from 'eslint-plugin-react-hooks';
import jsxA11y from 'eslint-plugin-jsx-a11y';

/**
 * ESLint v9+ usa Flat Config por padrão.
 * Regra principal aqui: impedir imports diretos de `ui/menu` fora do facade.
 */
export default [
  {
    files: ['src/**/*.{ts,tsx}'],
    languageOptions: {
      parser: tsParser,
      parserOptions: {
        ecmaVersion: 2020,
        sourceType: 'module',
        ecmaFeatures: {
          jsx: true,
        },
      },
    },
    plugins: {
      '@typescript-eslint': tsPlugin,
      'react-hooks': reactHooks,
      'jsx-a11y': jsxA11y,
    },
    rules: {
      'jsx-a11y/aria-props': 'error',
      'jsx-a11y/aria-role': 'error',
      'jsx-a11y/alt-text': 'error',
      'jsx-a11y/no-redundant-roles': 'warn',

      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],

      // Evita logs de debug no app; permite warn/error.
      // (Não quebra CI: fica como warning.)
      'no-console': ['warn', { allow: ['warn', 'error'] }],

      // Mantemos o plugin carregado (para não quebrar eslint-disable existentes),
      // mas não aplicamos as regras agora porque o repo ainda não está limpo nelas.
      'react-hooks/rules-of-hooks': 'off',
      'react-hooks/exhaustive-deps': 'off',

      // Força o app a usar o facade `src/components/menu` como ponto único.
      'no-restricted-imports': [
        'error',
        {
          patterns: ['**/ui/menu', '**/ui/menu/*'],
        },
      ],
    },
  },
  {
    files: ['src/components/menu/index.ts'],
    rules: {
      // O facade precisa importar a implementação.
      'no-restricted-imports': 'off',
    },
  },
];
