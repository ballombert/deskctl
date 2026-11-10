import { useRef, useState, type CSSProperties, type PointerEvent } from "react";
import { allDesktopsId, type Item, type LayoutNode, type Rule, type RulePreview, type State } from "../api";
import { hue, layoutStatus } from "./labels";
import {
  addChild, describe, escapePattern, matchesLeaf, moveChild, newGroup, newLeaf, patternError, removeChild, resizePair, update, type Path,
} from "./layoutTree";
import { ZoneFields, zoneError } from "./ZoneFields";

interface Props {
  state: State;
  draft: Rule;
  isNew: boolean;
  busy: boolean;
  preview: RulePreview | null;
  previewError: string;
  selection: Item[];
  onChange(rule: Rule): void;
  onSave(rule: Rule, apply: boolean): void;
  onCancel(): void;
  groupMode?: boolean;
  applyDisabled?: boolean;
  saveDisabled?: boolean;
  onEditGroup?(id: string): void;
}

const samePath = (left: Path, right: Path) => left.length === right.length && left.every((value, index) => value === right[index]);

export function validate(rule: Rule): string {
  if (!rule.name.trim()) return "Donnez un nom à la règle.";
  if (!rule.desktopId) return "Choisissez un bureau cible.";
  if (zoneError(rule.zone)) return zoneError(rule.zone);
  if (rule.groupId) return "";
  const leaves: LayoutNode[] = [];
  const walk = (node: LayoutNode) => (node.kind === "window" ? leaves.push(node) : node.children?.forEach(walk));
  if (rule.layout) walk(rule.layout);
  else leaves.push({ kind: "window", weight: 0, application: rule.application, title: rule.title });
  if (rule.layout && !leaves.length) return "Le groupe doit contenir au moins une fenêtre.";
  for (const leaf of leaves) {
    if (!leaf.application && !leaf.title) return "Chaque fenêtre exige une application ou un titre.";
    const error = patternError(leaf.application) || patternError(leaf.title);
    if (error) return `${error} : ${leaf.application || leaf.title}`;
  }
  return "";
}

