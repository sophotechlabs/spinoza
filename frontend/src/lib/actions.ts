export type ActionTone = 'plain' | 'good' | 'caution' | 'danger';

export type ActionSize = 'normal' | 'dense';

const SHAPE = 'rounded border px-2 disabled:cursor-not-allowed disabled:border-edge';

const SIZES: Record<ActionSize, string> = {
  normal: 'py-1',
  dense: 'py-0.5',
};

const TONES: Record<ActionTone, string> = {
  plain: 'border-edge-strong text-fg hover:bg-surface-active disabled:text-fg-subtle',
  good: 'border-ok-line text-ok hover:bg-ok-tint disabled:text-fg-subtle',
  caution: 'border-warn-line text-warn hover:bg-warn-tint disabled:text-fg-subtle',
  danger: 'border-error-line text-error hover:bg-error-tint disabled:text-fg-subtle',
};

export function actionClass(tone: ActionTone = 'plain', size: ActionSize = 'normal'): string {
  return `${SHAPE} ${SIZES[size]} ${TONES[tone]}`;
}
