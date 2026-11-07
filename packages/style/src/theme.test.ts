import { afterEach, describe, expect, it } from "vitest";
import { applyTheme, cookieDomain, documentTheme, storedTheme } from "./theme";

describe("cookieDomain", () => {
  it("shares the cookie across the last two labels", () => {
    expect(cookieDomain("uwbiotron.ca")).toBe("; Domain=uwbiotron.ca");
    expect(cookieDomain("calendar.uwbiotron.ca")).toBe("; Domain=uwbiotron.ca");
    expect(cookieDomain("a.b.uwbiotron.dev")).toBe("; Domain=uwbiotron.dev");
  });

  it("keeps the cookie host-only on localhost and other dotless names", () => {
    expect(cookieDomain("localhost")).toBe("");
    expect(cookieDomain("calendar-web")).toBe("");
  });

  it("keeps the cookie host-only on IP addresses", () => {
    expect(cookieDomain("127.0.0.1")).toBe("");
    expect(cookieDomain("192.168.1.20")).toBe("");
    expect(cookieDomain("[::1]")).toBe("");
    expect(cookieDomain("[2001:db8::1]")).toBe("");
  });
});

describe("applyTheme", () => {
  afterEach(() => {
    document.cookie = "biotron-theme=; Path=/; Max-Age=0";
    document.documentElement.className = "";
    document.documentElement.removeAttribute("style");
  });

  it("paints the page and stores the choice in the cookie", () => {
    applyTheme("dark");
    const root = document.documentElement;
    expect(root.classList.contains("dark")).toBe(true);
    expect(root.style.colorScheme).toBe("dark");
    expect(root.style.backgroundColor).toBe("rgb(7, 11, 14)");
    expect(storedTheme()).toBe("dark");
    expect(documentTheme()).toBe("dark");

    applyTheme("light");
    expect(root.classList.contains("dark")).toBe(false);
    expect(storedTheme()).toBe("light");
  });

  it("can paint without storing", () => {
    applyTheme("dark", false);
    expect(storedTheme()).toBeNull();
    expect(documentTheme()).toBe("dark");
  });
});
