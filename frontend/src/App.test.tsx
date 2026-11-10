import { beforeEach, expect, test, vi } from "vitest";
import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { App } from "./App";
import { api, type GroupEvent, type State } from "./api";

vi.mock("./api", async importOriginal => ({
  ...(await importOriginal<typeof import("./api")>()),
  api: {
    Refresh: vi.fn(), Assign: vi.fn(), MoveToDisplay: vi.fn(), OpenDisplaySettings: vi.fn(), SaveRules: vi.fn(), PreviewRule: vi.fn(),
    PreviewUndo: vi.fn(), Execute: vi.fn(), ApplyRule: vi.fn(), ApplyAll: vi.fn(), RestoreGeometry: vi.fn(), CaptureLayout: vi.fn(),
    SetDisplayIndex: vi.fn(), SetTiling: vi.fn(), ImportRules: vi.fn(), ExportRules: vi.fn(),
    AssignGroup: vi.fn(), PreviewGroup: vi.fn(), SetGroupFollowing: vi.fn(), SubscribeGroups: vi.fn(),
  },
}));
const mocked = vi.mocked(api);
const pinned = "00000000-0000-0000-0000-000000000000";
let initial: State;

beforeEach(() => {
  vi.resetAllMocks();
  vi.spyOn(window, "confirm").mockReturnValue(true);
  initial = {
    directory: "C:\\deskctl",
    config: { version: 1, fancyWM: { executable: "fancywm", displayIndices: {} }, tiling: { engine: "auto", gap: 8 } },
    fancyRunning: true,
    displays: [
      { id: "\\\\.\\DISPLAY1", name: "DISPLAY1", x: 0, y: 0, width: 1920, height: 1080, primary: true, fancyWMIndex: 1, work: { x: 0, y: 0, width: 1920, height: 1040 } },
      { id: "\\\\.\\DISPLAY2", name: "DISPLAY2", x: 1920, y: 0, width: 2560, height: 1440, primary: false, fancyWMIndex: 2, work: { x: 1920, y: 0, width: 2560, height: 1400 } },
    ],
    rules: { version: 2, rules: [] },
    plan: {
      id: "snapshot", mode: "arrange", currentDesktopId: "base",
      desktops: [{ id: "base", name: "Base" }, { id: "dev", name: "Développement" }],
      items: [{
        window: {
          id: "10", processId: 7, application: "Code", title: "deskctl — Code", class: "editor", started: "start", desktopId: "base", pinned: false, blocked: "",
          displayId: "\\\\.\\DISPLAY1", bounds: { x: 0, y: 0, width: 960, height: 1040 },
        },
        targetId: "dev", source: "rule", status: "pending", explanation: "Règle Projet", ruleIds: ["projet"],
      }, {
        window: { id: "11", processId: 8, application: "Teams", title: "Autre projet", class: "chat", started: "start", desktopId: "base", pinned: false, blocked: "" },
        targetId: "", source: "", status: "unmatched", explanation: "Aucune règle",
      }],
      layouts: [],
    },
  };
  mocked.Refresh.mockResolvedValue(initial);
  mocked.PreviewRule.mockResolvedValue({ matches: ["10"] });
  mocked.PreviewGroup.mockResolvedValue({ matches: ["10"] });
  mocked.AssignGroup.mockImplementation(async () => ({ ...initial, plan: { ...initial.plan, id: "group-revision" } }));
  mocked.SetGroupFollowing.mockImplementation(async (id, enabled) => ({ ...initial, following: enabled ? [id] : [], plan: { ...initial.plan, id: "following-revision" } }));
  mocked.SaveRules.mockImplementation(async rules => ({ ...initial, rules }));
  mocked.SubscribeGroups.mockReturnValue(() => {});
  delete window.runtime;
  mocked.ApplyAll.mockResolvedValue({
    error: "", report: { results: [{ id: "10", title: "deskctl — Code", status: "success", message: "Vérifié", kind: "desktop" }] },
  });
});

async function open() {
  const user = userEvent.setup();
  render(<App />);
  await screen.findByRole("heading", { name: "Développement" });
  return user;
}
const dock = () => within(screen.getByRole("region", { name: "Fenêtres" }));

