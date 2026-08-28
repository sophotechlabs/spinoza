import { beforeEach, describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import {
  FILTER_INPUT_ID,
  focusFilter,
  modLabel,
  paletteChordLabel,
  shortcuts,
  useHotkeys,
} from '../../src/lib/hotkeys';

function actions() {
  return {
    palette: vi.fn(),
    filter: vi.fn(),
    help: vi.fn(),
    close: vi.fn(),
  };
}

function press(key: string, init: KeyboardEventInit = {}, target?: EventTarget): void {
  const event = new KeyboardEvent('keydown', { key, bubbles: true, ...init });
  if (target === undefined) {
    window.dispatchEvent(event);
    return;
  }
  target.dispatchEvent(event);
}

describe('useHotkeys', () => {
  it('opens the palette on ctrl-k and on cmd-k', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });

    press('k', { ctrlKey: true });
    press('K', { metaKey: true });

    expect(handlers.palette).toHaveBeenCalledTimes(2);
  });

  it('jumps to the filter on slash and shows the list on question mark', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });

    press('/');
    press('?');

    expect(handlers.filter).toHaveBeenCalledTimes(1);
    expect(handlers.help).toHaveBeenCalledTimes(1);
  });

  it('closes on escape', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });

    press('Escape');

    expect(handlers.close).toHaveBeenCalledTimes(1);
  });

  it('leaves a typed slash alone in an input, a textarea or a select', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });
    const input = document.createElement('input');
    const textarea = document.createElement('textarea');
    const select = document.createElement('select');
    document.body.append(input, textarea, select);

    press('/', {}, input);
    press('/', {}, textarea);
    press('/', {}, select);

    expect(handlers.filter).not.toHaveBeenCalled();
    input.remove();
    textarea.remove();
    select.remove();
  });

  it('leaves a typed slash alone in a contenteditable box', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });
    const box = document.createElement('div');
    box.contentEditable = 'true';
    Object.defineProperty(box, 'isContentEditable', { value: true });
    document.body.append(box);

    press('/', {}, box);

    expect(handlers.filter).not.toHaveBeenCalled();
    box.remove();
  });

  it('still closes on escape while the caret is in an input', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });
    const input = document.createElement('input');
    document.body.append(input);

    press('Escape', {}, input);

    expect(handlers.close).toHaveBeenCalledTimes(1);
    input.remove();
  });

  it('ignores a modified slash so browser chords keep working', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });

    press('/', { altKey: true });
    press('/', { metaKey: true });
    press('?', { ctrlKey: true });

    expect(handlers.filter).not.toHaveBeenCalled();
    expect(handlers.help).not.toHaveBeenCalled();
  });

  it('ignores keys nobody bound', () => {
    const handlers = actions();
    renderHook(() => {
      useHotkeys(handlers);
    });

    press('q');

    expect(handlers.palette).not.toHaveBeenCalled();
    expect(handlers.filter).not.toHaveBeenCalled();
  });

  it('stops listening once unmounted', () => {
    const handlers = actions();
    const view = renderHook(() => {
      useHotkeys(handlers);
    });

    view.unmount();
    press('Escape');

    expect(handlers.close).not.toHaveBeenCalled();
  });

  it('calls the newest handlers, not the ones it mounted with', () => {
    const first = actions();
    const second = actions();
    const view = renderHook(
      (handlers: ReturnType<typeof actions>) => {
        useHotkeys(handlers);
      },
      { initialProps: first },
    );

    view.rerender(second);
    press('Escape');

    expect(first.close).not.toHaveBeenCalled();
    expect(second.close).toHaveBeenCalledTimes(1);
  });
});

describe('focusFilter', () => {
  beforeEach(() => {
    document.body.innerHTML = '';
  });

  it('focuses and selects the filter input when it is on screen', () => {
    const input = document.createElement('input');
    input.id = FILTER_INPUT_ID;
    input.value = 'web';
    document.body.append(input);

    focusFilter();

    expect(document.activeElement).toBe(input);
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(3);
  });

  it('does nothing when no table is showing', () => {
    expect(() => {
      focusFilter();
    }).not.toThrow();
  });

  it('does nothing when the id belongs to something that is not an input', () => {
    const div = document.createElement('div');
    div.id = FILTER_INPUT_ID;
    document.body.append(div);

    expect(() => {
      focusFilter();
    }).not.toThrow();
  });
});

describe('the shortcut list', () => {
  function onPlatform(agent: string) {
    vi.spyOn(navigator, 'userAgent', 'get').mockReturnValue(agent);
  }

  const mac = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)';
  const windows = 'Mozilla/5.0 (Windows NT 10.0; Win64; x64)';

  it('names every binding the hook implements', () => {
    onPlatform(mac);

    expect(shortcuts().map((hotkey) => hotkey.keys)).toEqual([
      '⌘K',
      '/',
      '?',
      'Esc',
      's',
      'r',
      'Shift R',
      't',
    ]);
  });

  it('spells the palette chord the way the keyboard does', () => {
    onPlatform(mac);
    expect(paletteChordLabel()).toBe('⌘K');
    expect(modLabel()).toBe('⌘');

    onPlatform(windows);
    expect(paletteChordLabel()).toBe('Ctrl K');
    expect(modLabel()).toBe('Ctrl');
  });

  it('describes each binding', () => {
    onPlatform(windows);
    expect(shortcuts()[0].description).toBe('Open the command palette');
    expect(shortcuts()[0].keys).toBe('Ctrl K');
  });
});
