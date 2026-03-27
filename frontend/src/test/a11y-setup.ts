import * as matchers from 'vitest-axe/matchers';
import { expect } from 'vitest';
import 'vitest-axe/extend-expect';

expect.extend(matchers);
