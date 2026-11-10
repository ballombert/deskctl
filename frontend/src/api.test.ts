import { afterEach, expect, test, vi } from "vitest";
import { api } from "./api";

afterEach(() => { delete window.runtime; });
test("abonnement Wails utilise le payload direct et le desabonnement individuel v2", () => {
  const cancel = vi.fn();
  const EventsOn = vi.fn((_name: string, _callback: (...data: unknown[]) => void) => cancel);
  const EventsOff = vi.fn();
  window.runtime = { EventsOn, EventsOff };
  const listener = vi.fn();
  const unsubscribe = api.SubscribeGroups(listener);
  const payload = { message: "Déplacé", error: "", groupId: "work" };
  EventsOn.mock.calls[0][1](payload);
  expect(listener).toHaveBeenCalledWith(payload);
  unsubscribe();
  expect(cancel).toHaveBeenCalledOnce();
  expect(EventsOff).not.toHaveBeenCalled();
});
test("abonnement absent en navigateur ne pretend pas recevoir les evenements", () => {
  expect(() => api.SubscribeGroups(vi.fn())()).not.toThrow();
});
test("runtime ancien sans fonction de nettoyage utilise EventsOff avec le nom Wails", () => {
  const EventsOff = vi.fn();
  Object.defineProperty(window, "runtime", { configurable: true, value: { EventsOn: vi.fn(), EventsOff } });
  api.SubscribeGroups(vi.fn())();
  expect(EventsOff).toHaveBeenCalledExactlyOnceWith("groups:changed");
});
