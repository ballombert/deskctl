import { expect, test } from "vitest";
import type { Item } from "../api";
import { addChild, describe, equalize, escapePattern, matchesLeaf, newGroup, newLeaf, removeChild, resizePair } from "./layoutTree";

test("les parts restent a 100 % apres ajout, retrait et redimensionnement", () => {
  const total = (weights: number[]) => Math.round(weights.reduce((sum, weight) => sum + weight, 0) * 100) / 100;
  expect(equalize([newLeaf(), newLeaf(), newLeaf()]).map(child => child.weight)).toEqual([33.33, 33.33, 33.34]);
  let root = newGroup("h", [newLeaf("a"), newLeaf("b")]);
  root = addChild(root, [], newGroup("v"));
  expect(total(root.children!.map(child => child.weight))).toBe(100);
  root = resizePair(root, [], 0, 90);
  expect(root.children!.map(child => child.weight)).toEqual([61.66, 5, 33.34]);
  expect(total(root.children!.map(child => child.weight))).toBe(100);
  root = removeChild(root, [1]);
  expect(root.children!.map(child => child.weight)).toEqual([50, 50]);
  expect(describe(root)).toBe("▥ a 50% | ▤ toute fenêtre 50% | toute fenêtre 50% 50%");
});

test("les motifs suivent la semantique du moteur Go", () => {
  const item = { window: { application: "Code.exe", title: "projet — Visual Studio Code" } } as Item;
  expect(matchesLeaf({ application: escapePattern("code.exe") }, item)).toBe(true);
  expect(matchesLeaf({ application: escapePattern("Code") }, item)).toBe(false);
  expect(matchesLeaf({ title: "visual studio" }, item)).toBe(true);
  expect(matchesLeaf({ title: "[" }, item)).toBe(false);
});
