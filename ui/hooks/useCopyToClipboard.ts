import { useCallback, useRef, useState } from "react";
import { toast } from "sonner";

interface UseCopyToClipboardOptions {
	successMessage?: string;
	errorMessage?: string;
	resetDelay?: number;
	toastOnSuccess?: boolean;
}

export function useCopyToClipboard(options: UseCopyToClipboardOptions = {}) {
	const { successMessage = "Copied to clipboard", errorMessage = "Failed to copy", resetDelay = 2000, toastOnSuccess = true } = options;
	const [copied, setCopied] = useState(false);
	const timeoutRef = useRef<ReturnType<typeof setTimeout>>(undefined);

	const copy = useCallback(
		async (text: string) => {
			try {
				await navigator.clipboard.writeText(text);
				setCopied(true);
				if (toastOnSuccess) toast.success(successMessage);

				if (timeoutRef.current) clearTimeout(timeoutRef.current);
				timeoutRef.current = setTimeout(() => setCopied(false), resetDelay);
			} catch {
				toast.error(errorMessage);
			}
		},
		[successMessage, errorMessage, resetDelay, toastOnSuccess],
	);

	return { copy, copied };
}