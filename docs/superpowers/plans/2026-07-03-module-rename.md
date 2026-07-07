# Module Rename Implementation Plan (Q-9)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename the Go module from `changeme` to `gugacode`, recompute all 53 FNV-1a binding IDs, rename the bindings directory, and update all import paths.

**Architecture:** The Wails3 binding ID format is `{packagePath}.{TypeName}.{MethodName}` hashed with FNV-1a 32-bit. Changing the module from `changeme` to `gugacode` changes the packagePath from `changeme/services` to `gugacode/services`, which changes all 53 binding IDs. The `frontend/bindings/changeme/` directory must be renamed to `frontend/bindings/gugacode/`, and all import paths referencing it must be updated.

**Tech Stack:** Go 1.25, Wails v3 (alpha2.111), FNV-1a hash, Vue 3 + TypeScript

---

## Pre-computed Binding ID Mapping

All 53 new IDs have been pre-computed using `hash/fnv.New32a()` on `gugacode/services.{TypeName}.{MethodName}`. Verified: 51 of 53 old IDs match `changeme/services.{TypeName}.{MethodName}`. The 2 that don't (StartStream, StopStream) were previously assigned incorrect IDs — the new correct IDs fix this.

### FileService (8 methods)
| Method | Old ID | New ID |
|---|---|---|
| CreateDirectory | 2435312146 | 3651804085 |
| CreateFile | 4068495565 | 2668905352 |
| DeletePath | 1826616263 | 2928441494 |
| ListDirectory | 1062086154 | 2885314685 |
| PickDirectory | 2751859261 | 257010378 |
| ReadFile | 2912923369 | 1566684964 |
| RenamePath | 2118832546 | 4088970003 |
| WriteFile | 465925552 | 1279515011 |

### ProjectService (3 methods)
| Method | Old ID | New ID |
|---|---|---|
| AddProject | 705111184 | 1892807383 |
| GetRecentProjects | 2520918501 | 553094908 |
| RemoveProject | 115240173 | 428338444 |

### SettingsService (2 methods)
| Method | Old ID | New ID |
|---|---|---|
| LoadSettings | 1085344069 | 3130317876 |
| SaveSettings | 3472148316 | 3425851861 |

### WindowService (6 methods)
| Method | Old ID | New ID |
|---|---|---|
| Close | 3354120295 | 1238437224 |
| Maximise | 1160138902 | 98709275 |
| Minimise | 2683691908 | 3708357297 |
| SetTitle | 1891835319 | 2096623898 |
| SetWindow | 882226271 | 3118582208 |
| ToggleFullscreen | 1663021364 | 364473969 |

### TerminalService (6 methods)
| Method | Old ID | New ID |
|---|---|---|
| IsRunning | 2282587580 | 3820980655 |
| Kill | 2388417657 | 3863837304 |
| ReadOutput | 2568963780 | 1711976321 |
| Resize | 3211016577 | 3673291856 |
| Start | 986509887 | 239892036 |
| Write | 3687758780 | 3204099911 |

### AIService (9 methods)
| Method | Old ID | New ID |
|---|---|---|
| Complete | 2981282492 | 2011250937 |
| Send | 1122795881 | 3874089152 |
| SendStream | 1279532657 | 2258231168 |
| SetConfig | 3089612183 | 2620746092 |
| GetDefaultSystemPrompt | 1534773729 | 1032527000 |
| GetPresetPrompt | 1269834296 | 2172660579 |
| ListPresets | 3254381993 | 1403407850 |
| StartStream | 2834626423 | 1460695090 |
| StopStream | 3952176801 | 1510176274 |

### GitService (10 methods)
| Method | Old ID | New ID |
|---|---|---|
| Commit | 3879393956 | 3059827259 |
| GetBranchInfo | 187744619 | 140618586 |
| GetDiff | 748834820 | 2995240001 |
| GetStatus | 3474495673 | 3733983492 |
| Stage | 3272268965 | 687327160 |
| Unstage | 1174717948 | 4147888853 |
| ListBranches | 785200829 | 3898044034 |
| CreateBranch | 4256071557 | 3589253906 |
| CheckoutBranch | 523493657 | 1720605418 |
| DeleteBranch | 1768717546 | 628888929 |

### SearchService (2 methods)
| Method | Old ID | New ID |
|---|---|---|
| Search | 3723278217 | 1616798264 |
| Replace | 2022958371 | 321908540 |

### ConversationService (7 methods)
| Method | Old ID | New ID |
|---|---|---|
| Delete | 1682794813 | 3544063220 |
| GenerateConversationID | 1149103163 | 3190654506 |
| GenerateTitle | 3247645417 | 3541131574 |
| Load | 98817818 | 3620623743 |
| List | 437792510 | 1550729295 |
| UpdateTitle | 392297627 | 2756606464 |
| Save | 1629880591 | 332414834 |

---

## File Structure

- Modify: `go.mod` — module name
- Modify: `main.go` — import path
- Rename: `frontend/bindings/changeme/` → `frontend/bindings/gugacode/`
- Modify: All 9 service binding `.js` files in the renamed directory — binding IDs
- Modify: `frontend/src/api/services.ts` — import paths
- Modify: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts` — import path
- Modify: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventcreate.js` — import path

