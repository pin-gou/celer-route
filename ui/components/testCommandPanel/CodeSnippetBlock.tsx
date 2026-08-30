import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { cn } from "@/lib/utils";
import { Check, Copy } from "lucide-react";

export interface CodeSnippetBlockProps {
	code: string;
	testId?: string;
	copyTestId?: string;
	copyLabel?: string;
	copiedLabel?: string;
	copyText?: string;
	copySuccessMessage?: string;
	toastOnCopy?: boolean;
	showCopiedState?: boolean;
	maxHeight?: string;
}

export function CodeSnippetBlock({
	code,
	testId,
	copyTestId,
	copyLabel = "Copy",
	copiedLabel = "Copied",
	copyText,
	copySuccessMessage,
	toastOnCopy = true,
	showCopiedState = false,
	maxHeight,
}: CodeSnippetBlockProps) {
	const { copy, copied } = useCopyToClipboard({ successMessage: copySuccessMessage, toastOnSuccess: toastOnCopy });

	return (
		<div className="relative" data-testid={testId}>
			<pre
				className={cn("bg-zinc-950 text-zinc-100 overflow-x-auto rounded-md border px-4 py-3 font-mono text-xs whitespace-pre", maxHeight)}
			>
				<code>{code}</code>
			</pre>
			<button
				type="button"
				onClick={() => void copy(code)}
				className="absolute top-2 right-2 flex items-center gap-1 rounded p-1 text-zinc-300 hover:bg-zinc-800 hover:text-white"
				aria-label={copied && showCopiedState ? copiedLabel : copyLabel}
				data-testid={copyTestId}
			>
				{copied && showCopiedState ? <Check className="h-4 w-4" /> : <Copy className="h-4 w-4" />}
				{showCopiedState && <span className="text-[11px]">{copied ? copiedLabel : (copyText ?? copyLabel)}</span>}
			</button>
		</div>
	);
}