test("selection prepare une destination par fenetre sans executer", async () => {
  const updated = structuredClone(initial);
  updated.plan.id = "next";
  Object.assign(updated.plan.items[1], { targetId: "dev", source: "manual", status: "pending" });
  mocked.Assign.mockResolvedValue(updated);
  const user = await open();
  await user.click(dock().getByRole("button", { name: /Autre projet/ }));
  await user.selectOptions(screen.getByRole("combobox", { name: "Destination pour Autre projet" }), "dev");
  await waitFor(() => expect(mocked.Assign).toHaveBeenCalledWith("snapshot", "11", "dev"));
  expect(mocked.ApplyAll).not.toHaveBeenCalled();
  expect(screen.getByRole("button", { name: "Appliquer 2 déplacement(s)" })).toBeEnabled();
});

test("l’option tous les bureaux prepare l’epinglage", async () => {
  mocked.Assign.mockResolvedValue(initial);
  const user = await open();
  expect(screen.getByRole("heading", { name: "Tous les bureaux" })).toBeVisible();
  await user.click(dock().getByRole("button", { name: /Autre projet/ }));
  await user.selectOptions(screen.getByRole("combobox", { name: "Destination pour Autre projet" }), pinned);
  await waitFor(() => expect(mocked.Assign).toHaveBeenCalledWith("snapshot", "11", pinned));
});

test("envoyer une fenetre sur un autre ecran", async () => {
  const user = await open();
  await user.click(dock().getByRole("button", { name: /deskctl/ }));
  expect(screen.getByRole("button", { name: /Envoyer sur DISPLAY1/ })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: /Envoyer sur DISPLAY2/ }));
  await waitFor(() => expect(mocked.MoveToDisplay).toHaveBeenCalledWith("snapshot", "10", "\\\\.\\DISPLAY2"));
  expect(await screen.findByText(/envoyée sur DISPLAY2/)).toBeVisible();
});

test("les ecrans sont reglables depuis le studio", async () => {
  mocked.SetDisplayIndex.mockResolvedValue(initial);
  const user = await open();
  await user.selectOptions(screen.getByRole("combobox", { name: "Indice FancyWM pour DISPLAY2" }), "0");
  await waitFor(() => expect(mocked.SetDisplayIndex).toHaveBeenCalledWith("\\\\.\\DISPLAY2", 0));
  await user.click(screen.getByRole("button", { name: "Paramètres d’affichage Windows" }));
  expect(mocked.OpenDisplaySettings).toHaveBeenCalledOnce();
});

test("glisser deposer sur un bureau prepare le plan sans deplacement reel", async () => {
  mocked.Assign.mockResolvedValue(initial);
  await open();
  const payload = new Map<string, string>();
  const dataTransfer = {
    dropEffect: "none", effectAllowed: "all",
    setData: (key: string, value: string) => { payload.set(key, value); },
    getData: (key: string) => payload.get(key) ?? "",
  };
  const source = dock().getByRole("button", { name: /Autre projet/ });
  const target = screen.getByRole("heading", { name: "Développement" }).closest("article")!;
  fireEvent.dragStart(source, { dataTransfer });
  fireEvent.dragOver(target, { dataTransfer });
  fireEvent.drop(target, { dataTransfer });
  await waitFor(() => expect(mocked.Assign).toHaveBeenCalledWith("snapshot", "11", "dev"));
  expect(mocked.ApplyAll).not.toHaveBeenCalled();
});

