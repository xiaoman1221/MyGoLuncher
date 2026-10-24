let installed = false;

/**
 * Applies the launcher-wide content protection policy:
 * - text and images cannot be selected or dragged
 * - copy/cut, context menu and drag gestures are cancelled
 * - Ctrl/Cmd+C, X and A are suppressed
 *
 * This is a UI deterrent, not a DRM boundary: screenshots, DevTools and
 * operating-system-level capture tools cannot be prevented from a webview.
 */
export function installContentProtection() {
  if (installed || typeof document === 'undefined') return;
  installed = true;
  document.documentElement.classList.add('content-protected');

  const isControlGesture = (target: EventTarget | null) => {
    if (!(target instanceof HTMLInputElement)) return false;
    return ['range', 'checkbox', 'radio', 'color', 'file'].includes(target.type);
  };
  const block = (event: Event) => {
    // Range sliders and other native controls still need their normal drag or
    // click gestures; they do not expose selectable text.
    if (isControlGesture(event.target)) return;
    event.preventDefault();
    event.stopPropagation();
  };
  for (const type of ['copy', 'cut', 'selectstart', 'dragstart', 'contextmenu']) {
    document.addEventListener(type, block, true);
  }

  document.addEventListener('keydown', (event) => {
    if (event.isComposing || event.keyCode === 229) return;
    if (!(event.ctrlKey || event.metaKey) || event.altKey) return;
    const key = event.key.toLowerCase();
    if (key === 'c' || key === 'x' || key === 'a' || key === 'insert') {
      event.preventDefault();
      event.stopPropagation();
    }
  }, true);

  // Some WebView versions still leave a programmatic selection behind after
  // selectstart is blocked. Collapse it so no text can be copied afterwards.
  document.addEventListener('selectionchange', () => {
    const selection = window.getSelection();
    if (selection && !selection.isCollapsed) selection.removeAllRanges();
  });
}
