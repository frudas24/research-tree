import { describe, expect, test } from "bun:test";
import { resolveRetreeLibraryPath, retreeLibraryNames } from "./library_path";

describe("Research Tree native library resolution", () => {
  test("uses the release DLL name on Windows", () => {
    expect(retreeLibraryNames("win32", "x64")).toEqual([
      "libretree.dll",
      "libretree-amd64.dll",
    ]);
  });

  test("accepts an explicit existing override", () => {
    const resolved = resolveRetreeLibraryPath({
      moduleDir: "/repo/third_party/retree-bridge",
      platform: "win32",
      arch: "x64",
      override: "C:\\native\\libretree.dll",
      exists: (path) => path === "C:\\native\\libretree.dll",
    });
    expect(resolved).toBe("C:\\native\\libretree.dll");
  });

  test("rejects a missing explicit override", () => {
    expect(() =>
      resolveRetreeLibraryPath({
        moduleDir: "/repo/third_party/retree-bridge",
        platform: "linux",
        arch: "x64",
        override: "/missing/libretree.so",
        exists: () => false,
      }),
    ).toThrow("RETREE_LIBRARY_PATH does not exist");
  });
});
