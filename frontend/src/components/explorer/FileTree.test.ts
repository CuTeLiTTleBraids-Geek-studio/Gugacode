import { describe, it, expect, vi, beforeEach } from "vitest";
import { mount, flushPromises } from "@vue/test-utils";
import type { App } from "vue";
import ElementPlus from "element-plus";
import * as ElementPlusIconsVue from "@element-plus/icons-vue";
import FileTree from "./FileTree.vue";
import { fileService } from "@/api/services";
import type { DirEntry } from "@/types";

vi.mock("@/api/services", () => ({
  fileService: {
    listDirectory: vi.fn(),
    revealInOS: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("@/stores/terminal", () => ({
  createSession: vi.fn().mockResolvedValue("session-1"),
}));

vi.mock("@/stores/app", () => ({
  appState: { terminalVisible: false, currentProject: null },
}));

const iconPlugin = {
  install(app: App) {
    for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
      app.component(key, component);
    }
  },
};

function makeEntry(name: string, path: string, isDir: boolean): DirEntry {
  return { name, path, isDir, size: 0, modified: 0 };
}

function mountTree(props: Partial<{ path: string; name: string; depth: number; isDir: boolean }> = {}) {
  return mount(FileTree, {
    props: { path: "/root", name: "root", ...props },
    global: {
      plugins: [ElementPlus, iconPlugin],
    },
  });
}

describe("FileTree", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(fileService.listDirectory).mockResolvedValue([]);
  });

  it("renders the node name", () => {
    const wrapper = mountTree({ name: "my-project" });
    expect(wrapper.find(".file-tree__name").text()).toBe("my-project");
  });

  it("applies indentation based on depth", () => {
    const wrapper = mountTree({ depth: 2 });
    const row = wrapper.find(".file-tree__row");
    expect(row.attributes("style")).toContain("padding-left: 32px");
  });

  it("expands and fetches children when a folder row is clicked", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
      makeEntry("subfolder", "/root/subfolder", true),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    expect(fileService.listDirectory).toHaveBeenCalledWith("/root");
    expect(wrapper.findAll(".file-tree__children .file-tree")).toHaveLength(2);
  });

  it("shows loading state while fetching children", async () => {
    let resolveList!: (entries: DirEntry[]) => void;
    vi.mocked(fileService.listDirectory).mockReturnValue(
      new Promise<DirEntry[]>((resolve) => {
        resolveList = resolve;
      })
    );
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    expect(wrapper.find(".file-tree__loading").exists()).toBe(true);

    resolveList([]);
    await flushPromises();

    expect(wrapper.find(".file-tree__loading").exists()).toBe(false);
  });

  it("renders Folder icon for folders and Document icon for files", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
      makeEntry("subfolder", "/root/subfolder", true),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    const childWrappers = wrapper.findAllComponents(FileTree);
    const fileChild = childWrappers.find((w) => w.props("name") === "file.ts");
    const folderChild = childWrappers.find((w) => w.props("name") === "subfolder");

    expect(fileChild?.findComponent({ name: "Document" }).exists()).toBe(true);
    expect(folderChild?.findComponent({ name: "Folder" }).exists()).toBe(true);
  });

  it("emits select with the file path when a file row is clicked", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    const childRow = wrapper.findAll(".file-tree__row")[1];
    await childRow.trigger("click");

    const selectEvents = wrapper.emitted("select");
    expect(selectEvents).toBeTruthy();
    expect(selectEvents![0]).toEqual(["/root/file.ts"]);
  });

  it("expands a subfolder when its chevron is clicked", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("subfolder", "/root/subfolder", true),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();
    expect(fileService.listDirectory).toHaveBeenCalledTimes(1);

    const childChevron = wrapper.find(".file-tree__children .file-tree__chevron");
    await childChevron.trigger("click");
    await flushPromises();

    expect(fileService.listDirectory).toHaveBeenCalledWith("/root/subfolder");
    expect(fileService.listDirectory).toHaveBeenCalledTimes(2);
  });

  it("does not show a chevron for files", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    const childChevron = wrapper.find(".file-tree__children .file-tree__chevron");
    expect(childChevron.exists()).toBe(false);
  });

  it("handles fetch errors by showing an error message", async () => {
    vi.mocked(fileService.listDirectory).mockRejectedValue(new Error("permission denied"));
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    expect(wrapper.find(".file-tree__error").exists()).toBe(true);
    expect(wrapper.find(".file-tree__error").text()).toContain("permission denied");
    expect(wrapper.find(".file-tree__children").exists()).toBe(false);
  });

  it("collapses when an expanded folder is clicked again", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();
    expect(wrapper.find(".file-tree__children").exists()).toBe(true);

    await wrapper.find(".file-tree__row").trigger("click");
    expect(wrapper.find(".file-tree__children").exists()).toBe(false);
  });

  it("does not refetch children when collapsing and re-expanding", async () => {
    vi.mocked(fileService.listDirectory).mockResolvedValue([
      makeEntry("file.ts", "/root/file.ts", false),
    ]);
    const wrapper = mountTree({ path: "/root", name: "root" });

    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();
    expect(fileService.listDirectory).toHaveBeenCalledTimes(1);

    await wrapper.find(".file-tree__row").trigger("click");
    await wrapper.find(".file-tree__row").trigger("click");
    await flushPromises();

    expect(fileService.listDirectory).toHaveBeenCalledTimes(1);
  });
});