test("appliquer exige une confirmation et ne se repete pas apres le bilan", async () => {
  const user = await open();
  vi.mocked(window.confirm).mockReturnValueOnce(false);
  await user.click(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" }));
  expect(mocked.ApplyAll).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" }));
  await waitFor(() => expect(mocked.ApplyAll).toHaveBeenCalledExactlyOnceWith("snapshot"));
  await screen.findByRole("heading", { name: "Bilan du lot" });
  expect(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" })).toBeDisabled();
});

test("regle issue d’une fenetre : apercu puis enregistrement en regex", async () => {
  const updated = structuredClone(initial);
  mocked.SaveRules.mockResolvedValue(updated);
  const user = await open();
  await user.click(dock().getByRole("button", { name: /deskctl/ }));
  await user.click(screen.getByRole("button", { name: /En faire une règle/ }));
  await screen.findByText("1 fenêtre(s) correspondent");
  expect(mocked.SaveRules).not.toHaveBeenCalled();
  await user.click(screen.getByRole("button", { name: "Enregistrer cette règle" }));
  await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledTimes(1));
  const [doc, revision] = mocked.SaveRules.mock.calls[0];
  expect(revision).toBe("snapshot");
  expect(doc.version).toBe(3);
  expect(doc.groups).toEqual([]);
  expect(doc.rules[0]).toMatchObject({ name: "Code", application: "^Code$", desktopId: "dev", enabled: true });
  expect(mocked.ApplyRule).not.toHaveBeenCalled();
});

test("grouper la selection capture une disposition et l’editeur regle les tailles", async () => {
  mocked.CaptureLayout.mockResolvedValue({
    id: "code-teams", name: "Code + Teams", enabled: true, application: "", title: "", desktopId: "dev", displayId: "\\\\.\\DISPLAY2",
    layout: { kind: "group", axis: "h", weight: 100, children: [
      { kind: "window", weight: 60, application: "^Code$" },
      { kind: "window", weight: 40, application: "^Teams$" },
    ] },
  });
  mocked.SaveRules.mockResolvedValue(initial);
  const user = await open();
  await user.click(dock().getByRole("button", { name: /deskctl/ }));
  fireEvent.click(dock().getByRole("button", { name: /Autre projet/ }), { ctrlKey: true });
  await user.click(screen.getByRole("button", { name: /Grouper côte à côte/ }));
  await waitFor(() => expect(mocked.CaptureLayout).toHaveBeenCalledWith(["10", "11"], "h"));

  const editor = within(await screen.findByRole("region", { name: "Éditeur de groupe" }));
  await user.click(editor.getByRole("button", { name: "Zone Code" }));
  const size = editor.getByRole("spinbutton", { name: "Taille en pourcentage" });
  fireEvent.change(size, { target: { value: "70" } });
  await user.click(editor.getByRole("button", { name: "Enregistrer ce groupe" }));
  await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledTimes(1));
  const document = mocked.SaveRules.mock.calls[0][0];
  expect(document.groups![0].layout.children!.map(child => child.weight)).toEqual([70, 30]);
  expect(document.rules).toEqual([]);
  expect(mocked.ApplyRule).not.toHaveBeenCalled();
});

test("une regle s’applique seule avec son bouton", async () => {
  initial.rules.rules = [{ id: "projet", name: "Projet", enabled: true, application: "^Code$", title: "", desktopId: "dev" }];
  mocked.ApplyRule.mockResolvedValue({ error: "", report: { results: [] } });
  const user = await open();
  await user.click(screen.getByRole("button", { name: "Appliquer Projet" }));
  await waitFor(() => expect(mocked.ApplyRule).toHaveBeenCalledExactlyOnceWith("snapshot", "projet"));
  expect(mocked.ApplyAll).not.toHaveBeenCalled();
});

test("actualiser ne perd pas silencieusement les choix manuels", async () => {
  initial.plan.items[1].source = "manual";
  const user = await open();
  vi.mocked(window.confirm).mockReturnValueOnce(false);
  await user.click(screen.getByRole("button", { name: /Actualiser/ }));
  expect(mocked.Refresh).toHaveBeenCalledTimes(1);
});

