export interface DisabledActionReason {
  id: string;
  label: string;
  reason: string | null;
}

export function describedBy(reason: string | null, id: string): string | undefined {
  if (reason === null) {
    return undefined;
  }
  return id;
}

export function actionTitle(reason: string | null, available?: string): string | undefined {
  if (reason !== null) {
    return reason;
  }
  return available;
}
