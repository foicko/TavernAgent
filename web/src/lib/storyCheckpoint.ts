export interface StoryCheckpoint {
  narrativeArc: string;
  mindsets: { attr: string; content: string }[];
  hiddenTension: string;
  openLoops: string;
  milestones: string;
}

// Use the browser's XML parser so CDATA, quoted attributes and escaped text
// have the same meaning as the backend parser. Render only text, never markup.
export function parseStoryCheckpoint(text: string): StoryCheckpoint {
  const raw = text.trim();
  const fallback: StoryCheckpoint = { narrativeArc: raw, mindsets: [], hiddenTension: "", openLoops: "", milestones: "" };
  if (!raw.startsWith("<") || typeof DOMParser === "undefined") return fallback;
  const document = new DOMParser().parseFromString(raw, "application/xml");
  const root = document.documentElement;
  if (root.tagName !== "story_checkpoint" || document.querySelector("parsererror")) return fallback;
  const child = (element: Element | undefined, name: string) => Array.from(element?.children ?? []).find(node => node.tagName === name);
  const content = (element: Element | undefined) => element?.textContent?.trim() ?? "";
  const dynamics = child(root, "character_dynamics");
  return {
    narrativeArc: content(child(root, "narrative_arc")),
    mindsets: Array.from(dynamics?.children ?? []).filter(node => node.tagName === "mindset")
      .map(node => ({ attr: node.getAttribute("character") ?? "", content: content(node) })),
    hiddenTension: content(child(dynamics, "hidden_tension")),
    openLoops: content(child(root, "open_loops")),
    milestones: content(child(root, "milestones")),
  };
}
