// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import { NoPermissionView } from "@/components/noPermissionView";

// Mock react-i18next useTranslation
// After the dev phase, NoPermissionView will call useTranslation() to support
// the entityI18nKey prop. In the red phase, the entityI18nKey prop does not
// exist yet, so the test will fail at compile/type-check time.
vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => {
			// Return a predictable translated string for known keys
			if (key === "mcp:registry.permissionDenied") {
				return "MCP 注册中心";
			}
			if (key === "logs:page.permissionDenied") {
				return "日志";
			}
			// Fallback: return the key itself (react-i18next default behavior)
			return key;
		},
		i18n: {
			language: "zh-CN",
			options: { ns: [] },
			services: {},
		},
	}),
	Trans: ({ children }: { children: React.ReactNode }) => children,
}));

describe("NoPermissionView", () => {
	it("should render the entity string when only entity prop is provided (backward compatibility)", () => {
		// This test verifies backward-compatible behavior: when entityI18nKey is
		// not provided, the component should render the entity string directly.
		// In the red phase, NoPermissionViewProps does not yet have entityI18nKey,
		// so this test will fail at compile time.
		render(<NoPermissionView entity="mcp registry" />);

		expect(screen.getByText(/mcp registry/i), "Should display the entity string when entityI18nKey is not provided").toBeTruthy();
	});

	it("should display translated text when entityI18nKey is provided", () => {
		// After the dev phase, NoPermissionView will accept entityI18nKey and
		// use t(entityI18nKey) to render the translated entity name.
		// In the red phase, this test will fail because entityI18nKey is not a
		// valid prop yet.
		render(<NoPermissionView entity="mcp registry" entityI18nKey="mcp:registry.permissionDenied" />);

		expect(screen.getByText(/MCP 注册中心/i), "Should display the translated text from entityI18nKey").toBeTruthy();
	});

	it("should fall back to entity string when entityI18nKey is provided but translation returns the key itself", () => {
		// When the translation key is not found, react-i18next returns the key
		// itself as the fallback. The component should render this fallback text
		// rather than crashing.
		// In the red phase, this test will fail because entityI18nKey is not a
		// valid prop yet.
		render(<NoPermissionView entity="unknown module" entityI18nKey="nonexistent:route.key" />);

		// The t() mock returns the key itself for unknown keys
		expect(
			screen.getByText(/nonexistent:route.key/i),
			"Should fall back to showing the key text when translation is not found",
		).toBeTruthy();
	});
});