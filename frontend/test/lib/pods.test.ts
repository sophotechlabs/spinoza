import { describe, expect, it } from 'vitest';
import type { ObjectDetail } from '../../src/lib/types';
import { firstContainer, podFor, podTarget } from '../../src/lib/pods';
import { makeRow } from '../helpers';

const ref = { group: '', version: 'v1', resource: 'pods', namespace: 'prod', name: 'web' };

const detail: ObjectDetail = {
  apiVersion: 'v1',
  kind: 'Pod',
  name: 'web',
  namespace: 'prod',
  uid: 'uid-web',
  createdAt: '2026-08-03T09:00:00Z',
  containers: ['app', 'sidecar'],
  yaml: '',
};

describe('podTarget', () => {
  it('has nothing to shell into without a row', () => {
    expect(podTarget(null)).toBeNull();
  });

  it('has nothing to shell into for a row that reports no containers', () => {
    expect(podTarget(makeRow({}))).toBeNull();
  });

  it('has nothing to shell into for an empty container list', () => {
    expect(podTarget(makeRow({ containers: [] }))).toBeNull();
  });

  it('names the containers of a live row', () => {
    const row = makeRow({
      name: 'web',
      namespace: 'prod',
      containers: [{ name: 'app', state: 'running', ready: true, restarts: 0, init: false }],
    });

    expect(podTarget(row)).toEqual({ namespace: 'prod', name: 'web', containers: ['app'] });
  });
});

describe('firstContainer', () => {
  it('is empty without a pod', () => {
    expect(firstContainer(null)).toBe('');
  });

  it('is empty for a pod with no containers', () => {
    expect(firstContainer({ namespace: 'prod', name: 'web', containers: [] })).toBe('');
  });

  it('is the first one otherwise', () => {
    expect(firstContainer({ namespace: 'prod', name: 'web', containers: ['app', 'log'] })).toBe(
      'app',
    );
  });
});

describe('podFor', () => {
  it('has nothing to open without a selection', () => {
    expect(podFor(null, detail)).toBeNull();
  });

  it('prefers the live row so container state stays current', () => {
    const row = makeRow({
      name: 'web',
      namespace: 'prod',
      containers: [{ name: 'app', state: 'running', ready: true, restarts: 0, init: false }],
    });

    expect(podFor({ ref, row }, detail)?.containers).toEqual(['app']);
  });

  it('falls back to the fetched detail when no row resolved', () => {
    expect(podFor({ ref, row: null }, detail)?.containers).toEqual(['app', 'sidecar']);
  });

  it('offers nothing while the detail is still loading', () => {
    expect(podFor({ ref, row: null }, null)).toBeNull();
  });

  it('offers nothing for an object that is not a pod', () => {
    expect(podFor({ ref, row: null }, { ...detail, kind: 'Deployment' })).toBeNull();
  });

  it('offers nothing for a pod the server reports no containers for', () => {
    expect(podFor({ ref, row: null }, { ...detail, containers: undefined })).toBeNull();
  });
});
