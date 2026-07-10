import { describe, it, expect } from "vitest";
import {
  normalizeCoveragePath,
  coveragePathsMatch,
  parseLcovToHits,
} from "./coverage";

describe("coverage path match (prompt-11 11-B)", () => {
  it("normalizes slashes", () => {
    expect(normalizeCoveragePath(`pkg\\a\\foo.go`).includes("\\")).toBe(false);
  });

  it("matches package-relative suffix", () => {
    expect(coveragePathsMatch("pkg/a/foo.go", "E:/proj/pkg/a/foo.go")).toBe(true);
  });

  it("does not collide on basename alone", () => {
    expect(coveragePathsMatch("pkg/a/foo.go", "E:/proj/pkg/b/foo.go")).toBe(false);
    expect(coveragePathsMatch("foo.go", "E:/proj/pkg/a/foo.go")).toBe(false);
  });

  it("parses lcov", () => {
    const hits = parseLcovToHits("SF:src/x.ts\nDA:1,1\nDA:2,0\nend_of_record\n");
    expect(hits).toHaveLength(2);
    expect(hits[0].covered).toBe(true);
    expect(hits[1].covered).toBe(false);
  });
});