export function LayoutEditor({ state, draft, isNew, busy, preview, previewError, selection, onChange, onSave, onCancel, groupMode = false, onEditGroup, applyDisabled = false, saveDisabled = false }: Props) {
  const [focus, setFocus] = useState<Path>([]);
  const shared = (state.rules.groups ?? []).find(group => group.id === draft.groupId);
  const layout = shared?.layout ?? draft.layout;
  const set = (patch: Partial<Rule>) => onChange({ ...draft, ...patch });
  const setLayout = (next: LayoutNode) => set({ layout: next });
  const target = state.displays.find(display => display.id === draft.displayId) ?? state.displays.find(display => display.primary);
  const error = validate(draft) || (draft.groupId && !shared ? "Groupe introuvable." : "");
  const focused = layout ? safeNode(layout, focus) : undefined;

  const toggleLayout = (enabled: boolean) => {
    if (enabled) {
      onChange({ ...draft, application: "", title: "", layout: newGroup("h", [newLeaf(draft.application, draft.title), newLeaf()]) });
      setFocus([0]);
    } else {
      const first = firstLeaf(layout);
      onChange({ ...draft, application: first?.application ?? "", title: first?.title ?? "", layout: undefined });
      setFocus([]);
    }
  };

  const addSelection = () => {
    if (!layout || !focused || focused.kind !== "group") return;
    let next = layout;
    for (const item of selection) {
      const sameApp = state.plan.items.filter(candidate => candidate.window.application.toLowerCase() === item.window.application.toLowerCase()).length > 1;
      next = addChild(next, focus, newLeaf(escapePattern(item.window.application), sameApp ? escapePattern(item.window.title) : ""));
    }
    setLayout(next);
  };

  return (
    <section className="editor" aria-label={groupMode ? "Éditeur de groupe" : "Éditeur de règle"}>
      <header>
        <h2>{isNew ? (groupMode ? "Nouveau groupe" : "Nouvelle règle") : `Modifier « ${draft.name} »`}</h2>
        <button aria-label="Fermer l’éditeur" onClick={onCancel}>✕</button>
      </header>

      <div className="editor-grid">
        <label>
          Nom
          <input aria-label={groupMode ? "Nom du groupe" : "Nom de la règle"} value={draft.name} onChange={event => set({ name: event.target.value })} />
        </label>
        {!groupMode && <label>
          Bureau
          <select aria-label="Bureau cible" value={draft.desktopId} onChange={event => set({ desktopId: event.target.value })}>
            <option value="">Choisir…</option>
            {state.plan.desktops.map(desktop => <option key={desktop.id} value={desktop.id}>{desktop.name}</option>)}
            <option value={allDesktopsId}>📌 Tous les bureaux (épingle le groupe)</option>
          </select>
        </label>}
        {!groupMode && <label>
          Écran
          <select aria-label="Écran cible" value={draft.displayId ?? ""} onChange={event => set({ displayId: event.target.value || undefined })}>
            <option value="">{layout ? "Écran principal" : "Ne pas changer d’écran"}</option>
            {state.displays.map(display => <option key={display.id} value={display.id}>{display.name} ({display.width}×{display.height})</option>)}
          </select>
        </label>}
        {!groupMode && <label>
          Groupe réutilisable
          <select aria-label="Groupe de la règle" value={draft.groupId ?? ""} onChange={event => set({ groupId: event.target.value || undefined, layout: undefined, application: "", title: "" })}>
            <option value="">Règle simple ou nouvelle disposition</option>
            {(state.rules.groups ?? []).map(group => <option key={group.id} value={group.id}>{group.name}</option>)}
          </select>
        </label>}
        {!groupMode && !draft.groupId && <label className="toggle">
          <input type="checkbox" checked={!!layout} onChange={event => toggleLayout(event.target.checked)} />
          Disposer en tuiles (groupe de fenêtres)
        </label>}
      </div>
      {(groupMode || draft.groupId || layout) && <ZoneFields zone={draft.zone} optional label={groupMode ? "Zone par défaut" : "Zone de remplacement"} onChange={zone => set({ zone })} />}
      {shared && <p>Disposition partagée : {shared.name} <button onClick={() => onEditGroup?.(shared.id)}>Modifier le groupe partagé</button></p>}

      {!layout && (
        <div className="editor-grid">
          <PatternField label="Application (regex)" value={draft.application} onChange={application => set({ application })} />
          <PatternField label="Titre (regex)" value={draft.title} onChange={title => set({ title })} />
        </div>
      )}

      {layout && (
        <>
          <p className="muted small">Cliquez une zone pour la régler. Glissez les séparateurs pour changer les tailles. {describe(layout)}</p>
          <div className="layout-preview" style={{ aspectRatio: target ? `${target.work.width} / ${target.work.height}` : "16 / 9" }}>
            <NodeView node={layout} path={[]} root={layout} focus={focus} items={state.plan.items} onFocus={setFocus} onChange={setLayout} readOnly={!!shared} />
          </div>
          {focused && !shared && (
            <NodeForm
              node={focused}
              path={focus}
              root={layout}
              selectionCount={selection.length}
              onChange={setLayout}
              onFocus={setFocus}
              onAddSelection={addSelection}
            />
          )}
        </>
      )}

      <footer>
        <p className={error || previewError ? "warn-text" : "muted"} role="status">
          {error || previewError || (preview ? `${preview.matches.length} fenêtre(s) correspondent` : "Calcul de l’aperçu…")}
          {!error && !previewError && preview?.layout && <> · {layoutStatus[preview.layout.status] ?? preview.layout.status}</>}
        </p>
        {!error && preview?.layout?.explanation && <p className="muted small">{preview.layout.explanation}</p>}
        <div className="row">
          <button onClick={onCancel}>Annuler</button>
          <button disabled={busy || !!error || saveDisabled} onClick={() => onSave(draft, false)}>{groupMode ? "Enregistrer ce groupe" : "Enregistrer cette règle"}</button>
          {!groupMode && <button className="primary" disabled={busy || !!error || applyDisabled || saveDisabled} onClick={() => onSave(draft, true)}>Enregistrer et appliquer</button>}
        </div>
      </footer>
    </section>
  );
}

