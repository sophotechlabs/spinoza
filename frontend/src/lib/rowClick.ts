const CONTROLS = 'button, input, a, select, label, textarea';

export function onControl(target: EventTarget | null): boolean {
  if (!(target instanceof Element)) {
    return false;
  }
  return target.closest(CONTROLS) !== null;
}

export function selectingText(): boolean {
  const selection = window.getSelection();
  if (selection === null) {
    return false;
  }
  return selection.toString() !== '';
}

export function opensRow(target: EventTarget | null): boolean {
  if (onControl(target)) {
    return false;
  }
  return !selectingText();
}
