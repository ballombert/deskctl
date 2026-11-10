import { useMemo, useState } from "react";
import { allDesktopsId, windowTitle, type Item, type State } from "../api";
import { blockHint, desktopName, destination, hue, initials, itemStatus } from "./labels";

interface Props {
  state: State;
  busy: boolean;
  selected: string[];
  highlighted: Set<string>;
  onSelect(id: string, additive: boolean): void;
  onAssign(item: Item, desktopId: string): void;
  onMoveToDisplay(item: Item, displayId: string): void;
  onRuleFromWindow(item: Item): void;
  onCapture(axis: "h" | "v"): void;
}

type Filter = "all" | "pending" | "blocked" | "free";
const filters: [Filter, string][] = [["all", "Toutes"], ["pending", "À déplacer"], ["free", "Libres"], ["blocked", "Bloquées"]];

export function AppDock({ state, busy, selected, highlighted, ...actions }: Props) {
  const [query, setQuery] = useState("");
  const [filter, setFilter] = useState<Filter>("all");
  const items = useMemo(() => {
    const needle = query.trim().toLowerCase();
    return state.plan.items.filter(item => {
      if (needle && !`${item.window.application} ${item.window.title}`.toLowerCase().includes(needle)) return false;
      if (filter === "pending") return item.status === "pending";
      if (filter === "blocked") return item.status === "blocked" || item.status === "conflict";
      if (filter === "free") return item.status === "unmatched";
      return true;
    });
  }, [state.plan.items, query, filter]);
  const focused = state.plan.items.find(item => item.window.id === selected[selected.length - 1]);

  return (
    <aside className="dock" aria-label="Fenêtres" role="region">
      <header>
        <h2>Fenêtres <span className="count">{state.plan.items.length}</span></h2>
        <input type="search" placeholder="Rechercher…" aria-label="Rechercher une fenêtre" value={query} onChange={event => setQuery(event.target.value)} />
        <div className="chips" role="group" aria-label="Filtrer">
          {filters.map(([value, label]) => (
            <button key={value} className={filter === value ? "chip active" : "chip"} aria-pressed={filter === value} onClick={() => setFilter(value)}>{label}</button>
          ))}
        </div>
      </header>
      <ul className="window-list">
        {items.map(item => {
          const isSelected = selected.includes(item.window.id);
          return (
            <li key={item.window.id}>
              <button
                className={`window-row ${isSelected ? "selected" : ""} ${highlighted.has(item.window.id) ? "highlight" : ""} status-${item.status}`}
                draggable={item.status !== "blocked"}
                aria-pressed={isSelected}
                onClick={event => actions.onSelect(item.window.id, event.ctrlKey || event.metaKey || event.shiftKey)}
                onDragStart={event => {
                  event.dataTransfer.setData("text/plain", item.window.id);
                  event.dataTransfer.effectAllowed = "move";
                }}
              >
                <span className="app-icon" style={{ "--hue": hue(item.window.application) } as React.CSSProperties} aria-hidden>{initials(item.window.application)}</span>
                <span className="window-text">
                  <strong>{windowTitle(item)}</strong>
                  <small>{item.window.application} → {desktopName(state.plan.desktops, destination(item))}</small>
                </span>
                <span className={`badge ${item.status}`}>{itemStatus[item.status] ?? item.status}</span>
                {item.window.elevated && <span className="badge warn" title="Application administrateur">Admin</span>}
              </button>
            </li>
          );
        })}
        {items.length === 0 && <li className="muted empty">Aucune fenêtre.</li>}
      </ul>

      {selected.length > 1 && (
        <section className="inspector" aria-label="Sélection multiple">
          <h3>{selected.length} fenêtres sélectionnées</h3>
          <p className="muted">Créer un groupe réutilisable à partir de leur disposition actuelle.</p>
          <div className="row">
            <button disabled={busy} onClick={() => actions.onCapture("h")}>▥ Grouper côte à côte</button>
            <button disabled={busy} onClick={() => actions.onCapture("v")}>▤ Grouper empilées</button>
          </div>
        </section>
      )}

      {selected.length === 1 && focused && (
        <section className="inspector" aria-label="Fenêtre sélectionnée">
          <h3>{windowTitle(focused)}</h3>
          <p className="muted">{focused.explanation}</p>
          {focused.window.blockCode && <p className="warn-text">{blockHint[focused.window.blockCode] ?? focused.window.blockCode}</p>}
          <label>
            Destination
            <select
              aria-label={`Destination pour ${windowTitle(focused)}`}
              disabled={busy || focused.status === "blocked"}
              value={focused.targetId}
              onChange={event => actions.onAssign(focused, event.target.value)}
            >
              <option value="">Laisser sur son bureau</option>
              {state.plan.desktops.map(desktop => <option key={desktop.id} value={desktop.id}>{desktop.name}</option>)}
              <option value={allDesktopsId}>Tous les bureaux</option>
            </select>
          </label>
          <div className="row wrap">
            {state.displays.map(display => (
              <button key={display.id} disabled={busy || display.id === focused.window.displayId}
                title={display.fancyWMIndex ? `FancyWM écran ${display.fancyWMIndex}` : "Placement natif"}
                onClick={() => actions.onMoveToDisplay(focused, display.id)}>
                ⇢ Envoyer sur {display.name}
              </button>
            ))}
          </div>
          <button disabled={busy} onClick={() => actions.onRuleFromWindow(focused)}>✚ En faire une règle</button>
        </section>
      )}
    </aside>
  );
}
