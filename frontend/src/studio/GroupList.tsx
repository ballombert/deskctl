import { useState } from "react";
import { allDesktopsId, type Group, type GroupState, type State, type Zone } from "../api";
import { fullZone, ZoneFields, zoneError } from "./ZoneFields";

export function groupZone(group: Group, planned: GroupState | undefined, state: State): Zone {
  const display = state.displays.find(display => display.id === planned?.displayId);
  if (!display || !planned?.bounds?.width) return group.zone ?? fullZone;
  const { bounds } = planned;
  return { x: (bounds.x - display.work.x) / display.work.width, y: (bounds.y - display.work.y) / display.work.height,
    width: bounds.width / display.work.width, height: bounds.height / display.work.height };
}
interface Props {
  state: State; busy: boolean; stale?: boolean; runtimeAvailable: boolean;
  onCreate(): void; onEdit(group: Group): void; onDelete(group: Group): void;
  onFollowing(group: Group, enabled: boolean): void;
  onAssign(groupId: string, desktopId: string, displayId: string, zone: Zone): void;
  onDirty(): void;
}
export function GroupList({ state, busy, stale, runtimeAvailable, ...actions }: Props) {
  return <section className="group-manager" aria-label="Groupes" role="region">
    <header><h2>Groupes <span className="count">{(state.rules.groups ?? []).length}</span></h2>
      <button disabled={busy} onClick={actions.onCreate}>✚ Nouveau groupe</button></header>
    {!runtimeAvailable && <p className="muted small">Suivi indisponible : liaison d’événements Windows absente.</p>}
    {state.fancyRunning && <p className="warn-text small">Suivi indisponible pendant que FancyWM est actif : arrêtez FancyWM avant d’activer le suivi. Les dispositions manuelles restent disponibles.</p>}
    {state.trackingError && <p role="alert" className="warn-text">{state.trackingError}</p>}
    {(state.rules.groups ?? []).map(group => <GroupCard key={`${group.id}:${state.plan.id}`} group={group} state={state} busy={busy} stale={stale} runtimeAvailable={runtimeAvailable} {...actions} />)}
  </section>;
}
function GroupCard({ group, state, busy, stale, runtimeAvailable, onEdit, onDelete, onFollowing, onAssign, onDirty }: Omit<Props, "onCreate"> & { group: Group }) {
  const planned = state.plan.groups?.find(candidate => candidate.id === group.id);
  const [desktop, setDesktop] = useState(planned?.desktopId || state.plan.currentDesktopId || state.plan.desktops[0]?.id || "");
  const [display, setDisplay] = useState(planned?.displayId || state.displays.find(candidate => candidate.primary)?.id || state.displays[0]?.id || "");
  const [zone, setZone] = useState(groupZone(group, planned, state));
  const references = state.rules.rules.filter(rule => rule.groupId === group.id);
  return <article className="group-card">
    <h3><button draggable={!busy && !stale} aria-label={`Déplacer le groupe ${group.name}`} onDragStart={event => {
      event.dataTransfer.setData("application/x-deskctl-group", group.id); event.dataTransfer.effectAllowed = "move";
    }}>⠿ {group.name}</button></h3>
    <p className={planned?.status === "conflict" ? "warn-text" : "muted"}>{planned?.members?.length ?? 0} membre(s) · {planned?.status ?? "empty"} · {planned?.explanation}</p>
    <p className="small">Règles : {references.map(rule => rule.name).join(", ") || "aucune"}</p>
    <label><input type="checkbox" aria-label={`Suivre ${group.name}`} disabled={busy || !runtimeAvailable || !!state.fancyRunning}
      checked={(state.following ?? []).includes(group.id)} onChange={event => onFollowing(group, event.target.checked)} /> Suivre les déplacements Windows (session)</label>
    <div className="row"><button disabled={busy} onClick={() => onEdit(group)}>Modifier le groupe {group.name}</button>
      <button disabled={busy || stale} onClick={() => onDelete(group)}>Supprimer le groupe {group.name}</button></div>
    <details><summary>Préparer la destination de {group.name}</summary>
      <label>Bureau<select aria-label={`Bureau du groupe ${group.name}`} value={desktop} onChange={event => { setDesktop(event.target.value); onDirty(); }}>
        {state.plan.desktops.map(candidate => <option key={candidate.id} value={candidate.id}>{candidate.name}</option>)}
        <option value={allDesktopsId}>Tous les bureaux</option>
      </select></label>
      <label>Écran<select aria-label={`Écran du groupe ${group.name}`} value={display} onChange={event => { setDisplay(event.target.value); onDirty(); }}>
        {state.displays.map(candidate => <option key={candidate.id} value={candidate.id}>{candidate.name}</option>)}
      </select></label>
      <ZoneFields label={`Zone de ${group.name}`} zone={zone} onChange={next => { setZone(next ?? fullZone); onDirty(); }} />
      <button disabled={busy || stale || !desktop || !display || !!zoneError(zone)} onClick={() => onAssign(group.id, desktop, display, zone)}>Préparer le groupe {group.name}</button>
      <p className="muted small">Prépare le lot uniquement. Les règles ne sont pas modifiées.</p>
    </details>
  </article>;
}
