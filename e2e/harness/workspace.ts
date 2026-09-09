const LAYOUT_KEY = 'spinoza.layout.v1';

const ARRANGED_WORKSPACE = JSON.stringify({
  sizes: { left: null, right: null, bottom: null },
  collapsed: { left: false, right: false, bottom: false },
  active: { left: null, right: null, bottom: null },
  sidebar: null,
});

export async function arrangeWorkspace(baseURL: string, token: string): Promise<void> {
  const response = await fetch(`${baseURL}/api/settings`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json', 'X-Spinoza-Token': token },
    body: JSON.stringify({ values: { [LAYOUT_KEY]: ARRANGED_WORKSPACE } }),
  });
  if (!response.ok) {
    throw new Error(
      `the workspace at ${baseURL} could not be arranged: ${String(response.status)}`,
    );
  }
}
