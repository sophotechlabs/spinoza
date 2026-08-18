export interface FitColumn {
  id: string;
  size: number;
  fixed: boolean;
  sized: boolean;
}

export type Stretch = Partial<Record<string, number>>;

function stretchable(columns: FitColumn[]): FitColumn[] {
  const loose = columns.filter((column) => !column.fixed && !column.sized);
  if (loose.length > 0) {
    return loose;
  }
  return columns.filter((column) => !column.fixed);
}

export function extraWidths(columns: FitColumn[], slack: number): Stretch {
  if (slack <= 0) {
    return {};
  }
  const chosen = stretchable(columns);
  let total = 0;
  for (const column of chosen) {
    total += column.size;
  }
  if (total <= 0) {
    return {};
  }
  const out: Stretch = {};
  let given = 0;
  chosen.forEach((column, index) => {
    let extra = Math.floor((slack * column.size) / total);
    if (index === chosen.length - 1) {
      extra = slack - given;
    }
    given += extra;
    out[column.id] = extra;
  });
  return out;
}

export function widthOf(id: string, base: number, stretch: Stretch): number {
  return base + (stretch[id] ?? 0);
}
