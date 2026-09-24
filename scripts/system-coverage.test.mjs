import assert from "node:assert/strict";
import { test } from "node:test";
import { summarizeProfile } from "./system-coverage.mjs";

test("system coverage counts statements rather than blocks or execution frequency", () => {
  const profile =
    "mode: atomic\nexample/auth.go:1.1,3.2 5 900\nexample/auth.go:4.1,9.2 15 0\n";
  assert.deepEqual(summarizeProfile(profile, "cluster-mode-auth"), {
    suite: "cluster-mode-auth",
    covered: 5,
    total: 20,
    percent: 25,
  });
});

test("overlapping package profiles do not inflate a repeated block", () => {
  const profile =
    "mode: atomic\nexample/a.go:1.1,3.2 5 0\nexample/a.go:1.1,3.2 5 2\nexample/a.go:4.1,5.2 5 0\n";
  assert.deepEqual(summarizeProfile(profile, "mcp-cli"), {
    suite: "mcp-cli",
    covered: 5,
    total: 10,
    percent: 50,
  });
});

test("a repeated block with a different denominator is refused", () => {
  assert.throws(
    () =>
      summarizeProfile(
        "mode: atomic\na.go:1.1,2.1 2 0\na.go:1.1,2.1 3 1\n",
        "mcp-cli",
      ),
    /conflicting statement counts/,
  );
});

test("empty, truncated and malformed profiles cannot become successful coverage reports", () => {
  for (const profile of [
    "",
    "mode: atomic\n",
    "mode: atomic\npartial",
    "mode: atomic\na.go:1.1,2.1 0 3\n",
    "mode: bad\n",
  ]) {
    assert.throws(() => summarizeProfile(profile, "mcp-cli"));
  }
});

test("a valid wholly uncovered profile is reported as zero", () => {
  assert.deepEqual(
    summarizeProfile("mode: set\na.go:1.1,2.1 4 0\n", "cluster-mode-auth"),
    {
      suite: "cluster-mode-auth",
      covered: 0,
      total: 4,
      percent: 0,
    },
  );
});

test("suite names cannot inject report rows or paths", () => {
  assert.throws(
    () => summarizeProfile("mode: atomic\na.go:1.1,2.1 4 1\n", "../other|row"),
    /stable lowercase name/,
  );
});

test("counts beyond integer precision cannot produce a coverage percentage", () => {
  assert.throws(
    () =>
      summarizeProfile(
        "mode: atomic\na.go:1.1,2.1 9007199254740993 1",
        "mcp-cli",
      ),
    /invalid coverage count/,
  );
});
