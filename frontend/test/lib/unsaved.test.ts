import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { DISCARD_QUESTION, hasUnsaved, mayDiscard, setUnsaved } from '../../src/lib/unsaved';

beforeEach(() => {
  setUnsaved(false);
});

afterEach(() => {
  setUnsaved(false);
  vi.unstubAllGlobals();
});

describe('the unsaved-edit guard', () => {
  it('starts clean', () => {
    expect(hasUnsaved()).toBe(false);
  });

  it('remembers that an editor went dirty', () => {
    setUnsaved(true);

    expect(hasUnsaved()).toBe(true);
  });

  it('lets you leave when nothing is unsaved, without asking', () => {
    const confirm = vi.fn().mockReturnValue(false);
    vi.stubGlobal('confirm', confirm);

    expect(mayDiscard()).toBe(true);
    expect(confirm).not.toHaveBeenCalled();
  });

  it('asks before losing an edit and lets you through when you agree', () => {
    setUnsaved(true);
    const confirm = vi.fn().mockReturnValue(true);
    vi.stubGlobal('confirm', confirm);

    expect(mayDiscard()).toBe(true);
    expect(confirm).toHaveBeenCalledWith(DISCARD_QUESTION);
  });

  it('holds you there when you say no', () => {
    setUnsaved(true);
    vi.stubGlobal('confirm', vi.fn().mockReturnValue(false));

    expect(mayDiscard()).toBe(false);
  });
});
