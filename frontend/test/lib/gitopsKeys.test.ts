import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';
import { useGitopsKeys } from '../../src/lib/gitopsKeys';

function press(key: string, init: KeyboardEventInit = {}) {
  window.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, ...init }));
}

function bind() {
  const keys = {
    sync: vi.fn(),
    refresh: vi.fn(),
    deepRefresh: vi.fn(),
    terminate: vi.fn(),
  };
  const view = renderHook(() => {
    useGitopsKeys(keys);
  });
  return { keys, view };
}

describe('the gitops key bindings', () => {
  it('syncs on s', () => {
    const { keys } = bind();

    press('s');

    expect(keys.sync).toHaveBeenCalled();
  });

  it('refreshes on r and refreshes harder on shift r', () => {
    const { keys } = bind();

    press('r');
    press('R');

    expect(keys.refresh).toHaveBeenCalledOnce();
    expect(keys.deepRefresh).toHaveBeenCalledOnce();
  });

  it('terminates on t', () => {
    const { keys } = bind();

    press('t');

    expect(keys.terminate).toHaveBeenCalled();
  });

  it('takes the shifted letters too', () => {
    const { keys } = bind();

    press('S');
    press('T');

    expect(keys.sync).toHaveBeenCalled();
    expect(keys.terminate).toHaveBeenCalled();
  });

  it('leaves a key it does not know alone', () => {
    const { keys } = bind();

    press('q');

    expect(keys.sync).not.toHaveBeenCalled();
  });

  it('stays out of the way while a modifier is held', () => {
    const { keys } = bind();

    press('s', { metaKey: true });

    expect(keys.sync).not.toHaveBeenCalled();
  });

  it('stays out of the way while someone is typing', () => {
    const { keys } = bind();
    const input = document.createElement('input');
    document.body.append(input);
    input.focus();

    input.dispatchEvent(new KeyboardEvent('keydown', { key: 's', bubbles: true }));

    expect(keys.sync).not.toHaveBeenCalled();
    input.remove();
  });

  it('does nothing for a verb the object does not offer', () => {
    renderHook(() => {
      useGitopsKeys({ sync: undefined });
    });

    press('s');
    press('t');
  });

  it('stops listening once it is gone', () => {
    const { keys, view } = bind();
    view.unmount();

    press('s');

    expect(keys.sync).not.toHaveBeenCalled();
  });
});
