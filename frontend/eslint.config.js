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

      // A política de `no-console` é definida no override estrito abaixo
      // (proíbe todo `console.*` em src, exceto no logger centralizado).

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
  {
    // Override ESTRITO: proíbe QUALQUER uso de `console.*` no app.
    // Vem depois do bloco base para sobrescrever a política anterior.
    // A exceção do logger (bloco seguinte) vem por último para vencer aqui.
    files: ['src/**/*.{ts,tsx}'],
    rules: {
      'no-console': 'error',
    },
  },
  {
    // Exceção centralizada: o logger é o ÚNICO ponto autorizado a usar console.*
    // Precisa ser o último bloco para sobrescrever o override estrito acima.
    files: ['src/utils/logger.ts', 'src/utils/logger.test.ts'],
    rules: {
      'no-console': 'off',
    },
  },
];
