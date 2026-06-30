// Desktop notifications for incoming messages while the tab is in the background.

export async function ensureNotifyPermission() {
  if (!('Notification' in window)) return false;
  if (Notification.permission === 'granted') return true;
  if (Notification.permission === 'denied') return false;
  try {
    return (await Notification.requestPermission()) === 'granted';
  } catch {
    return false;
  }
}

// notifyMessage shows a notification only when the tab is not focused.
export function notifyMessage(title, body, onClick) {
  if (!('Notification' in window) || Notification.permission !== 'granted') return;
  if (typeof document !== 'undefined' && !document.hidden) return;
  try {
    const n = new Notification(title, { body, icon: '/icon.svg', tag: 'sunrise-msg', renotify: true });
    if (onClick) n.onclick = () => { window.focus(); try { n.close(); } catch { /* */ } onClick(); };
  } catch { /* ignore */ }
}
