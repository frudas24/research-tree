import { existsSync } from "fs";
import { join } from "path";

type RuntimePlatform = "darwin" | "linux" | "win32" | string;

export interface LibraryPathOptions {
  moduleDir: string;
  platform: RuntimePlatform;
  arch: string;
  override?: string;
  exists?: (path: string) => boolean;
}

export function retreeLibraryNames(platform: RuntimePlatform, arch: string): string[] {
  if (platform === "win32") {
    const architectureName = arch === "arm64" ? "libretree-arm64.dll" : "libretree-amd64.dll";
    return ["libretree.dll", architectureName];
  }
  if (platform === "darwin") {
    return ["libretree.dylib"];
  }
  return ["libretree.so"];
}

export function resolveRetreeLibraryPath(options: LibraryPathOptions): string {
  const pathExists = options.exists ?? existsSync;
  if (options.override) {
    if (pathExists(options.override)) {
      return options.override;
    }
    throw new Error(`RETREE_LIBRARY_PATH does not exist: ${options.override}`);
  }

  const names = retreeLibraryNames(options.platform, options.arch);
  const directories = [
    join(options.moduleDir, "..", "..", "build"),
    join(options.moduleDir, "..", "..", "dist"),
    options.moduleDir,
  ];
  for (const directory of directories) {
    for (const name of names) {
      const candidate = join(directory, name);
      if (pathExists(candidate)) {
        return candidate;
      }
    }
  }

  throw new Error(
    `Research Tree native library not found for ${options.platform}/${options.arch}. ` +
      "Set RETREE_LIBRARY_PATH or build the platform bridge.",
  );
}
