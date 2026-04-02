// eslint-disable-next-line @typescript-eslint/no-unused-vars
import type { AxeResults } from 'axe-core';

interface AxeMatchers {
  toHaveNoViolations(): void;
}

declare module 'vitest' {
  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  interface Assertion<T = unknown> extends AxeMatchers {}
  interface AsymmetricMatchersContaining extends AxeMatchers {}
}
