/**
 * prompt-5 Task I — bindings staleness check.
 *
 * Heuristic: ensure critical Go service method names appear in the matching
 * frontend bindings TypeScript files. A full `wails3 generate` requires the
 * Wails toolchain; this gate catches accidental renames without a regen.
 *
 * Exit 0 when all required symbols are present; exit 1 otherwise.
 */
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const bindingsDir = path.join(root, "frontend", "bindings", "gugacode", "services");

/** @type {Record<string, string[]>} */
const required = {
  "aiservice.ts": ["StartStream", "StopStream", "SetConfig"],
  "windowservice.ts": ["Minimise", "Maximise", "Close", "IsMaximised"],
  "settingsservice.ts": [
    "LoadSettings",
    "SaveSettings",
    "SavePersonalizationAsset",
    "ReadPersonalizationAsset",
  ],
  "agentservice.ts": ["ExecCommand", "CheckCommand"],
  "fileservice.ts": ["ReadFile", "WriteFile", "ListDirectory"],
};

let failed = false;
for (const [file, symbols] of Object.entries(required)) {
  const full = path.join(bindingsDir, file);
  if (!fs.existsSync(full)) {
    console.error(`[bindings] missing file: ${file}`);
    failed = true;
    continue;
  }
  const text = fs.readFileSync(full, "utf8");
  for (const sym of symbols) {
    // Allow either export function Sym or export const Sym
    if (!text.includes(sym)) {
      console.error(`[bindings] ${file}: missing symbol "${sym}" — regenerate with wails3 generate`);
      failed = true;
    }
  }
}

// ByName FQN inventory: methods still called via $Call.ByName in services.ts
// prompt-6 Task 7: target is 0 ByName call sites after wails3 generate.
const servicesTs = path.join(root, "frontend", "src", "api", "services.ts");
const src = fs.readFileSync(servicesTs, "utf8");
const byName = [...src.matchAll(/\$Call\.ByName\(([^)]+)\)/g)].map((m) => m[1]);
console.log(`[bindings] ByName call sites remaining: ${byName.length}`);
if (byName.length > 0) {
  console.error(`[bindings] expected 0 ByName sites (prompt-6 Task 7), found:`);
  for (const site of byName) console.error(`  - ${site}`);
  failed = true;
}

// Ensure WindowService AI methods are in generated bindings.
const windowTs = path.join(bindingsDir, "windowservice.ts");
if (fs.existsSync(windowTs)) {
  const wtext = fs.readFileSync(windowTs, "utf8");
  for (const sym of ["OpenAIWindow", "SendSelectionToAI", "OpenPathInExplorer"]) {
    if (!wtext.includes(sym)) {
      console.error(`[bindings] windowservice.ts missing ${sym}`);
      failed = true;
    }
  }
}

if (failed) {
  process.exit(1);
}
console.log("[bindings] OK — required symbols present, ByName=0");
