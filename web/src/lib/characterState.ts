import type { WorldState } from "../app/types";

export function primaryCharacterId(state: WorldState | null, presetKey: string | null): string | undefined {
  if (!state) return undefined;
  for (const id of [`npc_${presetKey}`, presetKey ?? ""]) {
    if (id !== "player" && state.characters[id]) return id;
  }
  const characters = Object.values(state.characters).filter(character => character.characterId !== "player");
  return (characters.find(character => character.participant) ?? characters[0])?.characterId;
}

export function affectionPercent(value: number): number {
  return Math.max(0, Math.min(100, (value + 100) / 2));
}
