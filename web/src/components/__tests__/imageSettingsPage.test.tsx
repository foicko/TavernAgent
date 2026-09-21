// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, waitFor, cleanup } from "@testing-library/react";
import { ImageSettingsPage } from "../ImageSettingsPage";
import { useImageSettings } from "../../stores/imageSettingsStore";
import * as api from "../../app/api";

describe("ImageSettingsPage", () => {
  afterEach(cleanup);
  beforeEach(() => {
    useImageSettings.getState().resetToDefaults();
    vi.restoreAllMocks();
  });

  it("renders default settings and applies presets", () => {
    render(<ImageSettingsPage />);

    expect(screen.getByText("生图引擎配置")).toBeTruthy();
    expect(screen.getByText("画面与艺术偏好")).toBeTruthy();

    const comfyBtn = screen.getByText("本地 ComfyUI (Qwen 2.1 / 推荐)");
    expect(comfyBtn).toBeTruthy();

    // Click OpenAI preset
    fireEvent.click(screen.getByText("OpenAI DALL-E 3"));
    expect(useImageSettings.getState().customModel).toBe("dall-e-3");

    // Click SD WebUI preset
    fireEvent.click(screen.getByText("本地 SD WebUI (Forge)"));
    expect(useImageSettings.getState().engine).toBe("sd-webui");
  });

  it("handles test generation successfully", async () => {
    const mockImage: api.GeneratedImageResult = {
      id: "img_test_abc",
      url: "/api/v1/images/img_test_abc.png",
      prompt: "magic tavern",
      mimeType: "image/png",
      createdAt: "2026-09-21T12:00:00Z",
    };
    vi.spyOn(api, "generateImage").mockResolvedValue(mockImage);

    render(<ImageSettingsPage />);

    const testBtn = screen.getByText("🎨 试运行生图");
    fireEvent.click(testBtn);

    await waitFor(() => {
      expect(screen.getByAltText("生图预览")).toBeTruthy();
      expect(screen.getByText(/图像生成成功并已本地落盘/)).toBeTruthy();
    });
  });
});
