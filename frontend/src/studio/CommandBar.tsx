import type { State } from "../api";

interface Props {
  state: State;
  busy: boolean;
  applyLabel: string;
  canApply: boolean;
  onRefresh(): void;
  onUndo(): void;
  onRestoreGeometry(): void;
  onImport(): void;
  onExport(): void;
  onApplyAll(): void;
  onTiling(engine: string, gap: number): void;
}

export function CommandBar({ state, busy, applyLabel, canApply, ...actions }: Props) {
  const tiling = state.config.tiling ?? { engine: "auto", gap: 8 };
  const fancy = state.fancyRunning;
  return (
    <header className="command-bar">
      <div className="brand">
        <span className="brand-mark" aria-hidden>◫</span>
        <div>
          <h1>deskctl Studio</h1>
          <p className="muted">{state.plan.mode === "undo" ? "Mode restauration du dernier lot" : state.directory}</p>
        </div>
      </div>
      <div className="engine" role="group" aria-label="Moteur de disposition">
        <span className={`pulse ${fancy ? "on" : "off"}`} title={fancy ? "FancyWM est lancé" : "FancyWM n’est pas lancé : disposition native"}>
          {fancy ? "FancyWM actif" : "FancyWM absent"}
        </span>
        <label>
          Moteur
          <select aria-label="Moteur" value={tiling.engine} disabled={busy} onChange={event => actions.onTiling(event.target.value, tiling.gap)}>
            <option value="auto">Auto (FancyWM puis natif)</option>
            <option value="fancywm">FancyWM uniquement</option>
            <option value="native">Natif uniquement</option>
          </select>
        </label>
        <label>
          Marge
          <input aria-label="Marge entre fenêtres" type="number" min={0} max={64} value={tiling.gap} disabled={busy}
            onChange={event => actions.onTiling(tiling.engine, Number(event.target.value))} />
        </label>
      </div>
      <nav className="toolbar" aria-label="Actions">
        <button disabled={busy} onClick={actions.onRefresh}>⟳ Actualiser</button>
        <button disabled={busy} onClick={actions.onUndo}>↶ Annuler le dernier lot</button>
        <button disabled={busy} onClick={actions.onRestoreGeometry}>⤺ Restaurer les positions</button>
        <button disabled={busy} onClick={actions.onImport}>Importer</button>
        <button disabled={busy} onClick={actions.onExport}>Exporter</button>
        <button className="primary" disabled={busy || !canApply} onClick={actions.onApplyAll}>{applyLabel}</button>
      </nav>
    </header>
  );
}
