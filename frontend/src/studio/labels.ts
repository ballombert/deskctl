import type { Desktop, Display, Item } from "../api";
import { allDesktopsId } from "../api";

export const itemStatus: Record<string, string> = {
  pending: "À déplacer",
  unchanged: "En place",
  unmatched: "Libre",
  blocked: "Bloquée",
  conflict: "Conflit",
};
export const layoutStatus: Record<string, string> = {
  pending: "Disposition prête",
  unchanged: "Déjà disposée",
  empty: "Aucune fenêtre",
  conflict: "Conflit",
};
export const resultStatus: Record<string, string> = {
  success: "Réussi",
  ignored: "Ignoré",
  uncertain: "Incertain",
  failed: "Échec",
};
export const blockHint: Record<string, string> = {
  identity: "Processus illisible (droits administrateur ?)",
  pin: "Épinglage illisible",
  "no-desktop": "Hors bureau virtuel",
  immovable: "Fenêtre système ou modale",
};

export function desktopName(desktops: Desktop[], id: string | undefined): string {
  if (!id) return "—";
  if (id === allDesktopsId) return "Tous les bureaux";
  return desktops.find(desktop => desktop.id === id)?.name ?? "Bureau absent";
}

export const displayName = (displays: Display[], id: string | undefined) =>
  !id ? "Écran actuel" : displays.find(display => display.id === id)?.name ?? "Écran absent";

/** Where the window will live once the plan is applied. */
export const destination = (item: Item) => item.targetId || (item.window.pinned ? allDesktopsId : item.window.desktopId);

export const initials = (application: string) =>
  application.replace(/\.exe$/i, "").split(/[\s._-]+/).filter(Boolean).slice(0, 2).map(part => part[0]!.toUpperCase()).join("") || "?";

/** Stable hue per application so the same app is recognisable everywhere. */
export function hue(application: string): number {
  let hash = 0;
  for (const char of application.toLowerCase()) hash = (hash * 31 + char.charCodeAt(0)) % 360;
  return hash;
}
