// @vitest-environment jsdom
import { describe, it, expect, beforeEach } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useLocale } from "@/lib/i18n/useLocale";

const STORAGE_KEY = "bifrost.locale";

describe("useLocale", () => {
  beforeEach(() => {
    // Clear localStorage before each test
    localStorage.clear();
  });

  it("should initialize with 'en' when no localStorage value is set", () => {
    // Detection chain: localStorage > navigator.language > en.
    // In the test environment navigator.language is "en-US", which
    // normalizes to "en" — the fallback chain's terminal state.
    const { result } = renderHook(() => useLocale());
    expect(result.current.locale).toBe("en");
  });

  it("should initialize with the value from localStorage if present", () => {
    localStorage.setItem(STORAGE_KEY, "zh-CN");
    const { result } = renderHook(() => useLocale());
    expect(result.current.locale).toBe("zh-CN");
  });

  it("should provide availableLocales containing exactly en and zh-CN", () => {
    const { result } = renderHook(() => useLocale());
    expect(result.current.availableLocales).toContain("en");
    expect(result.current.availableLocales).toContain("zh-CN");
    expect(result.current.availableLocales.length).toBe(2);
  });

  it("should persist locale to localStorage and update locale when setLocale is called", () => {
    const { result } = renderHook(() => useLocale());

    act(() => {
      result.current.setLocale("zh-CN");
    });

    expect(localStorage.getItem(STORAGE_KEY)).toBe("zh-CN");
    expect(result.current.locale).toBe("zh-CN");
  });

  it("should switch back to 'en' when setLocale is called with 'en'", () => {
    const { result } = renderHook(() => useLocale());

    act(() => {
      result.current.setLocale("zh-CN");
    });
    expect(result.current.locale).toBe("zh-CN");

    act(() => {
      result.current.setLocale("en");
    });
    expect(localStorage.getItem(STORAGE_KEY)).toBe("en");
    expect(result.current.locale).toBe("en");
  });

  it("should fall back to 'en' when localStorage contains a corrupted value", () => {
    // Corrupted / invalid locale value — must not throw, must fall back to en.
    localStorage.setItem(STORAGE_KEY, "zz-invalid");

    expect(() => {
      const { result } = renderHook(() => useLocale());
      expect(result.current.locale).toBe("en");
    }).not.toThrow();
  });

  it("should fall back to 'en' when localStorage contains a zh-CN-invalid corrupted value", () => {
    // "zh-CN-invalid" is a corrupted value that starts with "zh" but is not a
    // valid locale. Must fall back to en, not match zh-CN via prefix.
    localStorage.setItem(STORAGE_KEY, "zh-CN-invalid");

    const { result } = renderHook(() => useLocale());
    expect(result.current.locale).toBe("en");
  });
});