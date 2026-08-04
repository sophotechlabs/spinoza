export const DISCARD_QUESTION = 'You have unsaved YAML changes. Leave this object and lose them?';

let unsaved = false;

export function setUnsaved(value: boolean): void {
  unsaved = value;
}

export function hasUnsaved(): boolean {
  return unsaved;
}

export function mayDiscard(): boolean {
  if (!unsaved) {
    return true;
  }
  return window.confirm(DISCARD_QUESTION);
}
