import type { SessionView } from "../../app/types";

export function recoveryView(id = "A", complete = false): SessionView {
  const root = { nodeId: `root_${id}`, sessionId: id, parentId: "", kind: "root", depth: 0, turnNumber: 0,
    contentJson: JSON.stringify({ openingText: "你来到码头。" }), createdAt: "" };
  const turn = { nodeId: `turn_${id}`, sessionId: id, parentId: root.nodeId, kind: "turn", depth: 1, turnNumber: 1,
    contentJson: JSON.stringify({ blocks: [{ kind: "narration", text: "向导收好了地图。" }],
      options: [
        { optionId: "o1", intent: "clever", text: "查看灯塔" },
        { optionId: "o2", intent: "emotional", text: "向导打招呼" },
        { optionId: "o3", intent: "chaotic", text: "记录航线" },
      ] }), createdAt: "" };
  return { sessionId: id, characterId: "card_elena", title: "港口", rootNodeId: root.nodeId,
    branch: { branchId: `branch_${id}`, name: "main", headNodeId: complete ? turn.nodeId : root.nodeId, version: complete ? 1 : 0 },
    headNode: complete ? turn : root, nodes: complete ? [root, turn] : [root],
    state: { characters: { npc_elena: { characterId: "npc_elena", name: "向导", participant: true } }, items: {}, relationships: {}, promises: {}, moods: {} },
  };
}

export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  return { promise, resolve, reject };
}
