import { describe, expect, it } from 'vitest';
import { kindLabels } from '../../src/lib/kindLabels';
import { makeDescriptor } from '../helpers';

describe('kindLabels', () => {
  it('leaves a kind alone when it stands on its own', () => {
    const labels = kindLabels([makeDescriptor({ resource: 'pods', kind: 'Pod' })]);

    expect(labels['/v1/pods']).toBe('Pod');
  });

  it('names the api group when two kinds share a name', () => {
    const labels = kindLabels([
      makeDescriptor({ group: '', version: 'v1', resource: 'events', kind: 'Event' }),
      makeDescriptor({
        group: 'events.k8s.io',
        version: 'v1',
        resource: 'events',
        kind: 'Event',
      }),
    ]);

    expect(labels['/v1/events']).toBe('Event (core)');
    expect(labels['events.k8s.io/v1/events']).toBe('Event (events.k8s.io)');
  });

  it('leaves the rest of the category alone', () => {
    const labels = kindLabels([
      makeDescriptor({ group: '', version: 'v1', resource: 'events', kind: 'Event' }),
      makeDescriptor({
        group: 'events.k8s.io',
        version: 'v1',
        resource: 'events',
        kind: 'Event',
      }),
      makeDescriptor({ resource: 'pods', kind: 'Pod' }),
    ]);

    expect(labels['/v1/pods']).toBe('Pod');
  });
});
