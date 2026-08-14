import { afterEach, describe, expect, it, vi } from 'vitest';
import { onControl, opensRow, selectingText } from '../../src/lib/rowClick';

function element(html: string): Element {
  const host = document.createElement('div');
  host.innerHTML = html;
  const first = host.firstElementChild;
  if (first === null) {
    throw new Error('nothing to test');
  }
  document.body.append(host);
  return first;
}

afterEach(() => {
  document.body.innerHTML = '';
  vi.restoreAllMocks();
});

describe('onControl', () => {
  it('knows a control when the click landed on one', () => {
    expect(onControl(element('<button type="button">go</button>'))).toBe(true);
    expect(onControl(element('<input type="checkbox" />'))).toBe(true);
  });

  it('knows a control when the click landed inside one', () => {
    const button = element('<button type="button"><span>go</span></button>');

    expect(onControl(button.firstElementChild)).toBe(true);
  });

  it('leaves plain content alone', () => {
    expect(onControl(element('<span>prod</span>'))).toBe(false);
  });

  it('says no for a target that is not an element at all', () => {
    expect(onControl(window)).toBe(false);
    expect(onControl(null)).toBe(false);
  });
});

describe('selectingText', () => {
  it('is true while something is highlighted', () => {
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => 'prod',
    } as unknown as Selection);

    expect(selectingText()).toBe(true);
  });

  it('is false with an empty selection', () => {
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => '',
    } as unknown as Selection);

    expect(selectingText()).toBe(false);
  });

  it('is false where there is no selection to read', () => {
    vi.spyOn(window, 'getSelection').mockReturnValue(null);

    expect(selectingText()).toBe(false);
  });
});

describe('opensRow', () => {
  it('opens on a plain cell', () => {
    expect(opensRow(element('<span>prod</span>'))).toBe(true);
  });

  it('stays shut for a control', () => {
    expect(opensRow(element('<input type="checkbox" />'))).toBe(false);
  });

  it('stays shut while text is highlighted', () => {
    vi.spyOn(window, 'getSelection').mockReturnValue({
      toString: () => 'prod',
    } as unknown as Selection);

    expect(opensRow(element('<span>prod</span>'))).toBe(false);
  });
});
