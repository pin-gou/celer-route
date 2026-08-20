// ResizeObserver polyfill for jsdom — Radix UI components (Form, Switch, etc.)
// use @radix-ui/react-use-size which internally calls ResizeObserver.
// jsdom does not provide it, so we define a minimal no-op stub.
if (typeof globalThis.ResizeObserver === "undefined") {
	globalThis.ResizeObserver = class ResizeObserver {
		observe() {}
		unobserve() {}
		disconnect() {}
	};
}