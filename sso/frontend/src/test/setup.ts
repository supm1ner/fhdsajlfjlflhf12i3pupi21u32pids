/* Vitest global setup: jest-dom matchers + per-test cleanup. */
import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/react';
import { afterEach } from 'vitest';

afterEach(() => {
  cleanup();
  // Keep theme/lang persistence isolated between tests.
  localStorage.clear();
});
