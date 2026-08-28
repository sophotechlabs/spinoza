import { describe, expect, it } from 'vitest';
import {
  cpuFromCores,
  cpuFromMilli,
  cpuPair,
  memFromBytes,
  memFromMi,
  memPair,
} from '../../src/lib/units';

describe('cpuFromMilli', () => {
  it('prints millicores and hides zero', () => {
    expect(cpuFromMilli(150)).toBe('150m');
    expect(cpuFromMilli(0)).toBe('');
  });
});

describe('cpuFromCores', () => {
  it('scales cores up to millicores and keeps zero visible', () => {
    expect(cpuFromCores(0.0284)).toBe('28m');
    expect(cpuFromCores(1.5)).toBe('1500m');
    expect(cpuFromCores(0)).toBe('0m');
  });
});

describe('memFromMi', () => {
  it('prints mebibytes, switches to gibibytes and hides zero', () => {
    expect(memFromMi(192)).toBe('192Mi');
    expect(memFromMi(2048)).toBe('2.0Gi');
    expect(memFromMi(0)).toBe('');
  });
});

describe('memFromBytes', () => {
  it('scales bytes down to MiB and switches to GiB', () => {
    expect(memFromBytes(390721536)).toBe('373 MiB');
    expect(memFromBytes(3 * 1024 * 1024 * 1024)).toBe('3.00 GiB');
    expect(memFromBytes(0)).toBe('0 MiB');
  });
});

describe('the two unit families', () => {
  it('no longer share a name, so importing the wrong one cannot silently rescale', () => {
    expect(cpuFromMilli(1500)).toBe('1500m');
    expect(cpuFromCores(1500)).toBe('1500000m');
    expect(memFromMi(1024)).toBe('1.0Gi');
    expect(memFromBytes(1024)).toBe('0 MiB');
  });
});

describe('memPair', () => {
  it('writes the unit once, not twice', () => {
    expect(memPair(9256, 13169)).toBe('9.0/12.9Gi');
  });

  it('stays in mebibytes for a node too small to round', () => {
    expect(memPair(192, 512)).toBe('192/512Mi');
  });

  // The ceiling comes from the node object, which spinoza may not have read.
  it('says what is used when there is no ceiling to read against', () => {
    expect(memPair(2048, 0)).toBe('2.0Gi');
  });
});

describe('cpuPair', () => {
  it('reads what is used against what there is', () => {
    expect(cpuPair(301, 3920)).toBe('301/3920m');
  });

  it('says what is used when there is no ceiling to read against', () => {
    expect(cpuPair(1500, 0)).toBe('1500m');
  });
});
