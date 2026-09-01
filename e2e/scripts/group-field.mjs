import { groupByID, loadSuite } from './suite.mjs';

const id = process.argv[2];
const field = process.argv[3];
if (id === undefined || field === undefined) {
  throw new Error('usage: group-field.mjs GROUP FIELD');
}
const group = groupByID(loadSuite(), id);
const value = group[field];
if (typeof value !== 'string' && typeof value !== 'number') {
  throw new Error(`${id}.${field} is not a scalar`);
}
process.stdout.write(`${String(value)}\n`);
