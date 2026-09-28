// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { isModalOpen } from "../modalStack";

describe("isModalOpen", () => {
  afterEach(() => {
    document.body.innerHTML = "";
  });

  it("detects an open modal dialog", () => {
    document.body.innerHTML = '<div role="dialog" aria-modal="true"></div>';
    expect(isModalOpen()).toBe(true);
  });

  it("is false when nothing is open", () => {
    expect(isModalOpen()).toBe(false);
  });

  it("ignores non-modal dialogs (aria-modal absent/false)", () => {
    document.body.innerHTML = '<div role="dialog"></div><div role="dialog" aria-modal="false"></div>';
    expect(isModalOpen()).toBe(false);
  });
});
