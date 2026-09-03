import { describe, expect, it } from 'vitest';
import { apiVersionLabel, entityDetails, resourceIdentity } from '../../src/lib/entityLabel';

describe('entity labels', () => {
  it('names missing group and version identity explicitly', () => {
    expect(apiVersionLabel(undefined, undefined)).toBe('core');
  });

  it('includes the selected entity details', () => {
    expect(
      entityDetails(
        {
          name: 'api',
          kind: 'Deployment',
          namespace: 'production',
          cluster: 'west',
          group: 'apps',
          version: 'v1',
        },
        { kind: true, namespace: true, cluster: true, apiVersion: true },
      ),
    ).toBe('Deployment · production · west · apps/v1');
  });

  it('keeps an unstructured resource key usable', () => {
    expect(resourceIdentity('events')).toEqual({ name: 'events' });
  });
});
