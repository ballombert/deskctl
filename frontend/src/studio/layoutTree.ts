import type { Item, LayoutNode, Rule } from "../api";

export type Path = number[];

export const newLeaf = (application = "", title = ""): LayoutNode => ({ kind: "window", weight: 0, application, title });
export const newGroup = (axis: "h" | "v", children: LayoutNode[] = [newLeaf(), newLeaf()]): LayoutNode => ({
  kind: "group", axis, weight: 100, children: equalize(children),
});

/** Gives every child the same share, rounded so that the total stays 100. */
export function equalize(children: LayoutNode[]): LayoutNode[] {
  const share = Math.floor(10000 / Math.max(children.length, 1)) / 100;
  return children.map((child, index) => ({
    ...child,
    weight: index === children.length - 1 ? Math.round((100 - share * (children.length - 1)) * 100) / 100 : share,
  }));
}

export function update(root: LayoutNode, path: Path, change: (node: LayoutNode) => LayoutNode): LayoutNode {
  if (!path.length) return change(root);
  const [head, ...rest] = path;
  return { ...root, children: root.children!.map((child, index) => (index === head ? update(child, rest, change) : child)) };
}

export function addChild(root: LayoutNode, path: Path, child: LayoutNode): LayoutNode {
  return update(root, path, group => ({ ...group, children: equalize([...(group.children ?? []), child]) }));
}

export function removeChild(root: LayoutNode, path: Path): LayoutNode {
  const index = path[path.length - 1];
  return update(root, path.slice(0, -1), group => ({
    ...group, children: equalize(group.children!.filter((_, position) => position !== index)),
  }));
}

export function moveChild(root: LayoutNode, path: Path, offset: -1 | 1): LayoutNode {
  const index = path[path.length - 1];
  return update(root, path.slice(0, -1), group => {
    const children = [...group.children!];
    const target = index + offset;
    if (target < 0 || target >= children.length) return group;
    [children[index], children[target]] = [children[target], children[index]];
    return { ...group, children };
  });
}

/** Moves `delta` percent from child index+1 to child index, keeping at least 5 % on each side. */
export function resizePair(root: LayoutNode, groupPath: Path, index: number, delta: number): LayoutNode {
  return update(root, groupPath, group => {
    const children = [...group.children!];
    const total = children[index].weight + children[index + 1].weight;
    const next = Math.round(Math.min(Math.max(children[index].weight + delta, 5), total - 5) * 100) / 100;
    children[index] = { ...children[index], weight: next };
    children[index + 1] = { ...children[index + 1], weight: Math.round((total - next) * 100) / 100 };
    return { ...group, children };
  });
}

const patternCache = new Map<string, RegExp | null>();
function pattern(source: string): RegExp | null {
  if (!patternCache.has(source)) {
    try {
      patternCache.set(source, new RegExp(source, "i"));
    } catch {
      patternCache.set(source, null);
    }
  }
  return patternCache.get(source)!;
}

export const patternError = (source: string | undefined) => (source && pattern(source) === null ? "Expression invalide" : "");
export const escapePattern = (text: string) => `^${text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`;

/** Same semantics as the Go engine: case-insensitive regex, empty means "any". */
export function matchesLeaf(leaf: { application?: string; title?: string }, item: Item): boolean {
  const test = (source: string | undefined, value: string) => !source || (pattern(source)?.test(value) ?? false);
  return test(leaf.application, item.window.application) && test(leaf.title, item.window.title);
}

export function matchesRule(rule: Rule, item: Item): boolean {
  if (rule.groupId) return false;
  if (!rule.layout) return matchesLeaf(rule, item);
  const walk = (node: LayoutNode): boolean => (node.kind === "window" ? matchesLeaf(node, item) : (node.children ?? []).some(walk));
  return walk(rule.layout);
}

const readable = (source: string) => source.replace(/^\^|\$$/g, "").replace(/\\(.)/g, "$1");

export function describe(node: LayoutNode): string {
  if (node.kind === "window") return readable(node.application || node.title || "") || "toute fenêtre";
  const symbol = node.axis === "v" ? "▤" : "▥";
  return `${symbol} ${(node.children ?? []).map(child => `${describe(child)} ${Math.round(child.weight)}%`).join(" | ")}`;
}

export function describeRule(rule: Rule): string {
  if (rule.layout) return describe(rule.layout);
  const parts = [rule.application && `app ~ ${readable(rule.application)}`, rule.title && `titre ~ ${readable(rule.title)}`].filter(Boolean);
  return parts.join(" · ") || "toute fenêtre";
}

export function ruleId(name: string): string {
  const slug = name.normalize("NFD").replace(/[\u0300-\u036f]/g, "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
  return `${slug || "regle"}-${Date.now().toString(36)}`;
}
