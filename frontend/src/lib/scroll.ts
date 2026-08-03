export function scrollToBottom(node: HTMLDivElement | null): void {
  if (node === null) {
    return;
  }
  node.scrollTop = node.scrollHeight;
}
