// Plan 11 Task 14 — 智能回滚前端 store。
//
// 使用 lazy bindings + 可注入 backend 模式：
//   - 生产环境懒加载 Wails bindings
//   - 测试环境通过 setSnapshotBackend 注入 mock
//
// 职责（Step 1-10）：
//   - 创建/列出/删除快照（Step 2）
//   - 选择性回滚（Step 7）
//   - 快照差异比较（Step 2: DiffSnapshots）
//   - 清理策略（Step 5）
//   - Git 集成（Step 8，后端侧）
import { reactive } from "vue";
import { notifyError, notifySuccess } from "@/lib/notifications";
import type { Snapshot, SnapshotDiff, SnapshotReason } from "@/types";

// backend 接口镜像 services/snapshot_service.go 导出方法。
interface SnapshotBackend {
  createSnapshot(workspaceRoot: string, reason: string): Promise<Snapshot>;
  restoreSnapshot(snapshotID: string, workspaceRoot: string): Promise<void>;
  restorePartial(snapshotID: string, workspaceRoot: string, filePaths: string[]): Promise<void>;
  listSnapshots(): Promise<Snapshot[]>;
  deleteSnapshot(snapshotID: string): Promise<void>;
  diffSnapshots(fromID: string, toID: string): Promise<SnapshotDiff>;
  getSnapshot(id: string): Promise<Snapshot>;
  cleanupSnapshots(keepN: number, maxAgeMs: number): Promise<number>;
}

interface SnapshotState {
  /** 快照时间线（按创建时间降序）。 */
  snapshots: Snapshot[];
  /** 当前选中的快照（详情/回滚）。 */
  selected: Snapshot | null;
  /** 两个快照之间的差异（DiffSnapshots）。 */
  diff: SnapshotDiff | null;
  /** 选择性回滚时勾选的文件路径集合（Step 7）。 */
  selectedFilePaths: Set<string>;
  /** 当前工作区根（创建快照用）。 */
  workspaceRoot: string;
  loading: boolean;
  error: string | null;
}

export const snapshotState = reactive<SnapshotState>({
  snapshots: [],
  selected: null,
  diff: null,
  selectedFilePaths: new Set(),
  workspaceRoot: "",
  loading: false,
  error: null,
});

let backend: SnapshotBackend | null = null;

// 懒加载 Wails bindings（services.SnapshotService）。
async function getBackend(): Promise<SnapshotBackend> {
  if (backend) return backend;
  // 使用字面量路径（无 @vite-ignore），让 vite 将 bindings 打包为 chunk。
  const mod = await import("../../bindings/gugacode/services/snapshotservice.js");
  backend = {
    createSnapshot: (root, reason) =>
      mod.CreateSnapshot(root, reason) as Promise<Snapshot>,
    restoreSnapshot: (id, root) =>
      mod.RestoreSnapshot(id, root) as Promise<void>,
    restorePartial: (id, root, paths) =>
      mod.RestorePartial(id, root, paths) as Promise<void>,
    listSnapshots: () => mod.ListSnapshots() as Promise<Snapshot[]>,
    deleteSnapshot: (id) => mod.DeleteSnapshot(id) as Promise<void>,
    diffSnapshots: (from, to) =>
      mod.DiffSnapshots(from, to) as Promise<SnapshotDiff>,
    getSnapshot: (id) => mod.GetSnapshot(id) as Promise<Snapshot>,
    cleanupSnapshots: (keepN, maxAgeMs) =>
      mod.CleanupSnapshots(keepN, maxAgeMs) as Promise<number>,
  };
  return backend;
}

// 测试注入。
export function setSnapshotBackend(b: SnapshotBackend): void {
  backend = b;
}

/** 设置工作区根（创建快照前调用）。 */
export function setSnapshotWorkspaceRoot(root: string): void {
  snapshotState.workspaceRoot = root;
}

// ---- Step 2: 创建快照 ----