test("erreur partielle conserve bilan et donnees de recuperation", async () => {
  mocked.ApplyAll.mockResolvedValue({
    error: "Journal inaccessible",
    report: { results: [{ id: "10", title: "deskctl — Code", status: "uncertain", message: "État à vérifier" }], recovery: { batch: "lot" } },
  });
  const user = await open();
  await user.click(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Journal inaccessible");
  expect(screen.getByText("Incertain")).toBeVisible();
  expect(screen.getByRole("textbox", { name: "Données de récupération" })).toHaveValue('{\n  "batch": "lot"\n}');
});

  function withGroup() {
    initial.rules.groups = [{
      id: "work", name: "Travail", layout: { kind: "group", axis: "h", weight: 100, children: [
        { kind: "window", weight: 60, application: "^Code$" }, { kind: "window", weight: 40, application: "^Teams$" },
      ] }, zone: { x: .1, y: .1, width: .5, height: .6 },
    }];
    initial.plan.groups = [{
      id: "work", name: "Travail", members: ["10", "11"], displayId: initial.displays[0].id, desktopId: "dev",
      bounds: { x: 192, y: 104, width: 960, height: 624 }, status: "ready", explanation: "Groupe prêt",
    }];
  }

  test("creer un groupe sans fenetres, criteres persistants et aucune application automatique", async () => {
    initial.plan.items = [];
    const user = await open();
    await user.click(screen.getByRole("button", { name: /Nouveau groupe/ }));
    const editor = within(screen.getByRole("region", { name: "Éditeur de groupe" }));
    await user.type(editor.getByRole("textbox", { name: "Nom du groupe" }), "Futur");
    await user.click(editor.getByRole("button", { name: "Zone toute fenêtre" }));
    await user.type(editor.getByRole("textbox", { name: "Application (regex)" }), "^Code$");
    await user.click(editor.getByRole("button", { name: "Enregistrer ce groupe" }));
    await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledOnce());
    expect(mocked.SaveRules.mock.calls[0][0]).toMatchObject({
      version: 3, rules: [], groups: [{ name: "Futur", layout: { children: [{ application: "^Code$" }] } }],
    });
    expect(mocked.ApplyAll).not.toHaveBeenCalled();
  });

  test("referencer un groupe partage, modifier ses criteres une seule fois et refuser sa suppression referencee", async () => {
    withGroup();
    initial.rules.rules = [{ id: "shared-rule", name: "Bureau travail", enabled: true, desktopId: "dev", groupId: "work" }];
    const user = await open();
    await user.click(screen.getByRole("button", { name: "Supprimer le groupe Travail" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Bureau travail");
    expect(mocked.SaveRules).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Modifier Bureau travail" }));
    const editor = within(screen.getByRole("region", { name: "Éditeur de règle" }));
    expect(editor.queryByRole("separator")).not.toBeInTheDocument();
    expect(editor.queryByRole("textbox", { name: "Application (regex)" })).not.toBeInTheDocument();
    await user.click(editor.getByRole("button", { name: "Modifier le groupe partagé" }));
    const groupEditor = within(screen.getByRole("region", { name: "Éditeur de groupe" }));
    await user.clear(groupEditor.getByRole("textbox", { name: "Nom du groupe" }));
    await user.type(groupEditor.getByRole("textbox", { name: "Nom du groupe" }), "Commun");
    await user.click(groupEditor.getByRole("button", { name: "Enregistrer ce groupe" }));
    await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledOnce());
    expect(mocked.SaveRules.mock.calls[0][0]).toMatchObject({ groups: [{ id: "work", name: "Commun" }], rules: [{ groupId: "work", id: "shared-rule" }] });
  });

  test("nouvelle regle de groupe ne persiste pas de criteres locaux et les bascules preservent le catalogue", async () => {
    withGroup();
    const user = await open();
    await user.click(screen.getByRole("button", { name: /Nouvelle règle/ }));
    const editor = within(screen.getByRole("region", { name: "Éditeur de règle" }));
    await user.type(editor.getByRole("textbox", { name: "Nom de la règle" }), "Partagée");
    await user.selectOptions(editor.getByRole("combobox", { name: "Groupe de la règle" }), "work");
    await user.click(editor.getByRole("button", { name: "Enregistrer cette règle" }));
    await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledOnce());
    const rule = mocked.SaveRules.mock.calls[0][0].rules[0];
    expect(rule).toMatchObject({ groupId: "work" });
    expect(rule).not.toHaveProperty("application");
    expect(rule).not.toHaveProperty("title");
    expect(rule).not.toHaveProperty("layout");
    await user.click(await screen.findByRole("checkbox", { name: "Activer Partagée" }));
    await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledTimes(2));
    expect(mocked.SaveRules.mock.calls[1][0].groups).toEqual(initial.rules.groups);
  });

  test("destination accessible de groupe : une revision, fractions et tous les bureaux", async () => {
    withGroup();
    const user = await open();
    await user.click(screen.getByText("Préparer la destination de Travail"));
    await user.selectOptions(screen.getByRole("combobox", { name: "Bureau du groupe Travail" }), pinned);
    await user.selectOptions(screen.getByRole("combobox", { name: "Écran du groupe Travail" }), initial.displays[1].id);
    fireEvent.change(screen.getByRole("spinbutton", { name: "Zone de Travail x (%)" }), { target: { value: "25" } });
    await user.click(screen.getByRole("button", { name: "Préparer le groupe Travail" }));
    await waitFor(() => expect(mocked.AssignGroup).toHaveBeenCalledExactlyOnceWith("snapshot", "work", pinned, initial.displays[1].id, { x: .25, y: .1, width: .5, height: .6 }));
    expect(mocked.Assign).not.toHaveBeenCalled();
    expect(mocked.MoveToDisplay).not.toHaveBeenCalled();
    expect(mocked.ApplyAll).not.toHaveBeenCalled();
  });

  test("depot sur ecran garde tout le groupe, borne la zone et ne double pas le depot fenetre", async () => {
    withGroup();
    await open();
    const target = screen.getByRole("group", { name: "DISPLAY2 dans Tous les bureaux" });
    vi.spyOn(target.parentElement!, "getBoundingClientRect").mockReturnValue({ x: 0, y: 0, left: 0, top: 0, right: 448, bottom: 144, width: 448, height: 144, toJSON: () => ({}) });
    const payload = new Map<string, string>([["text/plain", "10"]]);
    const dataTransfer = { getData: (key: string) => payload.get(key) ?? "", setData: (key: string, value: string) => payload.set(key, value), dropEffect: "none" };
    const drop = new MouseEvent("drop", { bubbles: true, clientX: 440, clientY: 130 });
    Object.defineProperty(drop, "dataTransfer", { value: dataTransfer });
    fireEvent(target, drop);
    await waitFor(() => expect(mocked.AssignGroup).toHaveBeenCalledOnce());
    expect(mocked.AssignGroup.mock.calls[0]).toEqual(["snapshot", "work", pinned, initial.displays[1].id, { x: .5, y: .4, width: .5, height: .6 }]);
    expect(mocked.Assign).not.toHaveBeenCalled();
    expect(mocked.MoveToDisplay).not.toHaveBeenCalled();
  });

  test("une appartenance ambigue bloque le deplacement individuel", async () => {
    withGroup();
    initial.plan.groups!.push({ ...initial.plan.groups![0], id: "other", name: "Concurrent" });
    const user = await open();
    await user.click(dock().getByRole("button", { name: /deskctl/ }));
    await user.selectOptions(screen.getByRole("combobox", { name: "Destination pour deskctl — Code" }), "base");
    expect(screen.getByRole("alert")).toHaveTextContent("Conflit de groupes");
    expect(mocked.Assign).not.toHaveBeenCalled();
    expect(mocked.AssignGroup).not.toHaveBeenCalled();
  });

  test("suivi explicite preserve une regle non enregistree et utilise la revision retournee", async () => {
    withGroup();
    initial.fancyRunning = false;
    window.runtime = { EventsOn: vi.fn(() => () => {}), EventsOff: vi.fn() };
    const user = await open();
    expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).not.toBeChecked();
    await user.click(screen.getByRole("button", { name: /Nouvelle règle/ }));
    await user.type(screen.getByRole("textbox", { name: "Nom de la règle" }), "Pas perdue");
    await user.click(screen.getByRole("checkbox", { name: "Suivre Travail" }));
    await waitFor(() => expect(mocked.SetGroupFollowing).toHaveBeenCalledExactlyOnceWith("work", true));
    expect(screen.getByRole("textbox", { name: "Nom de la règle" })).toHaveValue("Pas perdue");
    expect(mocked.ApplyAll).not.toHaveBeenCalled();
    await user.click(screen.getByRole("button", { name: "Annuler" }));
    await user.click(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" }));
    await waitFor(() => expect(mocked.ApplyAll).toHaveBeenCalledWith("following-revision"));
  });

  test("evenement de suivi invalide le plan sans perdre les choix, exige actualisation et nettoie l’abonnement", async () => {
    withGroup();
    initial.plan.items[1].source = "manual";
    let eventListener: ((event: GroupEvent) => void) | undefined;
    const cancel = vi.fn();
    mocked.SubscribeGroups.mockImplementation(listener => { eventListener = listener; return cancel; });
    const user = await open();
    await user.click(screen.getByRole("button", { name: /Nouvelle règle/ }));
    await user.type(screen.getByRole("textbox", { name: "Nom de la règle" }), "Brouillon");
    act(() => eventListener!({ message: "Groupe déplacé", error: "Hook interrompu", groupId: "work" }));
    expect(screen.getByText("Hook interrompu")).toBeVisible();
    expect(screen.getByRole("textbox", { name: "Nom de la règle" })).toHaveValue("Brouillon");
    expect(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" })).toBeDisabled();
    expect(mocked.Refresh).toHaveBeenCalledTimes(1);
    vi.mocked(window.confirm).mockReturnValueOnce(false);
    await user.click(screen.getByRole("button", { name: /Actualiser/ }));
    expect(mocked.Refresh).toHaveBeenCalledTimes(1);
    await user.click(screen.getByRole("button", { name: /Actualiser/ }));
    await waitFor(() => expect(mocked.Refresh).toHaveBeenCalledTimes(2));
    expect(screen.queryByText(/Plan périmé/)).not.toBeInTheDocument();
    cleanup();
    expect(cancel).toHaveBeenCalledOnce();
  });

test("redimensionner un cadre prepare une seule revision au relachement", async () => {
  withGroup();
  await open();
  const handle = screen.getByRole("button", { name: "Redimensionner le groupe Travail" });
  vi.spyOn(handle.closest(".stage")!, "getBoundingClientRect").mockReturnValue({ x: 0, y: 0, left: 0, top: 0, right: 448, bottom: 144, width: 448, height: 144, toJSON: () => ({}) });
  fireEvent(handle, new MouseEvent("pointerdown", { bubbles: true, clientX: 100, clientY: 100 }));
  fireEvent(window, new MouseEvent("pointermove", { bubbles: true, clientX: 110, clientY: 105 }));
  expect(mocked.AssignGroup).not.toHaveBeenCalled();
  fireEvent(window, new MouseEvent("pointerup", { bubbles: true, clientX: 110, clientY: 105 }));
  await waitFor(() => expect(mocked.AssignGroup).toHaveBeenCalledOnce());
  const [revision, id, desktopId, displayId, zone] = mocked.AssignGroup.mock.calls[0];
  expect([revision, id, desktopId, displayId]).toEqual(["snapshot", "work", "dev", initial.displays[0].id]);
  expect(zone.width).toBeCloseTo(1060 / 1920);
  expect(zone.height).toBeCloseTo(674 / 1040);
  expect(mocked.MoveToDisplay).not.toHaveBeenCalled();
  expect(mocked.ApplyAll).not.toHaveBeenCalled();
});

test("ajouter des fenetres de meme application enregistre des titres distinctifs", async () => {
  withGroup();
  initial.plan.items[1].window.application = "Code";
  const user = await open();
  await user.click(dock().getByRole("button", { name: /deskctl/ }));
  fireEvent.click(dock().getByRole("button", { name: /Autre projet/ }), { ctrlKey: true });
  await user.click(screen.getByRole("button", { name: "Modifier le groupe Travail" }));
  const editor = within(screen.getByRole("region", { name: "Éditeur de groupe" }));
  await user.click(editor.getByRole("button", { name: "✚ Ajouter la sélection (2)" }));
  await user.click(editor.getByRole("button", { name: "Enregistrer ce groupe" }));
  await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledOnce());
  expect(mocked.SaveRules.mock.calls[0][0].groups![0].layout.children!.slice(-2)).toMatchObject([
    { application: "^Code$", title: "^deskctl — Code$" }, { application: "^Code$", title: "^Autre projet$" },
  ]);
});

