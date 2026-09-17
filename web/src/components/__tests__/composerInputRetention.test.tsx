// @vitest-environment jsdom
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ComposerShelf } from "../ComposerShelf";
import { NarrativeStream } from "../NarrativeStream";
import { recoveryView } from "../../stores/__tests__/turnRecoveryFixtures";

const mocks = vi.hoisted(() => ({
  api: { getSession: vi.fn(), listMemories: vi.fn(), acceptTurn: vi.fn(), getTurn: vi.fn() },
  turn: vi.fn(), session: vi.fn(),
}));
vi.mock("../../app/api", () => ({ api: mocks.api, subscribeTurnEvents: mocks.turn, subscribeSessionEvents: mocks.session }));
import { clearSessionCache, useStory } from "../../stores/storyStore";
import { useUi } from "../../stores/uiStore";
import { useSettings } from "../../stores/settingsStore";

beforeEach(async () => {
  await useStory.getState().startNewSessionForCurrentChar();
  vi.resetAllMocks();
  clearSessionCache();
  localStorage.clear();
  useStory.setState(useStory.getInitialState(), true);
  useUi.setState({ ...useUi.getInitialState(), notifyQuiet: vi.fn() }, true);
  useSettings.setState({ autoContinue: false, optionMode: "fill-edit" });
  mocks.turn.mockImplementation(() => vi.fn());
  mocks.session.mockImplementation(() => vi.fn());
  mocks.api.getSession.mockImplementation((id: string) => Promise.resolve(recoveryView(id)));
  mocks.api.listMemories.mockResolvedValue({ memories: [] });
  mocks.api.acceptTurn.mockResolvedValue({ turnId: "t1", status: "preparing" });
  await useStory.getState().openSession("A");
});
afterEach(async () => {
  cleanup();
  await useStory.getState().startNewSessionForCurrentChar();
});

// 早退路径过去是 `return`：输入台已经清空，文字却既没上屏也没进回合，
// 玩家看到的是"我打的字凭空消失"。现在必须把文字还回来并说明原因。
describe("composer input retention", () => {
  it("returns the text and explains when the story and character disagree", async () => {
    useStory.setState({ activeCharacterId: "card_someone_else" });
    render(<><NarrativeStream /><ComposerShelf /></>);
    const box = screen.getByRole("textbox") as HTMLTextAreaElement;
    fireEvent.change(box, { target: { value: "我要说话" } });
    await act(async () => { fireEvent.click(screen.getByRole("button", { name: "推进剧情" })); });
    expect(box.value).toBe("我要说话");
    expect(mocks.api.acceptTurn).not.toHaveBeenCalled();
    expect(screen.getByText(/当前会话与角色不匹配/)).toBeTruthy();
  });
});
