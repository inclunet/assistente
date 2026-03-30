import type { AxeResults } from 'axe-core';

interface AxeMatchers {
  toHaveNoViolations(): void;
}

declare module 'vitest' {
  interface Assertion<T = any> extends AxeMatchers {}
  interface AsymmetricMatchersContaining extends AxeMatchers {}
}
