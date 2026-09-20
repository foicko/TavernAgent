// @vitest-environment jsdom
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CardPreview } from "../../app/types";
import fixture from "../../../../scripts/fixtures/quality_story_card.json";

const mocks = vi.hoisted(() => ({ preview: vi.fn(), getCard: vi.fn() }));
vi.mock("../../app/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../app/api")>();
  return { ...actual, api: { ...actual.api, importCardFile: mocks.preview, getCard: mocks.getCard } };
});
import { CharacterImportModal } from "../CharacterImportModal";
import { useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";

const preview: CardPreview = {
  name: fixture.name, format: "原生角色卡", characterJson: JSON.stringify(fixture),
  supported: [], ignored: [], warnings: [], openings: fixture.openingVariants,
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}
async function upload(user: ReturnType<typeof userEvent.setup>, name = "quality.json") {
  await user.upload(document.getElementById("char-card-file-input") as HTMLInputElement,
    new File([JSON.stringify(fixture)], name, { type: "application/json" }));
}

beforeEach(() => {
  vi.resetAllMocks();
  useStory.setState(useStory.getInitialState(), true);
  useUi.setState({ ...useUi.getInitialState(), charImportOpen: true, notifyQuiet: vi.fn() }, true);
  mocks.preview.mockResolvedValue(preview);
});
afterEach(cleanup);

describe("character onboarding", () => {
  it("submits the selected opening and trimmed player identity with the complete card", async () => {
    const createSession = vi.fn().mockResolvedValue({ sessionId: "created" });
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user);
    await user.clear(screen.getByLabelText("玩家姓名"));
    await user.type(screen.getByLabelText("玩家姓名"), " 林舟 ");
    await user.type(screen.getByLabelText("身份或背景（可选）"), " 游历者 ");
    await user.selectOptions(screen.getByLabelText("选择开场"), "harbor");
    expect(screen.getByText(fixture.openingVariants[1].text)).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    expect(createSession).toHaveBeenCalledExactlyOnceWith({
      title: `${fixture.name} 的冒险`, characterJson: preview.characterJson,
      playerName: "林舟", playerRole: "游历者", openingVariantId: "harbor", openingText: undefined,
    });
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("requires a first scene when the imported card has no opening", async () => {
    mocks.preview.mockResolvedValue({ ...preview, openings: [] });
    const createSession = vi.fn().mockResolvedValue({ sessionId: "created" });
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user);
    expect((screen.getByRole("button", { name: "开启冒险" }) as HTMLButtonElement).disabled).toBe(true);
    await user.type(screen.getByLabelText("这张卡没有开场，请填写故事的第一幕"), "我们在码头重逢。");
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    expect(createSession.mock.calls[0][0]).toMatchObject({ openingText: "我们在码头重逢。", openingVariantId: undefined });
  });

  it("keeps the chosen setup after a creation failure and allows a retry", async () => {
    const createSession = vi.fn().mockRejectedValueOnce(new Error("服务暂时不可用")).mockResolvedValue({ sessionId: "created" });
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user);
    await user.selectOptions(screen.getByLabelText("选择开场"), "harbor");
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    expect((await screen.findByRole("alert")).textContent).toContain("服务暂时不可用");
    expect((screen.getByLabelText("选择开场") as HTMLSelectElement).value).toBe("harbor");
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    expect(createSession.mock.calls[1][0]).toEqual(createSession.mock.calls[0][0]);
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  });

  it("prevents duplicate creation and closing while a creation is pending", async () => {
    const pending = deferred<{ sessionId: string }>();
    const createSession = vi.fn().mockReturnValue(pending.promise);
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user);
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    const button = screen.getByRole("button", { name: "创建中…" }) as HTMLButtonElement;
    expect(button.disabled).toBe(true);
    expect((screen.getByLabelText("关闭角色导入") as HTMLButtonElement).disabled).toBe(true);
    await user.click(button);
    expect(createSession).toHaveBeenCalledTimes(1);
    await act(async () => { pending.resolve({ sessionId: "created" }); });
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("ignores late file parsing after selecting another file or closing the dialog", async () => {    const first = deferred<CardPreview>();
    const afterClose = deferred<CardPreview>();
    mocks.preview.mockReturnValueOnce(first.promise).mockResolvedValueOnce({ ...preview, name: "后选择的角色" }).mockReturnValueOnce(afterClose.promise);
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user, "first.json");
    await upload(user, "second.json");
    await act(async () => { first.resolve(preview); });
    expect(screen.getByText("解析就绪：后选择的角色")).toBeTruthy();
    await upload(user, "closed.json");
    await user.click(screen.getByLabelText("关闭角色导入"));
    await act(async () => { useUi.getState().setCharImportOpen(true); afterClose.resolve(preview); });
    expect(screen.queryByText(/解析就绪/)).toBeNull();
    expect(screen.queryByRole("button", { name: "开启冒险" })).toBeNull();
  });

  it("starts the session by cardId when the import was persisted to the server library", async () => {
    mocks.preview.mockResolvedValue({ ...preview, cardId: "card_fixture" });
    const createSession = vi.fn().mockResolvedValue({ sessionId: "created" });
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    await upload(user);
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    // 卡已在服务端：只传 cardId，不再回传兆级 characterJson。
    expect(createSession.mock.calls[0][0]).toMatchObject({ cardId: "card_fixture", characterJson: undefined });
  });

  it("prefills a library card without re-uploading the file", async () => {
    const libraryPreview: CardPreview = { ...preview, cardId: "card_lib", name: "库中角色" };
    mocks.getCard.mockResolvedValue(libraryPreview);
    useUi.setState({ charImportOpen: true, pendingCardId: "card_lib" });
    const createSession = vi.fn().mockResolvedValue({ sessionId: "created" });
    useStory.setState({ createSession });
    const user = userEvent.setup();
    render(<CharacterImportModal />);
    expect(await screen.findByText("解析就绪：库中角色")).toBeTruthy();
    expect(mocks.getCard).toHaveBeenCalledWith("card_lib");
    expect(mocks.preview).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "开启冒险" }));
    expect(createSession.mock.calls[0][0]).toMatchObject({ cardId: "card_lib" });
  });
});
