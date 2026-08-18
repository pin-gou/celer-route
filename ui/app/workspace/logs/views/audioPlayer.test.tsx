// @vitest-environment jsdom
/**
 * @file AudioPlayer — leak-prevention tests
 *
 * The original implementation had three Blob URL leak paths:
 *   1. every new play() leaked the previous URL when overwriting audio.src
 *   2. play() rejection left the URL unrevoked (onended never fires)
 *   3. component unmount left the URL unrevoked and <audio> still bound to it
 *
 * These tests pin each of those paths.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { act, render, fireEvent } from "@testing-library/react";

import AudioPlayer from "./audioPlayer";

class FakeAudio extends EventTarget {
	src = "";
	paused = true;
	play = vi.fn().mockResolvedValue(undefined);
	pause = vi.fn(() => {
		this.paused = true;
	});
	load = vi.fn();
	removeAttribute = vi.fn((name: string) => {
		if (name === "src") this.src = "";
	});
	onended: (() => void) | null = null;
}

let latestAudio: FakeAudio | null = null;
const originalAudio = (globalThis as any).Audio;
const originalCreateObjectURL = URL.createObjectURL;
const originalRevokeObjectURL = URL.revokeObjectURL;
let createCalls = 0;
let revokeCalls = 0;
let lastCreatedURL = "";

class FakeAudioFactory {
	constructor() {
		const a = new FakeAudio();
		latestAudio = a;
		return a as unknown as FakeAudioFactory;
	}
}

beforeEach(() => {
	latestAudio = null;
	createCalls = 0;
	revokeCalls = 0;
	lastCreatedURL = "";
	(globalThis as any).Audio = FakeAudioFactory;
	URL.createObjectURL = vi.fn((blob: Blob) => {
		createCalls += 1;
		lastCreatedURL = `blob:fake-${createCalls}`;
		return lastCreatedURL;
	});
	URL.revokeObjectURL = vi.fn(() => {
		revokeCalls += 1;
	});
});

afterEach(() => {
	(globalThis as any).Audio = originalAudio;
	URL.createObjectURL = originalCreateObjectURL;
	URL.revokeObjectURL = originalRevokeObjectURL;
});

function renderPlayer() {
	return render(<AudioPlayer src="AQID" format="wav" />);
}

describe("AudioPlayer — Blob URL leak prevention", () => {
	it("revokes the previous blob URL when play is invoked a second time", () => {
		const { getByText } = renderPlayer();
		const playBtn = getByText("views.play");

		act(() => {
			fireEvent.click(playBtn);
		});
		expect(createCalls).toBe(1);
		expect(latestAudio).not.toBeNull();
		// Simulate natural end of first clip
		act(() => {
			latestAudio!.onended?.();
		});

		act(() => {
			fireEvent.click(playBtn);
		});
		// Second click issued a new URL and revoked the previous one.
		expect(createCalls).toBe(2);
		expect(revokeCalls).toBeGreaterThanOrEqual(1);
	});

	it("revokes the blob URL when play() rejects (onended never fires)", async () => {
		const { getByText } = renderPlayer();
		const playBtn = getByText("views.play");
		// Override the play() that the render-time constructor installed with a
		// rejecting one — simulating a browser block on autoplay or codec error.
		latestAudio!.play = vi.fn().mockRejectedValue(new Error("blocked"));

		await act(async () => {
			fireEvent.click(playBtn);
		});
		// Flush microtasks so the .catch handler in handlePlayPause runs.
		await act(async () => {
			await Promise.resolve();
			await Promise.resolve();
		});

		expect(createCalls).toBe(1);
		expect(revokeCalls).toBe(1);
	});

	it("revokes the blob URL and tears down the audio element on unmount", () => {
		const { getByText, unmount } = renderPlayer();
		const playBtn = getByText("views.play");

		act(() => {
			fireEvent.click(playBtn);
		});
		expect(createCalls).toBe(1);
		const audio = latestAudio!;

		unmount();

		expect(audio.pause).toHaveBeenCalled();
		expect(audio.removeAttribute).toHaveBeenCalledWith("src");
		expect(audio.load).toHaveBeenCalled();
		expect(audio.onended).toBeNull();
		expect(revokeCalls).toBe(1);
	});
});