function PatternField({ label, value, onChange }: { label: string; value: string | undefined; onChange(value: string): void }) {
  const error = patternError(value);
  return (
    <label className={error ? "invalid" : ""}>
      {label}
      <input aria-label={label} value={value ?? ""} spellCheck={false} placeholder="ex. ^Code$" onChange={event => onChange(event.target.value)} />
      {error && <small className="warn-text">{error}</small>}
    </label>
  );
}

function safeNode(root: LayoutNode, path: Path): LayoutNode | undefined {
  let node: LayoutNode | undefined = root;
  for (const index of path) node = node?.children?.[index];
  return node;
}

function firstLeaf(node: LayoutNode | undefined): LayoutNode | undefined {
  if (!node) return undefined;
  if (node.kind === "window") return node;
  for (const child of node.children ?? []) {
    const leaf = firstLeaf(child);
    if (leaf) return leaf;
  }
  return undefined;
}

interface NodeViewProps {
  readOnly?: boolean;
  node: LayoutNode;
  path: Path;
  root: LayoutNode;
  focus: Path;
  items: Item[];
  onFocus(path: Path): void;
  onChange(root: LayoutNode): void;
}

function NodeView({ node, path, root, focus, items, onFocus, onChange, readOnly }: NodeViewProps) {
  const container = useRef<HTMLDivElement>(null);
  const focused = samePath(path, focus);
  if (node.kind === "window") {
    const matches = items.filter(item => matchesLeaf(node, item)).length;
    const label = describe(node);
    return (
      <button type="button" className={`leaf ${focused ? "focused" : ""} ${matches ? "" : "unmatched"}`}
        style={{ "--hue": hue(label) } as CSSProperties} aria-label={`Zone ${label}`}
        onClick={event => { event.stopPropagation(); onFocus(path); }}>
        <strong>{label}</strong>
        <small>{matches} fenêtre(s)</small>
      </button>
    );
  }
  const horizontal = node.axis !== "v";
  const startDrag = (index: number) => (event: PointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.stopPropagation();
    const box = container.current?.getBoundingClientRect();
    const size = horizontal ? box?.width : box?.height;
    if (!size) return;
    const origin = horizontal ? event.clientX : event.clientY;
    const snapshot = root;
    const move = (moveEvent: globalThis.PointerEvent) => {
      const delta = (((horizontal ? moveEvent.clientX : moveEvent.clientY) - origin) / size) * 100;
      onChange(resizePair(snapshot, path, index, Math.round(delta)));
    };
    const stop = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", stop);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", stop);
  };
  const children = node.children ?? [];
  return (
    <div ref={container} className={`group ${horizontal ? "h" : "v"} ${focused ? "focused" : ""}`}
      onClick={event => { event.stopPropagation(); onFocus(path); }}>
      {children.map((child, index) => (
        <div key={index} className="cell" style={{ flexGrow: child.weight, flexBasis: 0 }}>
          <NodeView node={child} path={[...path, index]} root={root} focus={focus} items={items} onFocus={onFocus} onChange={onChange} readOnly={readOnly} />
          <span className="weight">{Math.round(child.weight)}%</span>
          {!readOnly && index < children.length - 1 && (
            <div className="splitter" role="separator" aria-orientation={horizontal ? "vertical" : "horizontal"}
              aria-valuenow={Math.round(child.weight)} tabIndex={0} aria-label="Redimensionner"
              onPointerDown={startDrag(index)}
              onKeyDown={event => {
                const step = event.key === "ArrowRight" || event.key === "ArrowDown" ? 5 : event.key === "ArrowLeft" || event.key === "ArrowUp" ? -5 : 0;
                if (step) {
                  event.preventDefault();
                  onChange(resizePair(root, path, index, step));
                }
              }} />
          )}
        </div>
      ))}
    </div>
  );
}

