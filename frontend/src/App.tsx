import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, allDesktopsId, errorText, type Execution, type Group, type Item, type LayoutPlan, type Rule, type RulePreview, type Rules, type State, type Zone } from "./api";
import { AppDock } from "./studio/AppDock";
import { CommandBar } from "./studio/CommandBar";
import { LayoutEditor, validate } from "./studio/LayoutEditor";
import { Report } from "./studio/Report";
import { RuleList } from "./studio/RuleList";
import { StudioCanvas } from "./studio/StudioCanvas";
import { escapePattern, matchesRule, newGroup, newLeaf, ruleId } from "./studio/layoutTree";
import { GroupList, groupZone } from "./studio/GroupList";

interface EditorState {
  draft: Rule;
  isNew: boolean;
  dirty: boolean;
}

export function App() {
  const [state, setState] = useState<State | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [execution, setExecution] = useState<Execution | null>(null);
  const [spent, setSpent] = useState(false);
  const [selected, setSelected] = useState<string[]>([]);
  const [hovered, setHovered] = useState<string | null>(null);
  const [pinnedPreview, setPinnedPreview] = useState<{ ruleId: string; preview: RulePreview } | null>(null);
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [editorPreview, setEditorPreview] = useState<RulePreview | null>(null);
  const [editorError, setEditorError] = useState("");
  const [imported, setImported] = useState<Rules | null>(null);
  const [groupEditor, setGroupEditor] = useState<{ draft: Group; isNew: boolean; dirty: boolean } | null>(null);
  const [stale, setStale] = useState(false);
  const staleRef = useRef(false);
  const eventRevision = useRef(0);
  const [manualGroup, setManualGroup] = useState(false);
  const [groupControlsDirty, setGroupControlsDirty] = useState(false);

  const load = useCallback((next: State) => {
    if (!next.plan.id) {
      staleRef.current = true;
      setStale(true);
    }
    setState(next);
    setSpent(false);
    setPinnedPreview(null);
    setSelected(current => current.filter(id => next.plan.items.some(item => item.window.id === id)));
  }, []);

  const guard = useCallback(async (action: () => Promise<void>) => {
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await action();
    } catch (failure) {
      setError(errorText(failure));
    } finally {
      setBusy(false);
    }
  }, []);

  useEffect(() => {
    void guard(async () => load(await api.Refresh()));
  }, [guard, load]);
  useEffect(() => api.SubscribeGroups?.(event => {
    eventRevision.current++;
    staleRef.current = true;
    setStale(true);
    setNotice(event.message || "Les fenêtres ont changé. Actualisez explicitement avant d’appliquer.");
    if (event.error) setError(event.error);
  }), []);

  // Live preview of the rule being edited, debounced so typing a regex stays fluid.
  const previewTicket = useRef(0);
  const draft = editor?.draft;
  const groupDraft = groupEditor?.draft;
  useEffect(() => {
    if (!draft && !groupDraft) return;
    const ticket = ++previewTicket.current;
    if (validate(groupDraft ? { id: groupDraft.id, name: groupDraft.name, enabled: true, application: "", title: "", desktopId: "group", layout: groupDraft.layout, zone: groupDraft.zone } : draft!)) {
      setEditorPreview(null);
      setEditorError("");
      return;
    }
    const timer = setTimeout(() => {
      (groupDraft ? api.PreviewGroup(groupDraft) : api.PreviewRule(draft!)).then(
        preview => {
          if (ticket !== previewTicket.current) return;
          setEditorPreview(preview);
          setEditorError("");
        },
        failure => {
          if (ticket !== previewTicket.current) return;
          setEditorPreview(null);
          setEditorError(errorText(failure));
        },
      );
    }, 250);
    return () => { clearTimeout(timer); previewTicket.current++; };
  }, [draft, groupDraft]);

  useEffect(() => {
    const close = (event: KeyboardEvent) => {
      if (event.key === "Escape") { setEditor(null); setGroupEditor(null); }
    };
    window.addEventListener("keydown", close);
    return () => window.removeEventListener("keydown", close);
  }, []);

  const items = useMemo(() => state?.plan.items ?? [], [state]);
  const ghost: LayoutPlan | null = useMemo(() => {
    if (editor || groupEditor) return editorPreview?.layout ?? null;
    if (hovered) return state?.plan.layouts?.find(layout => layout.ruleId === hovered || layout.ruleIds?.includes(hovered)) ?? null;
    return pinnedPreview?.preview.layout ?? null;
  }, [editor, groupEditor, editorPreview, hovered, pinnedPreview, state]);

  const highlighted = useMemo(() => {
    if (editor || groupEditor) return new Set(editorPreview?.matches ?? []);
    if (!hovered && pinnedPreview) return new Set(pinnedPreview.preview.matches);
    const rule = state?.rules.rules.find(candidate => candidate.id === hovered);
    return new Set(rule?.groupId ? state?.plan.groups?.find(group => group.id === rule.groupId)?.members ?? [] : rule ? items.filter(item => matchesRule(rule, item)).map(item => item.window.id) : []);
  }, [editor, groupEditor, editorPreview, hovered, pinnedPreview, state, items]);

  if (!state) {
    return (
      <main className="splash">
        <span className="brand-mark" aria-hidden>◫</span>
        {error ? <p role="alert" className="error">{error}</p> : <p>Lecture des bureaux et des écrans…</p>}
      </main>
    );
  }

  const planId = state.plan.id;
  const planBlocked = stale || !planId;
  const arrange = state.plan.mode === "arrange";
  const moves = items.filter(item => item.status === "pending").length;
  const layoutsReady = (state.plan.layouts ?? []).filter(layout => layout.status === "pending").length;
  const selectedItems = selected.map(id => items.find(item => item.window.id === id)).filter((item): item is Item => !!item);
  const defaultDesktop = state.plan.currentDesktopId || state.plan.desktops[0]?.id || "";

  const select = (id: string, additive: boolean) =>
    setSelected(current => (additive ? (current.includes(id) ? current.filter(value => value !== id) : [...current, id]) : [id]));

  const refresh = () => {
    const manual = items.some(item => item.source === "manual");
    if ((manual || manualGroup || groupControlsDirty || editor?.dirty || groupEditor?.dirty) && !window.confirm("Actualiser abandonne vos choix manuels et l’édition en cours. Continuer ?")) return;
    const revision = eventRevision.current;
    void guard(async () => {
      const next = await api.Refresh();
      load(next);
      if (!next.plan.id) throw new Error("Plan indisponible : actualisez à nouveau avant toute modification.");
      setEditor(null); setGroupEditor(null); setExecution(null);
      if (revision === eventRevision.current) { staleRef.current = false; setStale(false); }
      setManualGroup(false); setGroupControlsDirty(false);
    });
  };

  const assignGroup = (groupId: string, desktopId: string, displayId: string, zone: Zone) => {
    if (staleRef.current || !planId) return setError("Plan périmé : actualisez avant de préparer une destination.");
    void guard(async () => { load(await api.AssignGroup(planId, groupId, desktopId, displayId, zone)); setManualGroup(true); });
  };
  const memberGroup = (id: string) => {
    const groups = (state.plan.groups ?? []).filter(group => group.members?.includes(id));
    if (groups.length > 1 || groups[0]?.status === "conflict") {
      setError("Conflit de groupes : cette fenêtre ne peut pas être déplacée isolément.");
      return false;
    }
    return groups[0];
  };
  const assign = (item: Item, desktopId: string) => {
    if (staleRef.current || !planId) return setError("Plan périmé : actualisez avant de préparer une destination.");
    const group = memberGroup(item.window.id);
    if (group === false) return;
    if (group) {
      const saved = state.rules.groups?.find(candidate => candidate.id === group.id);
      if (!saved) return setError("Groupe introuvable.");
      assignGroup(group.id, desktopId, group.displayId, groupZone(saved, group, state)); return;
    }
    void guard(async () => load(await api.Assign(planId, item.window.id, desktopId)));
  };

  const moveToDisplay = (item: Item, displayId: string) => {
    if (staleRef.current || !planId) return setError("Plan périmé : actualisez avant de préparer une destination.");
    const group = memberGroup(item.window.id);
    if (group === false) return;
    if (group) {
      const saved = state.rules.groups?.find(candidate => candidate.id === group.id);
      if (!saved) return setError("Groupe introuvable.");
      assignGroup(group.id, group.desktopId, displayId, groupZone(saved, group, state)); return;
    }
    void guard(async () => {
      await api.MoveToDisplay(planId, item.window.id, displayId);
      if (!items.some(candidate => candidate.source === "manual")) load(await api.Refresh());
      setNotice(`« ${item.window.title} » envoyée sur ${state.displays.find(display => display.id === displayId)?.name ?? displayId}.`);
    });
  };

  const run = (question: string, action: () => Promise<Execution>) => {
    if (staleRef.current || !planId) return setError("Plan périmé : actualisez avant d’appliquer.");
    if (!window.confirm(question)) return;
    void guard(async () => {
      setExecution(await action());
      setSpent(true);
    });
  };

  const saveRules = async (rules: Rule[], groups = state.rules.groups ?? []) => {
    if (staleRef.current || !planId) throw new Error("Plan périmé : actualisez avant d’enregistrer.");
    const next = await api.SaveRules({ version: 3, rules, groups }, planId);
    load(next);
    return next;
  };

  const openEditor = (rule: Rule, isNew: boolean) => {
    setEditorPreview(null);
    setEditorError("");
    setEditor({ draft: rule, isNew, dirty: isNew });
  };
  const openGroup = (group: Group, isNew: boolean) => {
    setEditorPreview(null); setEditorError("");
    setGroupEditor({ draft: structuredClone(group), isNew, dirty: isNew });
  };
  const captureGroup = (axis: "h" | "v") => void guard(async () => {
    const captured = await api.CaptureLayout(selected, axis);
    if (!captured.layout) throw new Error("La capture n’a renvoyé aucune disposition.");
    openGroup({ id: ruleId("groupe"), name: captured.name, layout: captured.layout }, true);
  });

  const ruleFromWindow = (item: Item) => openEditor({
    id: ruleId(item.window.application), name: item.window.application.replace(/\.exe$/i, ""), enabled: true,
    application: escapePattern(item.window.application), title: "",
    desktopId: item.targetId || (item.window.pinned ? allDesktopsId : item.window.desktopId) || defaultDesktop,
  }, true);

  const saveEditor = (rule: Rule, apply: boolean) => void guard(async () => {
    let groups = state.rules.groups ?? [];
    if (rule.layout) {
      const group: Group = { id: ruleId("groupe"), name: rule.name, layout: rule.layout };
      groups = [...groups, group];
      rule = { ...rule, groupId: group.id };
    }
    if (rule.groupId) {
      const { layout: _layout, application: _application, title: _title, ...reference } = rule;
      rule = reference;
    }
    const exists = state.rules.rules.some(existing => existing.id === rule.id);
    const rules = exists ? state.rules.rules.map(existing => (existing.id === rule.id ? rule : existing)) : [...state.rules.rules, rule];
    const next = await saveRules(rules, groups);
    setEditor(null);
    if (apply && next.plan.id && !staleRef.current && window.confirm(`Appliquer la règle « ${rule.name} » ?`)) {
      setExecution(await api.ApplyRule(next.plan.id, rule.id));
      setSpent(true);
      return;
    }
    setNotice(`Règle « ${rule.name} » enregistrée.`);
  });

  return (
    <div className="studio">
      <CommandBar
        state={state}
        busy={busy}
        canApply={!spent && !planBlocked && moves + layoutsReady > 0}
        applyLabel={layoutsReady ? `Tout appliquer · ${moves} déplacement(s) · ${layoutsReady} disposition(s)` : `Appliquer ${moves} déplacement(s)`}
        onRefresh={refresh}
        onUndo={() => void guard(async () => {
          setExecution(null);
          load(await api.PreviewUndo());
        })}
        onRestoreGeometry={() => run("Remettre les fenêtres à leurs positions d’avant la dernière disposition ?", () => api.RestoreGeometry())}
        onImport={() => void guard(async () => setImported(await api.ImportRules()))}
        onExport={() => void guard(async () => setNotice(`Règles exportées : ${await api.ExportRules()}`))}
        onApplyAll={() => run(`Appliquer ${moves} déplacement(s) et ${layoutsReady} disposition(s) ?`, () => api.ApplyAll(planId))}
        onTiling={(engine, gap) => void guard(async () => load(await api.SetTiling(engine, gap)))}
      />

      {error && <p role="alert" className="banner error">{error}</p>}
      {notice && <p role="status" className="banner notice">{notice}</p>}
      {planBlocked && <p role="alert" className="banner error">Plan périmé. Vos choix restent affichés. Actualisez explicitement avant d’enregistrer, de préparer ou d’appliquer un lot.</p>}

      <div className="workspace">
        <AppDock
          state={state}
          busy={busy || !arrange || planBlocked}
          selected={selected}
          highlighted={highlighted}
          onSelect={select}
          onAssign={assign}
          onMoveToDisplay={moveToDisplay}
          onRuleFromWindow={ruleFromWindow}
          onCapture={captureGroup}
        />

        <main className="center">
          <StudioCanvas
            state={state}
            busy={busy || !arrange || planBlocked}
            selected={selected}
            ghost={ghost}
            onSelect={select}
            onDropWindow={(id, desktopId) => {
              const item = items.find(candidate => candidate.window.id === id);
              if (item) assign(item, desktopId);
            }}
            onAssignGroup={assignGroup}
            onConflict={message => setError(message)}
            onDisplayIndex={(displayId, index) => void guard(async () => load(await api.SetDisplayIndex(displayId, index)))}
            onOpenDisplaySettings={() => void api.OpenDisplaySettings()}
          />
          {execution && <Report execution={execution} onClose={() => setExecution(null)} />}
        </main>

        <aside className="catalog">
        <GroupList state={state} busy={busy || !arrange} stale={planBlocked} runtimeAvailable={!!window.runtime?.EventsOn}
          onCreate={() => openGroup({ id: ruleId("groupe"), name: "", layout: newGroup("h", [newLeaf()]) }, true)}
          onEdit={group => openGroup(group, false)}
          onDelete={group => {
            const references = state.rules.rules.filter(rule => rule.groupId === group.id);
            if (references.length) { setError(`Suppression refusée : « ${group.name} » est référencé par ${references.map(rule => rule.name).join(", ")}.`); return; }
            if (window.confirm(`Supprimer le groupe « ${group.name} » ?`)) void guard(async () => { await saveRules(state.rules.rules, (state.rules.groups ?? []).filter(candidate => candidate.id !== group.id)); });
          }}
          onFollowing={(group, enabled) => void guard(async () => load(await api.SetGroupFollowing(group.id, enabled)))}
          onAssign={assignGroup}
          onDirty={() => setGroupControlsDirty(true)}
        />
        <RuleList
          state={state}
          busy={busy}
          mutationsDisabled={planBlocked}
          spent={spent || planBlocked}
          previewing={pinnedPreview?.ruleId ?? ""}
          onHover={setHovered}
          onPreview={rule => {
            if (pinnedPreview?.ruleId === rule.id) return setPinnedPreview(null);
            void guard(async () => setPinnedPreview({ ruleId: rule.id, preview: await api.PreviewRule(rule) }));
          }}
          onApply={rule => run(`Appliquer la règle « ${rule.name} » ?`, () => api.ApplyRule(planId, rule.id))}
          onToggle={rule => void guard(async () => {
            await saveRules(state.rules.rules.map(existing => (existing.id === rule.id ? { ...existing, enabled: !existing.enabled } : existing)));
          })}
          onEdit={rule => openEditor(structuredClone(rule), false)}
          onDelete={rule => {
            if (!window.confirm(`Supprimer la règle « ${rule.name} » ?`)) return;
            void guard(async () => {
              await saveRules(state.rules.rules.filter(existing => existing.id !== rule.id));
            });
          }}
          onCreate={() => openEditor({ id: ruleId("regle"), name: "", enabled: true, application: "", title: "", desktopId: defaultDesktop }, true)}
        />
        </aside>
      </div>

      {editor && (
        <div className="drawer">
          <LayoutEditor
            state={state}
            draft={editor.draft}
            isNew={editor.isNew}
            busy={busy}
            applyDisabled={planBlocked}
            saveDisabled={planBlocked}
            preview={editorPreview}
            previewError={editorError}
            selection={selectedItems}
            onChange={next => setEditor({ ...editor, draft: next, dirty: true })}
            onSave={saveEditor}
            onCancel={() => setEditor(null)}
            onEditGroup={id => { const group = state.rules.groups?.find(candidate => candidate.id === id); if (group) openGroup(group, false); }}
          />
        </div>
      )}
      {groupEditor && <div className="drawer group-drawer">
        <LayoutEditor state={state} groupMode draft={{ ...groupEditor.draft, enabled: true, application: "", title: "", desktopId: "group" }}
          isNew={groupEditor.isNew} busy={busy} saveDisabled={planBlocked} preview={editorPreview} previewError={editorError} selection={selectedItems}
          onChange={next => setGroupEditor({ ...groupEditor, draft: { id: next.id, name: next.name, layout: next.layout!, zone: next.zone }, dirty: true })}
          onCancel={() => { setGroupEditor(null); setEditorPreview(null); }}
          onSave={next => void guard(async () => {
            const group: Group = { id: next.id, name: next.name, layout: next.layout!, zone: next.zone };
            const groups = state.rules.groups ?? [];
            await saveRules(state.rules.rules, groups.some(candidate => candidate.id === group.id) ? groups.map(candidate => candidate.id === group.id ? group : candidate) : [...groups, group]);
            setGroupEditor(null); setNotice(`Groupe « ${group.name} » enregistré. Aucune fenêtre déplacée.`);
          })}
        />
      </div>}

      {imported && (
        <div className="drawer">
          <section className="editor" aria-label="Import de règles">
            <header>
              <h2>Importer {imported.rules.length} règle(s) et {(imported.groups ?? []).length} groupe(s)</h2>
              <button aria-label="Fermer l’import" onClick={() => setImported(null)}>✕</button>
            </header>
            <p className="muted">Ces règles et groupes remplaceront les règles et groupes actuels. Le suivi n’est pas activé par l’import.</p>
            <ul className="import-list">
              {imported.rules.map(rule => <li key={rule.id}><strong>{rule.name}</strong>{rule.layout ? " · disposition" : ""}</li>)}
              {(imported.groups ?? []).map(group => <li key={group.id}>Groupe : <strong>{group.name}</strong></li>)}
            </ul>
            <footer className="row">
              <button onClick={() => setImported(null)}>Annuler</button>
              <button className="primary" disabled={busy || planBlocked} onClick={() => void guard(async () => {
                if (staleRef.current || !planId) throw new Error("Plan périmé : actualisez avant d’enregistrer.");
                load(await api.SaveRules({ ...imported, version: 3, groups: imported.groups ?? [] }, planId));
                setImported(null);
              })}>Enregistrer l’import</button>
            </footer>
          </section>
        </div>
      )}
    </div>
  );
}
