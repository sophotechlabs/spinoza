export const WARNING_LIMIT = 140;

export function shortened(message: string): string {
  if (message.length <= WARNING_LIMIT) {
    return message;
  }
  const cut = message.slice(0, WARNING_LIMIT);
  const stop = cut.lastIndexOf(' ');
  if (stop < 0) {
    return `${cut}…`;
  }
  return `${cut.slice(0, stop)}…`;
}