export async function createSnapshot(
  reason: SnapshotReason = "manual",
): Promise<Snapshot | null> {
  if (!snapshotState.workspaceRoot) {
    notifyError("workspace root not set");
    return null;
  }
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    const snap = await b.createSnapshot(snapshotState.workspaceRoot, reason);
    snapshotState.snapshots.unshift(snap);
    return snap;
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
    return null;
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 2: 列出快照 ----

export async function listSnapshots(): Promise<void> {
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    snapshotState.snapshots = await b.listSnapshots();
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 2: 获取详情 ----

export async function selectSnapshot(id: string): Promise<void> {
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    snapshotState.selected = await b.getSnapshot(id);
    snapshotState.selectedFilePaths = new Set();
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 2: 删除快照 ----

export async function deleteSnapshot(id: string): Promise<void> {
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    await b.deleteSnapshot(id);
    snapshotState.snapshots = snapshotState.snapshots.filter((s) => s.id !== id);
    if (snapshotState.selected?.id === id) {
      snapshotState.selected = null;
    }
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 2: 差异比较 ----

export async function diffSnapshots(fromID: string, toID: string): Promise<void> {
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    snapshotState.diff = await b.diffSnapshots(fromID, toID);
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 7: 选择性回滚 ----

/** 切换文件勾选（Step 7）。 */
export function toggleFileSelection(path: string): void {
  if (snapshotState.selectedFilePaths.has(path)) {
    snapshotState.selectedFilePaths.delete(path);
  } else {
    snapshotState.selectedFilePaths.add(path);
  }
}

/** 全选/取消全选当前快照文件（Step 7）。 */
export function toggleSelectAllFiles(selectAll: boolean): void {
  if (!snapshotState.selected) return;
  if (selectAll) {
    snapshotState.selectedFilePaths = new Set(
      snapshotState.selected.files.map((f) => f.path),
    );
  } else {
    snapshotState.selectedFilePaths = new Set();
  }
}

/** 选择性回滚勾选的文件（Step 7）。 */
export async function restorePartial(filePaths?: string[]): Promise<boolean> {
  if (!snapshotState.selected || !snapshotState.workspaceRoot) {
    notifyError("no snapshot or workspace root");
    return false;
  }
  const paths = filePaths ?? Array.from(snapshotState.selectedFilePaths);
  if (paths.length === 0) {
    notifyError("no files selected");
    return false;
  }
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    await b.restorePartial(
      snapshotState.selected.id,
      snapshotState.workspaceRoot,
      paths,
    );
    return true;
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
    return false;
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 2: 整体回滚 ----

export async function restoreSnapshot(snapshotID: string): Promise<boolean> {
  if (!snapshotState.workspaceRoot) {
    notifyError("workspace root not set");
    return false;
  }
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    await b.restoreSnapshot(snapshotID, snapshotState.workspaceRoot);
    return true;
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
    return false;
  } finally {
    snapshotState.loading = false;
  }
}

// ---- Step 5: 清理策略 ----

export async function cleanupSnapshots(
  keepN: number,
  maxAgeMs: number,
): Promise<number> {
  snapshotState.loading = true;
  snapshotState.error = null;
  try {
    const b = await getBackend();
    const deleted = await b.cleanupSnapshots(keepN, maxAgeMs);
    if (deleted > 0) {
      notifySuccess(`cleaned up ${deleted} snapshots`);
      await listSnapshots();
    }
    return deleted;
  } catch (e: unknown) {
    snapshotState.error = e instanceof Error ? e.message : String(e);
    notifyError(snapshotState.error);
    return 0;
  } finally {
    snapshotState.loading = false;
  }
}

/** 重置 store。 */
export function resetSnapshotStore(): void {
  snapshotState.snapshots = [];
  snapshotState.selected = null;
  snapshotState.diff = null;
  snapshotState.selectedFilePaths = new Set();
  snapshotState.workspaceRoot = "";
  snapshotState.loading = false;
  snapshotState.error = null;
}
