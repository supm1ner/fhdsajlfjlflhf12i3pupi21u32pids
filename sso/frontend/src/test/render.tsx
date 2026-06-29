/* Test helper: render a component wrapped in the router + app providers. */

import { render, type RenderOptions, type RenderResult } from '@testing-library/react';
import type { ReactElement, ReactNode } from 'react';
import { MemoryRouter } from 'react-router-dom';
import { AppProvider } from '../i18n/context';

interface Options extends Omit<RenderOptions, 'wrapper'> {
  route?: string;
}

export function renderWithProviders(ui: ReactElement, options: Options = {}): RenderResult {
  const { route = '/', ...rest } = options;
  const Wrapper = ({ children }: { children: ReactNode }): JSX.Element => (
    <MemoryRouter initialEntries={[route]}>
      <AppProvider>{children}</AppProvider>
    </MemoryRouter>
  );
  return render(ui, { wrapper: Wrapper, ...rest });
}