---

### Task 1: Rename Go Module

**Files:**
- Modify: `go.mod`
- Modify: `main.go`

- [ ] **Step 1: Update go.mod**

In `go.mod` line 1, change:
```
module changeme
```
to:
```
module gugacode
```

- [ ] **Step 2: Update main.go import**

In `main.go` line 8, change:
```go
	"changeme/services"
```
to:
```go
	"gugacode/services"
```

- [ ] **Step 3: Verify Go build**

Run: `cd e:\gugacode\gugacode\gugacode && go build .`
Expected: exit 0

- [ ] **Step 4: Verify Go tests**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/...`
Expected: ok gugacode/services

---

### Task 2: Rename Bindings Directory

**Files:**
- Rename: `frontend/bindings/changeme/` → `frontend/bindings/gugacode/`

- [ ] **Step 1: Copy directory**

Run PowerShell:
```powershell
Copy-Item -Path "e:\gugacode\gugacode\gugacode\frontend\bindings\changeme" -Destination "e:\gugacode\gugacode\gugacode\frontend\bindings\gugacode" -Recurse
```

- [ ] **Step 2: Verify copy**

Run: `Test-Path "e:\gugacode\gugacode\gugacode\frontend\bindings\gugacode\services\fileservice.js"`
Expected: True

- [ ] **Step 3: Delete old directory**

Run PowerShell:
```powershell
Remove-Item -Path "e:\gugacode\gugacode\gugacode\frontend\bindings\changeme" -Recurse -Force
```

---

### Task 3: Update All Binding IDs

**Files:**
- Modify: `frontend/bindings/gugacode/services/fileservice.js` (8 IDs)
- Modify: `frontend/bindings/gugacode/services/projectservice.js` (3 IDs)
- Modify: `frontend/bindings/gugacode/services/settingsservice.js` (2 IDs)
- Modify: `frontend/bindings/gugacode/services/windowservice.js` (6 IDs)
- Modify: `frontend/bindings/gugacode/services/terminalservice.js` (6 IDs)
- Modify: `frontend/bindings/gugacode/services/aiservice.js` (9 IDs)
- Modify: `frontend/bindings/gugacode/services/gitservice.js` (10 IDs)
- Modify: `frontend/bindings/gugacode/services/searchservice.js` (2 IDs)
- Modify: `frontend/bindings/gugacode/services/conversationservice.js` (7 IDs)

For each file, replace every `$Call.ByID(OLD_ID,` with `$Call.ByID(NEW_ID,` using the mapping table above.

- [ ] **Step 1: Update fileservice.js**

Replace each old ID with the new ID from the FileService table:
- `2435312146` → `3651804085` (CreateDirectory)
- `4068495565` → `2668905352` (CreateFile)
- `1826616263` → `2928441494` (DeletePath)
- `1062086154` → `2885314685` (ListDirectory)
- `2751859261` → `257010378` (PickDirectory)
- `2912923369` → `1566684964` (ReadFile)
- `2118832546` → `4088970003` (RenamePath)
- `465925552` → `1279515011` (WriteFile)

- [ ] **Step 2: Update projectservice.js**

- `705111184` → `1892807383` (AddProject)
- `2520918501` → `553094908` (GetRecentProjects)
- `115240173` → `428338444` (RemoveProject)

- [ ] **Step 3: Update settingsservice.js**

- `1085344069` → `3130317876` (LoadSettings)
- `3472148316` → `3425851861` (SaveSettings)

- [ ] **Step 4: Update windowservice.js**

- `3354120295` → `1238437224` (Close)
- `1160138902` → `98709275` (Maximise)
- `2683691908` → `3708357297` (Minimise)
- `1891835319` → `2096623898` (SetTitle)
- `882226271` → `3118582208` (SetWindow)
- `1663021364` → `364473969` (ToggleFullscreen)

- [ ] **Step 5: Update terminalservice.js**

- `2282587580` → `3820980655` (IsRunning)
- `2388417657` → `3863837304` (Kill)
- `2568963780` → `1711976321` (ReadOutput)
- `3211016577` → `3673291856` (Resize)
- `986509887` → `239892036` (Start)
- `3687758780` → `3204099911` (Write)

- [ ] **Step 6: Update aiservice.js**

- `2981282492` → `2011250937` (Complete)
- `1122795881` → `3874089152` (Send)
- `1279532657` → `2258231168` (SendStream)
- `3089612183` → `2620746092` (SetConfig)
- `1534773729` → `1032527000` (GetDefaultSystemPrompt)
- `1269834296` → `2172660579` (GetPresetPrompt)
- `3254381993` → `1403407850` (ListPresets)
- `2834626423` → `1460695090` (StartStream)
- `3952176801` → `1510176274` (StopStream)

- [ ] **Step 7: Update gitservice.js**

- `3879393956` → `3059827259` (Commit)
- `187744619` → `140618586` (GetBranchInfo)
- `748834820` → `2995240001` (GetDiff)
- `3474495673` → `3733983492` (GetStatus)
- `3272268965` → `687327160` (Stage)
- `1174717948` → `4147888853` (Unstage)
- `785200829` → `3898044034` (ListBranches)
- `4256071557` → `3589253906` (CreateBranch)
- `523493657` → `1720605418` (CheckoutBranch)
- `1768717546` → `628888929` (DeleteBranch)

- [ ] **Step 8: Update searchservice.js**

- `3723278217` → `1616798264` (Search)
- `2022958371` → `321908540` (Replace)

- [ ] **Step 9: Update conversationservice.js**

- `1682794813` → `3544063220` (Delete)
- `1149103163` → `3190654506` (GenerateConversationID)
- `3247645417` → `3541131574` (GenerateTitle)
- `98817818` → `3620623743` (Load)
- `437792510` → `1550729295` (List)
- `392297627` → `2756606464` (UpdateTitle)
- `1629880591` → `332414834` (Save)

---

### Task 4: Update Import Paths

**Files:**
- Modify: `frontend/src/api/services.ts` (9 import paths + 1 comment)
- Modify: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts`
- Modify: `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventcreate.js`

- [ ] **Step 1: Update services.ts imports**

In `frontend/src/api/services.ts`, replace all occurrences of `bindings/changeme/` with `bindings/gugacode/`.

Lines 3-12 change from:
```typescript
// and live in frontend/bindings/changeme/.
import * as FileServiceBindings from "../../bindings/changeme/services/fileservice.js";
import * as ProjectServiceBindings from "../../bindings/changeme/services/projectservice.js";
import * as SettingsServiceBindings from "../../bindings/changeme/services/settingsservice.js";
import * as WindowServiceBindings from "../../bindings/changeme/services/windowservice.js";
import * as TerminalServiceBindings from "../../bindings/changeme/services/terminalservice.js";
import * as AIServiceBindings from "../../bindings/changeme/services/aiservice.js";
import * as GitServiceBindings from "../../bindings/changeme/services/gitservice.js";
import * as SearchServiceBindings from "../../bindings/changeme/services/searchservice.js";
import * as ConversationServiceBindings from "../../bindings/changeme/services/conversationservice.js";
```
to:
```typescript
// and live in frontend/bindings/gugacode/.
import * as FileServiceBindings from "../../bindings/gugacode/services/fileservice.js";
import * as ProjectServiceBindings from "../../bindings/gugacode/services/projectservice.js";
import * as SettingsServiceBindings from "../../bindings/gugacode/services/settingsservice.js";
import * as WindowServiceBindings from "../../bindings/gugacode/services/windowservice.js";
import * as TerminalServiceBindings from "../../bindings/gugacode/services/terminalservice.js";
import * as AIServiceBindings from "../../bindings/gugacode/services/aiservice.js";
import * as GitServiceBindings from "../../bindings/gugacode/services/gitservice.js";
import * as SearchServiceBindings from "../../bindings/gugacode/services/searchservice.js";
import * as ConversationServiceBindings from "../../bindings/gugacode/services/conversationservice.js";
```

- [ ] **Step 2: Update eventdata.d.ts**

In `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventdata.d.ts` line 10, replace:
```typescript
import type * as main$0 from "../../../../../changeme/models.js";
```
with:
```typescript
import type * as main$0 from "../../../../../gugacode/models.js";
```

- [ ] **Step 3: Update eventcreate.js**

In `frontend/bindings/github.com/wailsapp/wails/v3/internal/eventcreate.js` line 11, replace:
```javascript
import * as main$0 from "../../../../../changeme/models.js";
```
with:
```javascript
import * as main$0 from "../../../../../gugacode/models.js";
```

---

### Task 5: Full Verification

- [ ] **Step 1: Go build**

Run: `cd e:\gugacode\gugacode\gugacode && go build .`
Expected: exit 0

- [ ] **Step 2: Go tests**

Run: `cd e:\gugacode\gugacode\gugacode && go test ./services/...`
Expected: ok gugacode/services

- [ ] **Step 3: Frontend type-check**

Run: `cd e:\gugacode\gugacode\gugacode\frontend && npx vue-tsc --noEmit`
Expected: exit 0

- [ ] **Step 4: Frontend tests**

Run: `cd e:\gugacode\gugacode\gugacode\frontend && npx vitest run`
Expected: All tests pass

- [ ] **Step 5: Verify no changeme references remain**

Run: `grep -rn "changeme" e:\gugacode\gugacode\gugacode\go.mod e:\gugacode\gugacode\gugacode\main.go e:\gugacode\gugacode\gugacode\frontend\src e:\gugacode\gugacode\gugacode\frontend\bindings`
Expected: No matches (excluding node_modules)

- [ ] **Step 6: Verify old bindings directory is gone**

Run: `Test-Path "e:\gugacode\gugacode\gugacode\frontend\bindings\changeme"`
Expected: False

- [ ] **Step 7: Verify new bindings directory exists**

Run: `Test-Path "e:\gugacode\gugacode\gugacode\frontend\bindings\gugacode\services\aiservice.js"`
Expected: True
