import { useEffect, useRef, useState, type CSSProperties, type DragEvent, type PointerEvent } from "react";
import { allDesktopsId, windowTitle, type GroupState, type Item, type LayoutPlan, type Rect, type State, type Zone } from "../api";
import { destination, hue, initials } from "./labels";
import { clampZone } from "./ZoneFields";
import { groupZone } from "./GroupList";

interface Props {
  state: State;
  busy: boolean;
  selected: string[];
  ghost: LayoutPlan | null;
  onSelect(id: string, additive: boolean): void;
  onDropWindow(windowId: string, desktopId: string): void;
  onDisplayIndex(displayId: string, index: number): void;
  onOpenDisplaySettings(): void;
  onAssignGroup(groupId: string, desktopId: string, displayId: string, zone: Zone): void;
  onConflict(message: string): void;
}

function bounds(state: State): Rect {
  if (!state.displays.length) return { x: 0, y: 0, width: 1920, height: 1080 };
  const left = Math.min(...state.displays.map(display => display.x));
  const top = Math.min(...state.displays.map(display => display.y));
  const right = Math.max(...state.displays.map(display => display.x + display.width));
  const bottom = Math.max(...state.displays.map(display => display.y + display.height));
  return { x: left, y: top, width: right - left, height: bottom - top };
}

/** Converts screen pixels to percentages of the virtual screen so mock-ups scale with the column. */
const place = (rect: Rect, area: Rect): CSSProperties => ({
  left: `${((rect.x - area.x) / area.width) * 100}%`,
  top: `${((rect.y - area.y) / area.height) * 100}%`,
  width: `${(rect.width / area.width) * 100}%`,
  height: `${(rect.height / area.height) * 100}%`,
});

