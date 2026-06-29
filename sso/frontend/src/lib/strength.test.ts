import { describe, expect, it } from 'vitest';
import { strength, STRENGTH_COLORS } from './strength';

describe('password strength', () => {
  it('scores an empty password as 0', () => {
    expect(strength('')).toBe(0);
  });

  it('scores a short lowercase-only password as 0', () => {
    expect(strength('abc')).toBe(0);
  });

  it('gives one point for length >= 8', () => {
    // 8 lowercase chars: length bonus only.
    expect(strength('abcdefgh')).toBe(1);
  });

  it('adds a point for mixed case', () => {
    // length(1) + mixed-case(1)
    expect(strength('Abcdefgh')).toBe(2);
  });

  it('adds a point for digits', () => {
    // length(1) + mixed-case(1) + digit(1)
    expect(strength('Abcdefg1')).toBe(3);
  });

  it('caps the score at 3 even with all classes', () => {
    // length + mixed-case + digit + symbol would be 4, capped to 3.
    expect(strength('Abcdefg1!')).toBe(3);
  });

  it('exposes a colour per score index', () => {
    expect(STRENGTH_COLORS).toHaveLength(4);
    for (const c of STRENGTH_COLORS) {
      expect(c).toMatch(/^hsl\(/);
    }
  });
});