test("import affiche et remplace les deux catalogues sans activer le suivi", async () => {
  withGroup();
  const imported = { version: 3, rules: [], groups: [{ ...initial.rules.groups![0], id: "import", name: "Importé" }] };
  mocked.ImportRules.mockResolvedValue(imported);
  const user = await open();
  await user.click(screen.getByRole("button", { name: /Importer/ }));
  expect(await screen.findByRole("heading", { name: "Importer 0 règle(s) et 1 groupe(s)" })).toBeVisible();
  expect(screen.getByText("Importé")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Enregistrer l’import" }));
  await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledExactlyOnceWith(imported, "snapshot"));
  expect(mocked.SetGroupFollowing).not.toHaveBeenCalled();
});

test("erreur d’activation de suivi explicite et suivi indisponible sans runtime", async () => {
  withGroup();
  initial.fancyRunning = false;
  const user = await open();
  expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).toBeDisabled();
  cleanup();
  window.runtime = { EventsOn: vi.fn(() => () => {}), EventsOff: vi.fn() };
  mocked.SetGroupFollowing.mockRejectedValue(new Error("Installation du hook refusée"));
  await open();
  await user.click(screen.getByRole("checkbox", { name: "Suivre Travail" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("Installation du hook refusée");
  expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).not.toBeChecked();
});

test("FancyWM actif interdit le suivi mais pas la preparation manuelle du groupe", async () => {
  withGroup();
  window.runtime = { EventsOn: vi.fn(() => () => {}), EventsOff: vi.fn() };
  const user = await open();
  expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).toBeDisabled();
  expect(screen.getByText(/Suivi indisponible pendant que FancyWM est actif/)).toBeVisible();
  await user.click(screen.getByText("Préparer la destination de Travail"));
  await user.click(screen.getByRole("button", { name: "Préparer le groupe Travail" }));
  await waitFor(() => expect(mocked.AssignGroup).toHaveBeenCalledOnce());
  expect(mocked.SetGroupFollowing).not.toHaveBeenCalled();
});

