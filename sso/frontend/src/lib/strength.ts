/* ============================================================================
 * strength.ts — client-side password strength meter
 *
 * Ported VERBATIM from the prototype (_design_ref/screen-auth.jsx `strength`).
 * Returns 0..3. The client meter is ADVISORY ONLY — the backend enforces the real
 * policy (design D3); this just drives the signup UI bars/label.
 * ==========================================================================*/

/** Score a password 0..3 (weak → strong), matching the prototype heuristic. */
export function strength(pw: string): number {
  let s = 0;
  if (pw.length >= 8) s++;
  if (/[A-Z]/.test(pw) && /[a-z]/.test(pw)) s++;
  if (/\d/.test(pw)) s++;
  if (/[^A-Za-z0-9]/.test(pw)) s++;
  return Math.min(s, 3);
}

/** Bar/label colours per score index (from the prototype). */
export const STRENGTH_COLORS = [
  'hsl(350 80% 60%)',
  'hsl(350 80% 60%)',
  'hsl(40 90% 58%)',
  'hsl(150 65% 48%)',
] as const;
