// @vitest-environment jsdom
/**
 * @file Regression tests for LogCleanupDialog's job persistence: a running
 * cleanup job id is tracked outside the dialog (URL param, set via
 * onJobStarted / cleared via onJobSettled) so a page refresh restores the
 * progress panel instead of losing the in-flight job.
 *
 * Covered:
 *  - A persistentJobId + running status reopens the dialog in the running
 *    panel state (not the "configure a new cleanup" form).
 *  - Starting a job reports the id via onJobStarted.
 *  - A terminal status (completed) reports settlement via onJobSettled.
 *  - A stale persisted id that 404s is dropped via onJobSettled and the
 *    dialog falls back to the configure view (instead of polling forever).
 */
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { CleanupJobStatus } from "@/lib/types/logs";

const mocks = vi.hoisted(() => ({
	status: undefined as CleanupJobStatus | undefined,
	statusError: false,
	statusErrorObj: undefined as unknown,
	previewTrigger: vi.fn(),
	startCleanup: vi.fn(),
	cancelCleanup: vi.fn(),
	dispatch: vi.fn(),
	invalidateTags: vi.fn(),
	hasSettingsUpdate: true,
}));

vi.mock("@/lib/store", () => ({
	useLazyPreviewCleanupByFilterQuery: () => [mocks.previewTrigger, { isFetching: false }],
	useStartCleanupMutation: () => [mocks.startCleanup, { isLoading: false }],
	useGetCleanupStatusQuery: () => ({
		data: mocks.status,
		isError: mocks.statusError,
		error: mocks.statusErrorObj,
	}),
	useCancelCleanupMutation: () => [mocks.cancelCleanup, { isLoading: false }],
}));

vi.mock("@/lib/store/apis/baseApi", () => ({
	baseApi: { util: { invalidateTags: mocks.invalidateTags } },
}));

vi.mock("@/lib/store/hooks", () => ({
	useAppDispatch: () => mocks.dispatch,
}));

vi.mock("@/lib/rbac", () => ({
	useRbac: () => mocks.hasSettingsUpdate,
	RbacResource: { Settings: "settings" },
	RbacOperation: { Update: "update" },
}));

vi.mock("react-i18next", () => ({
	useTranslation: () => ({
		t: (key: string) => key,
		i18n: { language: "en", options: { ns: [] }, services: {} },
	}),
}));

vi.mock("sonner", () => ({
	toast: { success: vi.fn(), error: vi.fn(), info: vi.fn() },
}));

import LogCleanupDialog from "./LogCleanupDialog";

const RUNNING_JOB: CleanupJobStatus = {
	id: "job-abc",
	status: "running",
	total: 100,
	processed: 42,
	deleted: 0,
	stripped: 0,
};

describe("LogCleanupDialog job persistence", () => {
	beforeEach(() => {
		mocks.status = undefined;
		mocks.statusError = false;
		mocks.statusErrorObj = undefined;
		mocks.previewTrigger.mockReset();
		mocks.startCleanup.mockReset();
		mocks.cancelCleanup.mockReset();
		mocks.invalidateTags.mockReset();
	});

	it("restores the running panel when opened with a persisted job id (page refresh)", () => {
		mocks.status = RUNNING_JOB;
		render(
			<LogCleanupDialog
				open
				onOpenChange={vi.fn()}
				filters={null}
				persistentJobId="job-abc"
				onJobStarted={vi.fn()}
				onJobSettled={vi.fn()}
			/>,
		);

		// Running panel is showing, configure form is not.
		expect(screen.getByTestId("logs-cleanup-progress")).toBeTruthy();
		expect(screen.queryByTestId("logs-cleanup-start")).toBeNull();
	});

	it("reports the started job id via onJobStarted and switches to the running panel", async () => {
		mocks.previewTrigger.mockReturnValue({
			unwrap: () => Promise.resolve({ scope: "older_than", matched_logs: 5, estimated_size_bytes: 1000 }),
		});
		mocks.startCleanup.mockReturnValue({
			unwrap: () => Promise.resolve({ id: "job-new", status: "pending", total: 0, processed: 0 }),
		});
		const onJobStarted = vi.fn();
		render(<LogCleanupDialog open onOpenChange={vi.fn()} filters={null} onJobStarted={onJobStarted} onJobSettled={vi.fn()} />);

		// Configure flow: preview then start.
		fireEvent.click(screen.getByRole("button", { name: "logging.cleanupPreview" }));
		await waitFor(() => {
			expect((screen.getByTestId("logs-cleanup-start") as HTMLButtonElement).disabled).toBe(false);
		});
		fireEvent.click(screen.getByTestId("logs-cleanup-start"));

		await waitFor(() => {
			expect(onJobStarted).toHaveBeenCalledWith("job-new");
		});
		expect(screen.getByTestId("logs-cleanup-progress")).toBeTruthy();
	});

	it("drops the tracked job via onJobSettled when it completes", async () => {
		mocks.status = { id: "job-abc", status: "completed", total: 100, processed: 100, deleted: 100, stripped: 0 };
		const onJobSettled = vi.fn();
		const onOpenChange = vi.fn();
		render(
			<LogCleanupDialog
				open
				onOpenChange={onOpenChange}
				filters={null}
				persistentJobId="job-abc"
				onJobStarted={vi.fn()}
				onJobSettled={onJobSettled}
			/>,
		);

		await waitFor(() => {
			expect(onJobSettled).toHaveBeenCalled();
		});
		expect(onOpenChange).toHaveBeenCalledWith(false);
	});

	it("drops a stale persisted id that 404s and falls back to the configure view", async () => {
		mocks.statusError = true;
		mocks.statusErrorObj = { status: 404, data: { error: "Job not found" } };
		const onJobSettled = vi.fn();
		render(
			<LogCleanupDialog
				open
				onOpenChange={vi.fn()}
				filters={null}
				persistentJobId="job-stale"
				onJobStarted={vi.fn()}
				onJobSettled={onJobSettled}
			/>,
		);

		await waitFor(() => {
			expect(onJobSettled).toHaveBeenCalled();
		});
		// Back to the configure form, not stuck on the progress panel.
		expect(screen.queryByTestId("logs-cleanup-progress")).toBeNull();
		expect(screen.getByTestId("logs-cleanup-start")).toBeTruthy();
	});
});