interface NodeFormProps {
  node: LayoutNode;
  path: Path;
  root: LayoutNode;
  selectionCount: number;
  onChange(root: LayoutNode): void;
  onFocus(path: Path): void;
  onAddSelection(): void;
}

function NodeForm({ node, path, root, selectionCount, onChange, onFocus, onAddSelection }: NodeFormProps) {
  const isRoot = path.length === 0;
  const index = path[path.length - 1];
  const edit = (patch: Partial<LayoutNode>) => onChange(update(root, path, current => ({ ...current, ...patch })));
  const parent = isRoot ? undefined : safeNode(root, path.slice(0, -1));
  const siblings = parent?.children?.length ?? 1;
  const remove = () => {
    if (siblings <= 1) return;
    onChange(removeChild(root, path));
    onFocus(path.slice(0, -1));
  };
  const split = (axis: "h" | "v") => {
    onChange(update(root, path, leaf => ({ ...newGroup(axis, [{ ...leaf }, newLeaf()]), weight: leaf.weight })));
    onFocus([...path, 1]);
  };
  const setWeight = (weight: number) => {
    if (isRoot || siblings < 2) return;
    const neighbour = index < siblings - 1 ? index : index - 1;
    const current = node.weight;
    onChange(resizePair(root, path.slice(0, -1), neighbour, neighbour === index ? weight - current : current - weight));
  };

  return (
    <div className="node-form">
      <div className="row wrap">
        {node.kind === "window" ? (
          <>
            <PatternField label="Application (regex)" value={node.application} onChange={application => edit({ application })} />
            <PatternField label="Titre (regex)" value={node.title} onChange={title => edit({ title })} />
          </>
        ) : (
          <div className="segmented" role="group" aria-label="Orientation du groupe">
            <button aria-pressed={node.axis !== "v"} onClick={() => edit({ axis: "h" })}>▥ Côte à côte</button>
            <button aria-pressed={node.axis === "v"} onClick={() => edit({ axis: "v" })}>▤ Empilées</button>
          </div>
        )}
        {!isRoot && (
          <label className="narrow">
            Taille %
            <input aria-label="Taille en pourcentage" type="number" min={5} max={95} value={Math.round(node.weight)}
              onChange={event => setWeight(Number(event.target.value))} />
          </label>
        )}
      </div>
      <div className="row wrap">
        {node.kind === "group" && (
          <>
            <button onClick={() => onChange(addChild(root, path, newLeaf()))}>✚ Fenêtre</button>
            <button onClick={() => onChange(addChild(root, path, newGroup("h")))}>✚ Groupe ▥</button>
            <button onClick={() => onChange(addChild(root, path, newGroup("v")))}>✚ Groupe ▤</button>
            {selectionCount > 0 && <button onClick={onAddSelection}>✚ Ajouter la sélection ({selectionCount})</button>}
          </>
        )}
        {node.kind === "window" && (
          <>
            <button onClick={() => split("h")}>Diviser ▥</button>
            <button onClick={() => split("v")}>Diviser ▤</button>
          </>
        )}
        {!isRoot && (
          <>
            <button aria-label="Déplacer avant" disabled={index === 0} onClick={() => { onChange(moveChild(root, path, -1)); onFocus([...path.slice(0, -1), index - 1]); }}>◀</button>
            <button aria-label="Déplacer après" disabled={index >= siblings - 1} onClick={() => { onChange(moveChild(root, path, 1)); onFocus([...path.slice(0, -1), index + 1]); }}>▶</button>
            <button className="danger" disabled={siblings <= 1} onClick={remove}>Retirer</button>
          </>
        )}
      </div>
    </div>
  );
}