export function StudioCanvas({ state, busy, selected, ghost, ...actions }: Props) {
  const [over, setOver] = useState("");
  const area = bounds(state);
  const columns = [...state.plan.desktops, { id: allDesktopsId, name: "Tous les bureaux" }];
  const usedIndices = new Set(state.displays.map(display => display.fancyWMIndex).filter(Boolean));
  const [resizing, setResizing] = useState<{ id: string; bounds: Rect } | null>(null);
  const cancelResize = useRef<(() => void) | null>(null);
  useEffect(() => () => cancelResize.current?.(), []);
  useEffect(() => { if (busy) cancelResize.current?.(); }, [busy]);

  const resolveGroup = (event: DragEvent) => {
    const explicit = event.dataTransfer.getData("application/x-deskctl-group");
    const windowId = event.dataTransfer.getData("text/plain");
    const matches = (state.plan.groups ?? []).filter(group => group.members?.includes(windowId));
    if (matches.length > 1 || matches[0]?.status === "conflict") {
      actions.onConflict("Conflit de groupes : appartenance ambiguë. Aucun déplacement préparé.");
      return false;
    }
    return explicit || matches[0]?.id || "";
  };
  const dropOnMonitor = (event: DragEvent, desktopId: string, displayId: string, stage: HTMLElement) => {
    event.preventDefault(); event.stopPropagation(); setOver("");
    if (busy) return;
    const groupId = resolveGroup(event);
    if (groupId === false) return;
    if (!groupId) {
      const id = event.dataTransfer.getData("text/plain");
      if (id) actions.onDropWindow(id, desktopId);
      return;
    }
    const display = state.displays.find(display => display.id === displayId)!;
    const group = state.rules.groups?.find(group => group.id === groupId);
    if (!group) return actions.onConflict("Groupe introuvable.");
    const box = stage.getBoundingClientRect();
    if (!box.width || !box.height || !Number.isFinite(event.clientX) || !Number.isFinite(event.clientY))
      return actions.onConflict("Géométrie de l’écran indisponible : utilisez les champs de destination du groupe.");
    const pixelX = area.x + (event.clientX - box.left) / box.width * area.width;
    const pixelY = area.y + (event.clientY - box.top) / box.height * area.height;
    const zone = groupZone(group, state.plan.groups?.find(candidate => candidate.id === groupId), state);
    actions.onAssignGroup(groupId, desktopId, displayId, clampZone({
      ...zone, x: (pixelX - display.work.x) / display.work.width, y: (pixelY - display.work.y) / display.work.height,
    }));
  };
  const resize = (event: PointerEvent<HTMLButtonElement>, group: GroupState) => {
    event.preventDefault(); event.stopPropagation();
    if (busy || group.status === "conflict") return;
    cancelResize.current?.();
    const stage = event.currentTarget.closest(".stage") as HTMLElement;
    const box = stage.getBoundingClientRect();
    if (!box.width || !box.height) return;
    const display = state.displays.find(display => display.id === group.displayId);
    if (!display) return;
    const start = { x: event.clientX, y: event.clientY };
    const original = group.bounds;
    let next = original;
    const move = (pointer: globalThis.PointerEvent) => {
      next = { ...original,
        width: Math.max(display.work.width * .01, Math.min(display.work.x + display.work.width - original.x, original.width + (pointer.clientX - start.x) / box.width * area.width)),
        height: Math.max(display.work.height * .01, Math.min(display.work.y + display.work.height - original.y, original.height + (pointer.clientY - start.y) / box.height * area.height)),
      };
      setResizing({ id: group.id, bounds: next });
    };
    const stop = () => {
      window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", stop); window.removeEventListener("pointercancel", cancel);
      setResizing(null);
      cancelResize.current = null;
      actions.onAssignGroup(group.id, group.desktopId, group.displayId, clampZone({
        x: (next.x - display.work.x) / display.work.width, y: (next.y - display.work.y) / display.work.height,
        width: next.width / display.work.width, height: next.height / display.work.height,
      }));
    };
    const cancel = () => { window.removeEventListener("pointermove", move); window.removeEventListener("pointerup", stop); window.removeEventListener("pointercancel", cancel); setResizing(null); cancelResize.current = null; };
    cancelResize.current = cancel;
    window.addEventListener("pointermove", move); window.addEventListener("pointerup", stop); window.addEventListener("pointercancel", cancel);
  };

  const drop = (desktopId: string) => ({
    onDragOver: (event: DragEvent) => {
      event.preventDefault();
      event.dataTransfer.dropEffect = "move";
      setOver(desktopId);
    },
    onDragLeave: () => setOver(""),
    onDrop: (event: DragEvent) => {
      event.preventDefault();
      event.stopPropagation();
      setOver("");
      const group = resolveGroup(event);
      if (group === false) return;
      if (group) {
        if (!busy) actions.onConflict("Déposez le groupe sur un écran précis ou utilisez sa destination accessible.");
        return;
      }
      const id = event.dataTransfer.getData("text/plain");
      if (id && !busy) actions.onDropWindow(id, desktopId);
    },
  });

  return (
    <section className="canvas" aria-label="Bureaux et écrans">
      <div className="screens-strip" role="group" aria-label="Écrans">
        {state.displays.map((display, index) => (
          <div key={display.id} className="screen-chip">
            <span className="screen-number">{index + 1}</span>
            <span>
              <strong>{display.name}</strong>
              <small>{display.width}×{display.height}{display.primary ? " · principal" : ""}</small>
            </span>
            <label>
              FancyWM
              <select aria-label={`Indice FancyWM pour ${display.name}`} disabled={busy} value={display.fancyWMIndex || 0}
                onChange={event => actions.onDisplayIndex(display.id, Number(event.target.value))}>
                <option value={0}>—</option>
                {state.displays.map((_, position) => position + 1).map(value => (
                  <option key={value} value={value} disabled={value !== display.fancyWMIndex && usedIndices.has(value)}>{value}</option>
                ))}
              </select>
            </label>
          </div>
        ))}
        <button className="ghost-button" onClick={actions.onOpenDisplaySettings}>Paramètres d’affichage Windows</button>
      </div>

      <div className="desktops">
        {columns.map(column => {
          const items = state.plan.items.filter(item => destination(item) === column.id);
          const current = column.id === state.plan.currentDesktopId;
          const showGhost = ghost && (ghost.pinned ? column.id === allDesktopsId : ghost.desktopId === column.id || (!ghost.desktopId && current));
          return (
            <article key={column.id} className={`desktop ${over === column.id ? "drop-over" : ""} ${column.id === allDesktopsId ? "pinned" : ""}`} {...drop(column.id)}>
              <header>
                <h2>{column.name}</h2>
                {current && <span className="badge info">Actuel</span>}
                <span className="count">{items.length}</span>
              </header>
              <div className="stage" style={{ aspectRatio: `${area.width} / ${area.height}` }}
                onDragOver={event => event.preventDefault()}
                onDrop={event => {
                  const box = event.currentTarget.getBoundingClientRect();
                  const x = area.x + (event.clientX - box.left) / box.width * area.width;
                  const y = area.y + (event.clientY - box.top) / box.height * area.height;
                  const display = state.displays.find(display => x >= display.x && x <= display.x + display.width && y >= display.y && y <= display.y + display.height);
                  if (display) dropOnMonitor(event, column.id, display.id, event.currentTarget);
                  else { event.preventDefault(); event.stopPropagation(); actions.onConflict("Choisissez un écran valide pour ce dépôt."); }
                }}>
                {state.displays.map((display, index) => (
                  <div key={display.id} className="monitor" style={place(display, area)} role="group" aria-label={`${display.name} dans ${column.name}`}
                    onDragOver={event => { event.preventDefault(); event.stopPropagation(); }}
                    onDrop={event => dropOnMonitor(event, column.id, display.id, event.currentTarget.parentElement!)}>
                    <span>{index + 1}</span>
                    <div className="work" style={place(display.work, display)} />
                  </div>
                ))}
                {items.filter(item => item.window.bounds).map(item => (
                  <Tile key={item.window.id} item={item} area={area} selected={selected.includes(item.window.id)} onSelect={actions.onSelect} />
                ))}
                {(state.plan.groups ?? []).filter(group => group.desktopId === column.id && group.bounds?.width > 0).map(group => (
                  <div key={group.id} className={`group-frame ${group.status}`} style={place(resizing?.id === group.id ? resizing.bounds : group.bounds, area)}
                    aria-label={`Cadre du groupe ${group.name}`}>
                    <button className="group-handle" draggable={!busy && group.status !== "conflict"} disabled={busy || group.status === "conflict"}
                      aria-label={`Déplacer le cadre ${group.name}`} title={group.explanation}
                      onDragStart={event => { event.stopPropagation(); event.dataTransfer.setData("application/x-deskctl-group", group.id); event.dataTransfer.effectAllowed = "move"; }}>
                      ⠿ {group.name} · {group.members?.length ?? 0}
                    </button>
                    <button className="group-resize" aria-label={`Redimensionner le groupe ${group.name}`} disabled={busy || group.status === "conflict"}
                      onPointerDown={event => resize(event, group)} title="Glisser pour préparer la taille ; champs numériques dans Groupes">◢</button>
                  </div>
                ))}
                {showGhost && ghost.slots.map(slot => (
                  <div key={slot.windowId} className="ghost-slot" style={place(slot.rect, area)}>
                    <span>{slot.application.replace(/\.exe$/i, "")}</span>
                  </div>
                ))}
              </div>
              <ul className="loose" aria-label={`Fenêtres de ${column.name}`}>
                {items.map(item => (
                  <li key={item.window.id}>
                    <button className={`pill ${item.status} ${selected.includes(item.window.id) ? "selected" : ""}`} draggable={item.status !== "blocked"}
                      onDragStart={event => event.dataTransfer.setData("text/plain", item.window.id)}
                      onClick={event => actions.onSelect(item.window.id, event.ctrlKey || event.metaKey || event.shiftKey)}>
                      {item.status === "pending" && <span aria-hidden>➜ </span>}
                      {windowTitle(item)}
                    </button>
                  </li>
                ))}
              </ul>
            </article>
          );
        })}
      </div>
    </section>
  );
}

function Tile({ item, area, selected, onSelect }: { item: Item; area: Rect; selected: boolean; onSelect(id: string, additive: boolean): void }) {
  return (
    <div
      className={`tile ${item.status} ${selected ? "selected" : ""}`}
      style={{ ...place(item.window.bounds!, area), "--hue": hue(item.window.application) } as CSSProperties}
      title={`${item.window.application} — ${windowTitle(item)}`}
      draggable={item.status !== "blocked"}
      onDragStart={event => {
        event.stopPropagation();
        event.dataTransfer.setData("text/plain", item.window.id);
        event.dataTransfer.effectAllowed = "move";
      }}
      onClick={event => onSelect(item.window.id, event.ctrlKey || event.metaKey || event.shiftKey)}
      aria-hidden
    >
      {initials(item.window.application)}
    </div>
  );
}
