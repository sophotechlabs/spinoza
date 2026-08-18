import { describe, expect, it } from 'vitest';
import { MAX_SUGGESTIONS, suggest } from '../../src/lib/filterSuggest';
import { fieldsOf } from '../../src/lib/filterChips';
import { makeRow } from '../helpers';

const fields = fieldsOf([{ name: 'Status' }, { name: 'Node' }], true);

const clusterFields = fieldsOf([{ name: 'Status' }], false);

const rows = [
  makeRow({ uid: 'a', name: 'web-1', namespace: 'prod', cells: ['Running', 'node-1'] }),
  makeRow({ uid: 'b', name: 'web-2', namespace: 'prod', cells: ['Pending', 'node-2'] }),
  makeRow({ uid: 'c', name: 'api-1', namespace: 'staging', cells: ['Running', ''] }),
];

const namespaces = ['airbyte', 'airbyte-jobs', 'default', 'kube-system'];

function labels(text: string, held = rows, names = namespaces): string[] {
  return suggest(text, fields, held, names).map((one) => one.label);
}

describe('suggesting a field', () => {
  it('has nothing to offer for an empty box', () => {
    expect(suggest('', fields, rows, namespaces)).toEqual([]);
    expect(suggest('   ', fields, rows, namespaces)).toEqual([]);
  });

  it('offers the fields whose key starts with what was typed', () => {
    expect(labels('n')).toEqual(['Name:', 'Namespace:', 'Node:']);
  });

  it('matches a field by its label as well as its key', () => {
    expect(labels('stat')).toEqual(['Status:']);
  });

  it('completes into the key, not the label', () => {
    const [first] = suggest('stat', fields, rows, namespaces);

    expect(first).toEqual({ kind: 'field', label: 'Status:', text: 'status:', hint: 'field' });
  });

  it('offers nothing for a word that names no field', () => {
    expect(labels('web')).toEqual([]);
  });
});

describe('suggesting a value', () => {
  it('offers every namespace the cluster reported', () => {
    expect(labels('ns:')).toEqual(namespaces);
  });

  it('narrows the namespaces as the value is typed', () => {
    expect(labels('ns:airbyte-')).toEqual(['airbyte-jobs']);
  });

  it('keeps the prefix the way it was typed', () => {
    const [first] = suggest('ns:kube', fields, rows, namespaces);

    expect(first).toEqual({
      kind: 'value',
      label: 'kube-system',
      text: 'ns:kube-system',
      hint: 'Namespace',
    });
  });

  it('reads column values off the rows on screen, without repeats', () => {
    expect(labels('status:')).toEqual(['Pending', 'Running']);
  });

  it('leaves out rows that have nothing in that column', () => {
    expect(labels('node:')).toEqual(['node-1', 'node-2']);
  });

  it('offers the names of the rows on screen', () => {
    expect(labels('name:web')).toEqual(['web-1', 'web-2']);
  });

  it('offers every name that contains the value', () => {
    expect(labels('name:1')).toEqual(['api-1', 'web-1']);
  });

  it('ranks a starting match above a containing one', () => {
    const held = [
      makeRow({ uid: 'a', name: 'my-web', namespace: 'prod', cells: [] }),
      makeRow({ uid: 'b', name: 'web-1', namespace: 'prod', cells: [] }),
    ];

    expect(labels('name:web', held)).toEqual(['web-1', 'my-web']);
  });

  it('offers nothing for a field the kind does not have', () => {
    expect(suggest('ns:prod', clusterFields, rows, namespaces)).toEqual([]);
    expect(suggest('image:nginx', fields, rows, namespaces)).toEqual([]);
  });

  it('stops at a screenful', () => {
    const many = Array.from({ length: MAX_SUGGESTIONS + 5 }, (_one, index) =>
      makeRow({ uid: `u-${String(index)}`, name: `pod-${String(index)}`, namespace: 'prod' }),
    );

    expect(labels('name:pod', many)).toHaveLength(MAX_SUGGESTIONS);
  });

  it('has nothing to offer when the namespaces have not arrived', () => {
    expect(labels('ns:', rows, [])).toEqual([]);
  });
});
