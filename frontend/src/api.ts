export interface Rect {
  x: number;
  y: number;
  width: number;
  height: number;
}
export interface Desktop {
  id: string;
  name: string;
}
export interface Display {
  id: string;
  name: string;
  x: number;
  y: number;
  width: number;
  height: number;
  primary: boolean;
  fancyWMIndex: number;
  work: Rect;
}
export interface WindowInfo {
  id: string;
  processId: number;
  application: string;
  title: string;
  class: string;
  started: string;
  desktopId: string;
  pinned: boolean;
  blocked: string;
  blockCode?: string;
  displayId?: string;
  bounds?: Rect;
  elevated?: boolean;
}
export interface Item {
  window: WindowInfo;
  targetId: string;
  source: string;
  status: string;
  explanation: string;
  ruleIds?: string[];
}
export interface Slot {
  windowId: string;
  application: string;
  title: string;
  path?: string;
  rect: Rect;
  before: Rect;
  elevated?: boolean;
}
export interface LayoutPlan {
  groupId?: string;
  bounds?: Rect;
  ruleIds?: string[];
  ruleId: string;
  ruleName: string;
  displayId: string;
  desktopId: string;
  pinned: boolean;
  layout?: LayoutNode;
  slots: Slot[];
  status: string;
  explanation: string;
}
export interface Plan {
  groups?: GroupState[];
  id: string;
  mode: string;
  desktops: Desktop[];
  currentDesktopId?: string;
  items: Item[];
  layouts?: LayoutPlan[];
}
/** A layout tree: groups split their space ("h" side by side, "v" stacked); weights are percentages. */
export interface LayoutNode {
  kind: "group" | "window";
  axis?: "h" | "v";
  weight: number;
  application?: string;
  title?: string;
  children?: LayoutNode[];
}
export interface Rule {
  id: string;
  name: string;
  enabled: boolean;
  application?: string;
  title?: string;
  desktopId: string;
  displayId?: string;
  layout?: LayoutNode;
  groupId?: string;
  zone?: Zone;
}
export interface Zone { x: number; y: number; width: number; height: number }
export interface Group { id: string; name: string; layout: LayoutNode; zone?: Zone }
export interface GroupState {
  id: string; name: string; members: string[]; displayId: string; desktopId: string;
  bounds: Rect; status: string; explanation: string;
}
export interface Rules {
  version: number;
  rules: Rule[];
  groups?: Group[];
}
export interface AppConfig {
  version: number;
  fancyWM: { executable: string; displayIndices: Record<string, number> };
  tiling?: { engine: string; gap: number };
}
export interface State {
  following?: string[];
  trackingError?: string;
  plan: Plan;
  rules: Rules;
  directory: string;
  config: AppConfig;
  displays: Display[];
  fancyRunning?: boolean;
}
export interface Result {
  id: string;
  title: string;
  status: string;
  message: string;
  kind?: string;
  engine?: string;
}
export interface Execution {
  report: { results: Result[]; recovery?: unknown; layouts?: LayoutPlan[] };
  error: string;
}
export interface RulePreview {
  matches: string[];
  layout?: LayoutPlan;
}
export interface Bridge {
  AssignGroup(planID: string, groupID: string, desktopID: string, displayID: string, zone: Zone): Promise<State>;
  PreviewGroup(group: Group): Promise<RulePreview>;
  SetGroupFollowing(groupID: string, enabled: boolean): Promise<State>;
  Refresh(): Promise<State>;
  Assign(planID: string, windowID: string, desktopID: string): Promise<State>;
  MoveToDisplay(planID: string, windowID: string, displayID: string): Promise<void>;
  OpenDisplaySettings(): Promise<void>;
  SaveRules(rules: Rules, planID: string): Promise<State>;
  PreviewRule(rule: Rule): Promise<RulePreview>;
  PreviewUndo(): Promise<State>;
  Execute(planID: string): Promise<Execution>;
  ApplyRule(planID: string, ruleID: string): Promise<Execution>;
  ApplyAll(planID: string): Promise<Execution>;
  RestoreGeometry(): Promise<Execution>;
  CaptureLayout(windowIDs: string[], axis: string): Promise<Rule>;
  SetDisplayIndex(displayID: string, index: number): Promise<State>;
  SetTiling(engine: string, gap: number): Promise<State>;
  ImportRules(): Promise<Rules>;
  ExportRules(): Promise<string>;
}

declare global {
  interface Window {
    go?: { gui?: { App?: Bridge } };
    runtime?: {
      EventsOn(name: string, callback: (...data: unknown[]) => void): () => void;
      EventsOff(...names: string[]): void;
    };
  }
}

function bridge(): Bridge {
  const app = window.go?.gui?.App;
  if (!app)
    throw new Error(
      "Lancez deskctl-gui.exe : la liaison Windows n'est pas disponible dans un navigateur ordinaire.",
    );
  return app;
}

export interface GroupEvent { message: string; error: string; groupId: string }
export const api: Bridge & { SubscribeGroups(listener: (event: GroupEvent) => void): () => void } = {
  SubscribeGroups: listener => {
    const runtime = window.runtime;
    if (!runtime?.EventsOn) return () => {};
    const cancel = runtime.EventsOn("groups:changed", (...data) => listener(data[0] as GroupEvent));
    return typeof cancel === "function" ? cancel : () => runtime.EventsOff("groups:changed");
  },
  AssignGroup: (...args) => bridge().AssignGroup(...args),
  PreviewGroup: (...args) => bridge().PreviewGroup(...args),
  SetGroupFollowing: (...args) => bridge().SetGroupFollowing(...args),
  Refresh: () => bridge().Refresh(),
  Assign: (...args) => bridge().Assign(...args),
  MoveToDisplay: (...args) => bridge().MoveToDisplay(...args),
  OpenDisplaySettings: () => bridge().OpenDisplaySettings(),
  SaveRules: (...args) => bridge().SaveRules(...args),
  PreviewRule: (...args) => bridge().PreviewRule(...args),
  PreviewUndo: () => bridge().PreviewUndo(),
  Execute: (...args) => bridge().Execute(...args),
  ApplyRule: (...args) => bridge().ApplyRule(...args),
  ApplyAll: (...args) => bridge().ApplyAll(...args),
  RestoreGeometry: () => bridge().RestoreGeometry(),
  CaptureLayout: (...args) => bridge().CaptureLayout(...args),
  SetDisplayIndex: (...args) => bridge().SetDisplayIndex(...args),
  SetTiling: (...args) => bridge().SetTiling(...args),
  ImportRules: () => bridge().ImportRules(),
  ExportRules: () => bridge().ExportRules(),
};

export const allDesktopsId = "00000000-0000-0000-0000-000000000000";
export const errorText = (error: unknown) => (error instanceof Error ? error.message : String(error));
export const windowTitle = (item: Item) => item.window.title || "(sans titre)";
