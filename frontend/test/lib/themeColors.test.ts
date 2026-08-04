import { describe, expect, it } from 'vitest';
import { ANSI_SLOTS, ansiPalette, terminalTheme } from '../../src/lib/themeColors';

describe('the ansi palette handed to xterm', () => {
  function reader(values: Record<string, string>) {
    return (name: string) => values[name] ?? '';
  }

  it('maps every slot to the css custom property that holds it', () => {
    const palette = ansiPalette(
      reader({
        '--ansi-red': '#ff0000',
        '--ansi-bright-green': '#00ff00',
        '--ansi-bright-black': 'oklch(62% 0 none)',
      }),
    );

    expect(palette.red).toBe('#ff0000');
    expect(palette.brightGreen).toBe('#00ff00');
    expect(palette.brightBlack).not.toBeUndefined();
  });

  it('converts oklch to something xterm can parse', () => {
    const palette = ansiPalette(reader({ '--ansi-green': 'oklch(79.2% 0.209 151.711)' }));

    expect(palette.green).toMatch(/^#[0-9a-f]{6}$/);
  });

  it('leaves out a slot the theme never defined', () => {
    const palette = ansiPalette(reader({}));

    expect(Object.keys(palette)).toHaveLength(0);
  });

  it('covers all sixteen slots when the theme defines them', () => {
    const values: Record<string, string> = {};
    for (const slot of ANSI_SLOTS) {
      values[`--ansi-${slot.replace(/[A-Z]/g, (letter) => `-${letter.toLowerCase()}`)}`] =
        '#123456';
    }

    expect(Object.keys(ansiPalette(reader(values)))).toHaveLength(16);
  });
});

describe('terminalTheme', () => {
  it('carries the background, foreground and the ansi palette together', () => {
    const theme = terminalTheme({ id: 'dark', name: 'Dark', base: 'dark' }, (name) =>
      name === '--ansi-red' ? '#ff0000' : '',
    );

    expect(theme.background).toBe('#0a0a0a');
    expect(theme.foreground).toBe('#d4d4d4');
    expect(theme.red).toBe('#ff0000');
  });
});
