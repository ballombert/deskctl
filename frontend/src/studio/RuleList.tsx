import type { Rule, State } from "../api";
import { desktopName, displayName, layoutStatus } from "./labels";
import { describeRule, matchesRule } from "./layoutTree";

interface Props {
  state: State;
  busy: boolean;
  spent: boolean;
  mutationsDisabled?: boolean;
  previewing: string;
  onHover(ruleId: string | null): void;
  onPreview(rule: Rule): void;
  onApply(rule: Rule): void;
  onToggle(rule: Rule): void;
  onEdit(rule: Rule): void;
  onDelete(rule: Rule): void;
  onCreate(): void;
}

export function RuleList({ state, busy, spent, mutationsDisabled, previewing, ...actions }: Props) {
  const rules = state.rules.rules;
  return (
    <aside className="rules" aria-label="Règles" role="region">
      <header>
        <h2>Règles <span className="count">{rules.length}</span></h2>
        <button className="primary" disabled={busy || state.plan.mode !== "arrange"} onClick={actions.onCreate}>✚ Nouvelle règle</button>
      </header>
      {rules.length === 0 && <p className="muted empty">Aucune règle. Sélectionnez des fenêtres puis « Grouper », ou créez-en une.</p>}
      <ol className="rule-list">
        {rules.map(rule => {
          const group = state.rules.groups?.find(group => group.id === rule.groupId);
          const matches = rule.groupId ? state.plan.groups?.find(group => group.id === rule.groupId)?.members?.length ?? 0 : state.plan.items.filter(item => matchesRule(rule, item)).length;
          const layout = state.plan.layouts?.find(plan => plan.ruleId === rule.id || plan.ruleIds?.includes(rule.id));
          const moves = state.plan.items.filter(item => item.status === "pending" && item.ruleIds?.includes(rule.id)).length;
          const actionable = rule.enabled && (moves > 0 || layout?.status === "pending");
          return (
            <li key={rule.id} className={`rule-card ${rule.enabled ? "" : "disabled"} ${previewing === rule.id ? "previewing" : ""}`}
              onMouseEnter={() => actions.onHover(rule.id)} onMouseLeave={() => actions.onHover(null)}
              onFocus={() => actions.onHover(rule.id)} onBlur={() => actions.onHover(null)}>
              <header>
                <label className="switch" title={rule.enabled ? "Désactiver" : "Activer"}>
                  <input type="checkbox" aria-label={`Activer ${rule.name}`} checked={rule.enabled} disabled={busy || mutationsDisabled} onChange={() => actions.onToggle(rule)} />
                  <span aria-hidden />
                </label>
                <h3>{rule.name}</h3>
                <span className="count" title="Fenêtres correspondantes">{matches}</span>
              </header>
              <p className="pattern">{rule.groupId ? `Groupe partagé : ${group?.name ?? "introuvable"}` : describeRule(rule)}</p>
              <p className="target">
                → {desktopName(state.plan.desktops, rule.desktopId)}
                {(rule.displayId || rule.layout || rule.groupId) && <> · {displayName(state.displays, rule.displayId)}</>}
              </p>
              <p className="muted small">
                {moves > 0 && <span className="badge pending">{moves} déplacement(s)</span>}
                {layout && <span className={`badge ${layout.status}`} title={layout.explanation}>{layoutStatus[layout.status] ?? layout.status}</span>}
              </p>
              <div className="row">
                <button disabled={busy} aria-pressed={previewing === rule.id} onClick={() => actions.onPreview(rule)}>👁 Aperçu</button>
                <button className="accent" disabled={busy || spent || !actionable} aria-label={`Appliquer ${rule.name}`} onClick={() => actions.onApply(rule)}>▶ Appliquer</button>
                <button disabled={busy} aria-label={`Modifier ${rule.name}`} onClick={() => actions.onEdit(rule)}>✎</button>
                <button disabled={busy || mutationsDisabled} aria-label={`Supprimer ${rule.name}`} onClick={() => actions.onDelete(rule)}>🗑</button>
              </div>
            </li>
          );
        })}
      </ol>
    </aside>
  );
}
