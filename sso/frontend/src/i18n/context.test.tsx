import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { LangSwitch } from '../components/LangSwitch';
import { ThemeSwitch } from '../components/ThemeSwitch';
import { AppProvider, useApp } from './context';

/** Tiny probe that prints the current localized title + active lang/theme. */
function Probe(): JSX.Element {
  const { t, lang, theme } = useApp();
  return (
    <div>
      <span data-testid="title">{t.login_title}</span>
      <span data-testid="lang">{lang}</span>
      <span data-testid="theme">{theme}</span>
    </div>
  );
}

function setup(): void {
  render(
    <AppProvider>
      <LangSwitch />
      <ThemeSwitch />
      <Probe />
    </AppProvider>,
  );
}

describe('AppProvider — theme + language', () => {
  it('defaults to RU + dark', () => {
    setup();
    expect(screen.getByTestId('lang')).toHaveTextContent('ru');
    expect(screen.getByTestId('theme')).toHaveTextContent('dark');
    // RU default copy.
    expect(screen.getByTestId('title')).toHaveTextContent('С возвращением');
    expect(document.documentElement.getAttribute('data-theme')).toBe('dark');
    expect(document.documentElement.getAttribute('lang')).toBe('ru');
  });

  it('switches all copy when toggling the language and persists the choice', async () => {
    setup();
    await userEvent.click(screen.getByRole('button', { name: /toggle language/i }));
    expect(screen.getByTestId('lang')).toHaveTextContent('en');
    // EN copy is now rendered.
    expect(screen.getByTestId('title')).toHaveTextContent('Welcome back');
    expect(localStorage.getItem('cid_lang')).toBe('en');
  });

  it('toggles the theme, applies it to <html>, and persists the choice', async () => {
    setup();
    await userEvent.click(screen.getByRole('button', { name: /switch to light theme/i }));
    expect(screen.getByTestId('theme')).toHaveTextContent('light');
    expect(document.documentElement.getAttribute('data-theme')).toBe('light');
    expect(localStorage.getItem('cid_theme')).toBe('light');
  });

  it('restores a persisted language on mount', () => {
    localStorage.setItem('cid_lang', 'en');
    render(
      <AppProvider>
        <Probe />
      </AppProvider>,
    );
    expect(screen.getByTestId('lang')).toHaveTextContent('en');
    expect(screen.getByTestId('title')).toHaveTextContent('Welcome back');
  });
});
