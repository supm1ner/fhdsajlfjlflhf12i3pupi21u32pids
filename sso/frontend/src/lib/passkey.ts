/* ============================================================================
 * passkey.ts — small WebAuthn UX helpers shared by the login screen and the
 * passkey-management page (add-passkey-auth §D6).
 *
 * The ceremony itself (option encoding, navigator.credentials.{create,get}) is
 * handled by `@github/webauthn-json`; these helpers only classify the errors that
 * ceremony surfaces so the UI can stay quiet on a deliberate user cancellation
 * versus showing a real failure message.
 * ==========================================================================*/

/**
 * True when a WebAuthn ceremony rejection represents the user dismissing the
 * platform prompt (or the request timing out / being aborted) rather than a real
 * error. The WebAuthn spec maps "no credential chosen / user declined / timeout"
 * onto `NotAllowedError`; an in-flight `AbortController` surfaces `AbortError`.
 * In both cases we should silently stand down instead of flashing an error.
 */
export function isPasskeyCancellation(err: unknown): boolean {
  if (err instanceof DOMException) {
    return err.name === 'NotAllowedError' || err.name === 'AbortError';
  }
  // Some engines throw a plain Error whose `name` mirrors the DOMException name.
  if (err instanceof Error) {
    return err.name === 'NotAllowedError' || err.name === 'AbortError';
  }
  return false;
}
