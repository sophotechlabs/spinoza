import { describe, expect, it } from 'vitest';
import { hasAnsi, segmentsOf, severityClass, severityOf } from '../../src/lib/logColor';

const ESC = '[';

describe('severityOf', () => {
  it('reads a json level, the shape flux controllers emit', () => {
    expect(
      severityOf('{"level":"error","ts":"2026-08-03T18:17:12.116Z","msg":"reconciliation failed"}'),
    ).toBe('error');
    expect(severityOf('{"level":"info","ts":"2026-08-03T18:17:12.116Z","msg":"in-sync"}')).toBe(
      'info',
    );
  });

  it('reads a logfmt level, the shape cilium emits', () => {
    expect(severityOf('time=2026-08-03T18:16:03Z level=warning msg="probe failed"')).toBe('warn');
    expect(severityOf('time=2026-08-03T18:16:03Z level=info msg="node addresses updated"')).toBe(
      'info',
    );
  });

  it('reads a bracketed level, the shape forgejo emits', () => {
    expect(severityOf('2026/08/03 18:17:15 ...routing/logger.go:109 [E] router failed')).toBe(
      'error',
    );
    expect(severityOf('2026/08/03 18:17:15 ...routing/logger.go:109 [W] slow query')).toBe('warn');
    expect(severityOf('2026/08/03 18:17:15 ...routing/logger.go:109 [I] completed GET')).toBe(
      'info',
    );
  });

  it('reads a bare level word, the shape keycloak emits', () => {
    expect(severityOf('2026-08-03 17:39:11,358 WARN  [org.keycloak.events] LOGIN_ERROR')).toBe(
      'warn',
    );
    expect(severityOf('2026-08-03 14:14:01,537 INFO  [org.keycloak] Bootstrap completed')).toBe(
      'info',
    );
  });

  it('treats fatal and panic as errors', () => {
    expect(severityOf('level=fatal msg="cannot bind"')).toBe('error');
    expect(severityOf('2026-08-03 PANIC runtime error')).toBe('error');
  });

  it('dims debug and trace', () => {
    expect(severityOf('level=debug msg="cache hit"')).toBe('debug');
    expect(severityOf('{"level":"trace","msg":"enter"}')).toBe('debug');
  });

  it('does not colour a line that merely mentions an error far into the message', () => {
    const line =
      'time=2026-08-03T18:16:03Z level=info msg="reconciled cleanly with no error reported at all"';

    expect(severityOf(line)).toBe('info');
  });

  it('falls back to info for a line with no level at all', () => {
    expect(severityOf('Defaulted container "forgejo" out of: forgejo, init-directories')).toBe(
      'info',
    );
  });
});

describe('severityClass', () => {
  it('reds errors, ambers warnings and leaves info alone', () => {
    expect(severityClass('error')).toBe('text-red-400');
    expect(severityClass('warn')).toBe('text-amber-400');
    expect(severityClass('debug')).toBe('text-neutral-500');
    expect(severityClass('info')).toBe('');
  });
});

describe('segmentsOf', () => {
  it('dims a leading timestamp and colours the rest by level', () => {
    const segments = segmentsOf('2026-08-03 17:39:11,358 WARN  [org.keycloak.events] denied');

    expect(segments).toHaveLength(2);
    expect(segments[0].text.trim()).toBe('2026-08-03 17:39:11,358');
    expect(segments[0].className).toBe('text-neutral-500');
    expect(segments[1].className).toBe('text-amber-400');
  });

  it('dims the logfmt time= prefix too', () => {
    const segments = segmentsOf('time=2026-08-03T18:16:03Z level=error msg="boom"');

    expect(segments[0].className).toBe('text-neutral-500');
    expect(segments[1].className).toBe('text-red-400');
  });

  it('leaves a line with no timestamp in one piece', () => {
    const segments = segmentsOf('{"level":"error","msg":"boom"}');

    expect(segments).toHaveLength(1);
    expect(segments[0].className).toBe('text-red-400');
  });

  it('keeps the whole line when there is nothing to colour', () => {
    const segments = segmentsOf('plain output');

    expect(segments).toEqual([{ text: 'plain output', className: '' }]);
  });
});

describe('a container that colours its own output', () => {
  it('is reported as carrying ansi', () => {
    expect(hasAnsi(`${ESC}31mred${ESC}0m`)).toBe(true);
    expect(hasAnsi('level=error msg="boom"')).toBe(false);
  });

  it('renders the colours it asked for instead of the escape codes', () => {
    const segments = segmentsOf(`ok ${ESC}31mfailed${ESC}0m done`);

    expect(segments).toEqual([
      { text: 'ok ', className: '' },
      { text: 'failed', className: 'text-red-400' },
      { text: ' done', className: '' },
    ]);
  });

  it('keeps a colour until it is reset', () => {
    const segments = segmentsOf(`${ESC}33mwarn one two${ESC}0m`);

    expect(segments).toEqual([{ text: 'warn one two', className: 'text-amber-400' }]);
  });

  it('follows a later colour change', () => {
    const segments = segmentsOf(`${ESC}32mgreen${ESC}36mcyan`);

    expect(segments).toEqual([
      { text: 'green', className: 'text-green-400' },
      { text: 'cyan', className: 'text-cyan-400' },
    ]);
  });

  it('ignores a code it does not map', () => {
    const segments = segmentsOf(`${ESC}1mbold${ESC}0m`);

    expect(segments).toEqual([{ text: 'bold', className: '' }]);
  });

  it('treats an empty code list as a reset', () => {
    const segments = segmentsOf(`${ESC}31mred${ESC}mplain`);

    expect(segments).toEqual([
      { text: 'red', className: 'text-red-400' },
      { text: 'plain', className: '' },
    ]);
  });

  it('drops a trailing escape with nothing after it', () => {
    const segments = segmentsOf(`text${ESC}0m`);

    expect(segments).toEqual([{ text: 'text', className: '' }]);
  });

  it('does not try to read a level out of a coloured line', () => {
    const segments = segmentsOf(`${ESC}90m2026-08-03${ESC}0m ERROR boom`);

    expect(segments[0].className).toBe('text-neutral-500');
    expect(segments[1].className).toBe('');
  });
});
