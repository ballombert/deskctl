import type { Zone } from "../api";

export const fullZone: Zone = { x: 0, y: 0, width: 1, height: 1 };
export function zoneError(zone?: Zone): string {
  if (!zone) return "";
  return Object.values(zone).every(Number.isFinite) && zone.x >= 0 && zone.y >= 0 &&
    zone.width > 0 && zone.height > 0 && zone.x + zone.width <= 1.000001 && zone.y + zone.height <= 1.000001
    ? "" : "La zone doit rester dans l’écran (0 à 100 %, dimensions positives).";
}
export function clampZone(zone: Zone): Zone {
  const width = Math.min(1, Math.max(.01, zone.width));
  const height = Math.min(1, Math.max(.01, zone.height));
  return { width, height, x: Math.min(1 - width, Math.max(0, zone.x)), y: Math.min(1 - height, Math.max(0, zone.y)) };
}
export function ZoneFields({ zone, onChange, label = "Zone", optional = false }: {
  zone?: Zone; onChange(zone: Zone | undefined): void; label?: string; optional?: boolean;
}) {
  const value = zone ?? fullZone;
  return <fieldset className="zone-fields">
    <legend>{label} · % de la zone de travail</legend>
    {optional && <label><input type="checkbox" checked={!!zone} onChange={event => onChange(event.target.checked ? { ...fullZone } : undefined)} /> Personnaliser la zone</label>}
    {(!optional || zone) && <div className="editor-grid">{(["x", "y", "width", "height"] as const).map((key, index) =>
      <label key={key}>{["Gauche", "Haut", "Largeur", "Hauteur"][index]}
        <input aria-label={`${label} ${key} (%)`} type="number" min={0} max={100} step={1} value={Math.round(value[key] * 10000) / 100}
          onChange={event => onChange({ ...value, [key]: Number(event.target.value) / 100 })} />
      </label>)}</div>}
    {zoneError(zone) && <p className="warn-text" role="status">{zoneError(zone)}</p>}
  </fieldset>;
}