test("un etat de suivi sans ID bloque sauvegardes et destinations jusqu’a Actualiser", async () => {
  withGroup();
  initial.fancyRunning = false;
  initial.rules.rules = [{ id: "shared", name: "Partagée", enabled: true, desktopId: "dev", groupId: "work" }];
  window.runtime = { EventsOn: vi.fn(() => () => {}), EventsOff: vi.fn() };
  mocked.SetGroupFollowing.mockResolvedValue({ ...initial, following: ["work"], plan: { ...initial.plan, id: "" } });
  const user = await open();
  await user.click(screen.getByRole("button", { name: "Modifier Partagée" }));
  const editor = within(screen.getByRole("region", { name: "Éditeur de règle" }));
  await user.type(editor.getByRole("textbox", { name: "Nom de la règle" }), " brouillon");
  await user.click(screen.getByRole("checkbox", { name: "Suivre Travail" }));
  await waitFor(() => expect(mocked.SetGroupFollowing).toHaveBeenCalledOnce());
  expect(editor.getByRole("textbox", { name: "Nom de la règle" })).toHaveValue("Partagée brouillon");
  expect(editor.getByRole("button", { name: "Enregistrer cette règle" })).toBeDisabled();
  expect(editor.getByRole("button", { name: "Enregistrer et appliquer" })).toBeDisabled();
  expect(screen.getByRole("checkbox", { name: "Activer Partagée" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Supprimer Partagée" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Supprimer le groupe Travail" })).toBeDisabled();
  await user.click(screen.getByText("Préparer la destination de Travail"));
  expect(screen.getByRole("button", { name: "Préparer le groupe Travail" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" })).toBeDisabled();
  expect(mocked.SaveRules).not.toHaveBeenCalled();
  expect(mocked.AssignGroup).not.toHaveBeenCalled();
  mocked.SetGroupFollowing.mockResolvedValue({ ...initial, following: [], plan: { ...initial.plan, id: "reissued-but-not-refreshed" } });
  await user.click(screen.getByRole("checkbox", { name: "Suivre Travail" }));
  await waitFor(() => expect(mocked.SetGroupFollowing).toHaveBeenCalledTimes(2));
  expect(editor.getByRole("button", { name: "Enregistrer cette règle" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Appliquer 1 déplacement(s)" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: /Actualiser/ }));
  await waitFor(() => expect(screen.queryByText(/Plan périmé/)).not.toBeInTheDocument());
  expect(screen.getByRole("checkbox", { name: "Activer Partagée" })).toBeEnabled();
});

test("un evenement bloque aussi l’enregistrement d’un groupe et de l’import", async () => {
  withGroup();
  let notify: ((event: GroupEvent) => void) | undefined;
  mocked.SubscribeGroups.mockImplementation(listener => { notify = listener; return () => {}; });
  mocked.ImportRules.mockResolvedValue({ version: 3, rules: [], groups: [] });
  const user = await open();
  await user.click(screen.getByRole("button", { name: "Modifier le groupe Travail" }));
  await user.click(screen.getByRole("button", { name: /Importer/ }));
  act(() => notify!({ message: "Déplacé", error: "", groupId: "work" }));
  expect(screen.getByRole("button", { name: "Enregistrer ce groupe" })).toBeDisabled();
  expect(screen.getByRole("button", { name: "Enregistrer l’import" })).toBeDisabled();
  expect(mocked.SaveRules).not.toHaveBeenCalled();
});

test("tous les alias de regle affichent la disposition partagee et peuvent l’appliquer", async () => {
  withGroup();
  initial.plan.items.forEach(item => { item.status = "unchanged"; item.ruleIds = []; });
  initial.rules.rules = [
    { id: "first", name: "Première", enabled: true, desktopId: "dev", groupId: "work" },
    { id: "alias", name: "Alias", enabled: true, desktopId: "dev", groupId: "work" },
  ];
  initial.plan.layouts = [{
    ruleId: "first", ruleIds: ["first", "alias"], groupId: "work", ruleName: "Première", displayId: initial.displays[0].id,
    desktopId: "dev", pinned: false, slots: [], status: "pending", explanation: "Arbre commun prêt",
  }];
  mocked.ApplyRule.mockResolvedValue({ error: "", report: { results: [] } });
  const user = await open();
  expect(screen.getAllByText("Disposition prête")).toHaveLength(2);
  expect(screen.getByRole("button", { name: "Appliquer Première" })).toBeEnabled();
  expect(screen.getByRole("button", { name: "Appliquer Alias" })).toBeEnabled();
  await user.click(screen.getByRole("button", { name: "Appliquer Alias" }));
  await waitFor(() => expect(mocked.ApplyRule).toHaveBeenCalledExactlyOnceWith("snapshot", "alias"));
});

test("l’aperçu d’un groupe non enregistre passe le brouillon complet a PreviewGroup", async () => {
  const user = await open();
  await user.click(screen.getByRole("button", { name: /Nouveau groupe/ }));
  await user.type(screen.getByRole("textbox", { name: "Nom du groupe" }), "Nouveau");
  await user.click(screen.getByRole("button", { name: "Zone toute fenêtre" }));
  await user.type(screen.getByRole("textbox", { name: "Application (regex)" }), "^Code$");
  await waitFor(() => expect(mocked.PreviewGroup).toHaveBeenCalled());
  expect(mocked.PreviewGroup.mock.lastCall![0]).toMatchObject({ name: "Nouveau", layout: { children: [{ application: "^Code$" }] } });
  expect(mocked.PreviewRule).not.toHaveBeenCalled();
});

test("enregistrer les regles affiche le suivi vide renvoye par le backend", async () => {
  withGroup();
  initial.fancyRunning = false;
  initial.following = ["work"];
  initial.rules.rules = [{ id: "shared", name: "Partagée", enabled: true, desktopId: "dev", groupId: "work" }];
  window.runtime = { EventsOn: vi.fn(() => () => {}), EventsOff: vi.fn() };
  mocked.SaveRules.mockImplementation(async rules => ({ ...initial, rules, following: [] }));
  const user = await open();
  expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).toBeChecked();
  await user.click(screen.getByRole("checkbox", { name: "Activer Partagée" }));
  await waitFor(() => expect(mocked.SaveRules).toHaveBeenCalledOnce());
  expect(screen.getByRole("checkbox", { name: "Suivre Travail" })).not.toBeChecked();
});
