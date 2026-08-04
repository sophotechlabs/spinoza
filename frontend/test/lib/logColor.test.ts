import { describe, expect, it } from 'vitest';
import {
  clockOf,
  hasAnsi,
  prettySegments,
  rawSegments,
  segmentsOf,
  severityClass,
  severityOf,
} from '../../src/lib/logColor';

function joined(segments: { text: string }[]): string {
  return segments.map((segment) => segment.text).join('');
}

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
    expect(severityClass('error')).toBe('text-error');
    expect(severityClass('warn')).toBe('text-warn');
    expect(severityClass('debug')).toBe('text-fg-subtle');
    expect(severityClass('info')).toBe('');
  });
});

describe('segmentsOf', () => {
  it('dims a leading timestamp and colours the rest by level', () => {
    const segments = segmentsOf('2026-08-03 17:39:11,358 WARN  [org.keycloak.events] denied');

    expect(segments).toHaveLength(2);
    expect(segments[0].text.trim()).toBe('2026-08-03 17:39:11,358');
    expect(segments[0].className).toBe('text-fg-subtle');
    expect(segments[1].className).toBe('text-warn');
  });

  it('dims the logfmt time= prefix too', () => {
    const segments = segmentsOf('time=2026-08-03T18:16:03Z level=error msg="boom"');

    expect(segments[0].className).toBe('text-fg-subtle');
    expect(segments[1].className).toBe('text-error');
  });

  it('leaves a line with no timestamp in one piece', () => {
    const segments = segmentsOf('{"level":"error","msg":"boom"}');

    expect(segments).toHaveLength(1);
    expect(segments[0].className).toBe('text-error');
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
      { text: 'failed', className: 'text-ansi-red' },
      { text: ' done', className: '' },
    ]);
  });

  it('keeps a colour until it is reset', () => {
    const segments = segmentsOf(`${ESC}33mwarn one two${ESC}0m`);

    expect(segments).toEqual([{ text: 'warn one two', className: 'text-ansi-yellow' }]);
  });

  it('follows a later colour change', () => {
    const segments = segmentsOf(`${ESC}32mgreen${ESC}36mcyan`);

    expect(segments).toEqual([
      { text: 'green', className: 'text-ansi-green' },
      { text: 'cyan', className: 'text-ansi-cyan' },
    ]);
  });

  it('ignores a code it does not map', () => {
    const segments = segmentsOf(`${ESC}1mbold${ESC}0m`);

    expect(segments).toEqual([{ text: 'bold', className: '' }]);
  });

  it('treats an empty code list as a reset', () => {
    const segments = segmentsOf(`${ESC}31mred${ESC}mplain`);

    expect(segments).toEqual([
      { text: 'red', className: 'text-ansi-red' },
      { text: 'plain', className: '' },
    ]);
  });

  it('drops a trailing escape with nothing after it', () => {
    const segments = segmentsOf(`text${ESC}0m`);

    expect(segments).toEqual([{ text: 'text', className: '' }]);
  });

  it('does not try to read a level out of a coloured line', () => {
    const segments = segmentsOf(`${ESC}90m2026-08-03${ESC}0m ERROR boom`);

    expect(segments[0].className).toBe('text-ansi-bright-black');
    expect(segments[1].className).toBe('');
  });
});

describe('raw view', () => {
  it('hands back the line exactly as the container wrote it', () => {
    const line = '{"level":"info","ts":"2026-08-04T11:56:53.059Z","msg":"Starting"}';

    expect(rawSegments(line)).toEqual([{ text: line, className: '' }]);
  });
});

describe('clockOf', () => {
  it('takes the wall clock out of an rfc3339 stamp', () => {
    expect(clockOf('2026-08-04T11:56:53.059Z')).toBe('11:56:53');
  });

  it('converts epoch seconds and epoch milliseconds alike', () => {
    expect(clockOf('1754308613')).toBe('11:56:53');
    expect(clockOf('1754308613059')).toBe('11:56:53');
  });

  it('gives up on a stamp it cannot read', () => {
    expect(clockOf(null)).toBe('');
    expect(clockOf('yesterday')).toBe('');
  });
});

describe('a structured log line rendered for a person', () => {
  it('leads with the clock, the level and the message', () => {
    const segments = prettySegments(
      '{"level":"info","ts":"2026-08-04T11:56:53.059Z","caller":"http/server.go:273","msg":"Starting HTTP Server.","addr":":9898"}',
    );

    expect(joined(segments)).toBe(
      '11:56:53  INFO   Starting HTTP Server.\n          caller=http/server.go:273 addr=:9898',
    );
  });

  it('colours the level and message by severity', () => {
    const segments = prettySegments('{"level":"error","msg":"boom"}');

    expect(segments[0].text).toBe('ERROR');
    expect(segments[0].className).toBe('text-error');
    expect(segments[2].className).toBe('text-error');
  });

  it('reads the level whatever the field is called and wherever it sits', () => {
    const late =
      '{"ts":"2026-08-04T11:56:53.059Z","caller":"a/very/long/package/path/server.go:273","level":"error","msg":"boom"}';

    expect(prettySegments(late)[1].className).toBe('text-error');
    expect(prettySegments('{"severity":"warn","msg":"careful"}')[0].className).toBe('text-warn');
    expect(prettySegments('{"lvl":"debug","msg":"noisy"}')[0].className).toBe('text-fg-subtle');
  });

  it('treats a level it has never seen as ordinary output', () => {
    const segments = prettySegments('{"level":"whatever","msg":"hello"}');

    expect(segments[0].text).toBe('WHATEVER');
    expect(segments[0].className).toBe('');
  });

  it('accepts the other common field names', () => {
    const segments = prettySegments(
      '{"timestamp":"2026-08-04T11:56:53Z","message":"hi","level":"info"}',
    );

    expect(joined(segments)).toBe('11:56:53  INFO   hi');
  });

  it('renders non-string fields as json rather than dropping them', () => {
    const segments = prettySegments('{"level":"info","msg":"served","code":200,"tags":["a","b"]}');

    expect(joined(segments)).toBe('INFO   served\n          code=200 tags=["a","b"]');
  });

  it('copes with a line that carries nothing but a message', () => {
    expect(joined(prettySegments('{"msg":"bare"}'))).toBe('bare');
  });

  it('copes with a line that carries no message at all', () => {
    expect(joined(prettySegments('{"level":"info"}'))).toBe('INFO   ');
  });

  it('reads a numeric timestamp, the shape zap emits by default', () => {
    const segments = prettySegments('{"level":"info","ts":1754308613,"msg":"up"}');

    expect(joined(segments)).toBe('11:56:53  INFO   up');
  });

  it('falls back to the plain view for anything that is not a json object', () => {
    expect(prettySegments('plain output')).toEqual([{ text: 'plain output', className: '' }]);
    expect(prettySegments('{"broken":')).toEqual([{ text: '{"broken":', className: '' }]);
    expect(prettySegments('{not json}')).toEqual([{ text: '{not json}', className: '' }]);
    expect(prettySegments('[1,2,3]')).toEqual([{ text: '[1,2,3]', className: '' }]);
    expect(prettySegments('null')).toEqual([{ text: 'null', className: '' }]);
  });

  it('still colours a plain text warning by its level', () => {
    expect(prettySegments('level=warn something happened')[0].className).toBe('text-warn');
  });
});
