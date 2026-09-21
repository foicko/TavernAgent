// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { IllustrationModal } from "../IllustrationModal";
import { TurnIllustration } from "../TurnIllustration";
import { useImageSettings } from "../../stores/imageSettingsStore";
import * as api from "../../app/api";

vi.mock("../../app/api", () => ({
  generateImage: vi.fn(),
}));

beforeEach(() => {
  localStorage.clear();
  vi.resetAllMocks();
  useImageSettings.setState(useImageSettings.getInitialState(), true);
});

afterEach(cleanup);

describe("IllustrationModal Component", () => {
  it("does not render when open is false", () => {
    const { container } = render(
      <IllustrationModal
        open={false}
        onClose={() => {}}
        turnId="turn-1"
        fallbackText="夜色中的酒馆"
      />
    );
    expect(container.firstChild).toBeNull();
  });

  it("renders when open is true with extracted prompt and allows editing", async () => {
    render(
      <IllustrationModal
        open={true}
        onClose={() => {}}
        turnId="turn-1"
        characterName="爱丽丝"
        fallbackText="爱丽丝微笑着推开酒馆大门"
      />
    );

    expect(screen.getByRole("heading", { name: /生成本幕场景插画/ })).toBeTruthy();
    const textarea = screen.getByRole("textbox") as HTMLTextAreaElement;
    expect(textarea.value).toContain("爱丽丝");
    expect(textarea.value).toContain("酒馆");

    fireEvent.change(textarea, { target: { value: "自定义魔法森林风景" } });
    expect(textarea.value).toBe("自定义魔法森林风景");
  });

  it("calls generateImage and attaches illustration on successful generation", async () => {
    const mockGenerate = vi.mocked(api.generateImage).mockResolvedValueOnce({
      id: "img-test-123",
      url: "/api/v1/images/img-test-123.png",
      prompt: "自定义魔法森林风景",
      mimeType: "image/png",
      createdAt: "2026-09-21T11:00:00Z",
    });

    render(
      <IllustrationModal
        open={true}
        onClose={() => {}}
        turnId="turn-100"
        fallbackText="森林中的城堡"
      />
    );

    const btn = screen.getByRole("button", { name: /开始绘制插画/ });
    fireEvent.click(btn);

    await waitFor(() => {
      expect(mockGenerate).toHaveBeenCalledTimes(1);
    });

    expect(mockGenerate).toHaveBeenCalledWith(
      expect.objectContaining({
        prompt: expect.stringContaining("城堡"),
        engine: "comfyui",
        size: "1024x576",
        style: "anime",
      })
    );

    // Verify success banner/preview
    await waitFor(() => {
      expect(screen.getByText(/插画已绘制成功并自动关联至本回合/)).toBeTruthy();
    });

    // Check store updated
    const saved = useImageSettings.getState().turnIllustrations["turn-100"];
    expect(saved).toBeDefined();
    expect(saved.url).toBe("/api/v1/images/img-test-123.png");
  });

  it("displays error banner when generation fails", async () => {
    vi.mocked(api.generateImage).mockRejectedValueOnce(new Error("API quota exceeded"));

    render(
      <IllustrationModal
        open={true}
        onClose={() => {}}
        turnId="turn-200"
        fallbackText="绝望的战场"
      />
    );

    const btn = screen.getByRole("button", { name: /开始绘制插画/ });
    fireEvent.click(btn);

    await waitFor(() => {
      expect(screen.getByText("API quota exceeded")).toBeTruthy();
    });
  });
});

describe("TurnIllustration Component", () => {
  it("renders nothing if turn has no illustration", () => {
    const { container } = render(<TurnIllustration turnId="non-existent" />);
    expect(container.firstChild).toBeNull();
  });

  it("renders illustration and handles lightbox and re-generate actions", async () => {
    useImageSettings.getState().attachIllustration("turn-abc", {
      id: "img-abc",
      url: "/api/v1/images/img-abc.png",
      prompt: "酒馆壁画",
      mimeType: "image/png",
      createdAt: "2026-09-21T11:00:00Z",
    });

    const onOpenModal = vi.fn();
    render(<TurnIllustration turnId="turn-abc" onOpenModal={onOpenModal} />);

    expect(screen.getByText(/场景插画 · 已保存/)).toBeTruthy();
    const thumbnail = screen.getByRole("img", { name: "酒馆壁画" });
    expect(thumbnail).toBeTruthy();

    // Click thumbnail opens lightbox
    fireEvent.click(thumbnail);
    expect(document.querySelector(".lightbox-img")).toBeTruthy();

    // Press Escape to close lightbox
    fireEvent.keyDown(window, { key: "Escape" });
    expect(document.querySelector(".lightbox-img")).toBeNull();

    // Click re-generate button calls onOpenModal
    const redrawBtn = screen.getByRole("button", { name: /重绘/ });
    fireEvent.click(redrawBtn);
    expect(onOpenModal).toHaveBeenCalledTimes(1);
  });
});
