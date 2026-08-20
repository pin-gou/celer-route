import { Button } from "@/components/ui/button";
import { CodeEditor } from "@/components/ui/codeEditor";
import { useCopyToClipboard } from "@/hooks/useCopyToClipboard";
import { cn } from "@/lib/utils";
import { ChevronDown, ChevronUp, Copy } from "lucide-react";
import { memo, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

interface LazyJsonBlockProps {
	text: string;
	title?: string;
	preview?: number;
	maxHeight?: number;
	wrap?: boolean;
	mono?: boolean;
	bordered?: boolean;
	className?: string;
}

function LazyJsonBlockInner({
	text,
	title,
	preview = 3,
	maxHeight = 400,
	wrap = true,
	mono = true,
	bordered = true,
	className,
}: LazyJsonBlockProps) {
	const { t } = useTranslation("logs");
	const [mounted, setMounted] = useState(false);
	const { copy } = useCopyToClipboard({ successMessage: t("detailView.copied") });

	const rawLines = useMemo(() => text.split("\n"), [text]);
	const lineOverflow = rawLines.length > preview;
	const moreCount = rawLines.length - preview;
	// Cap collapsed preview so a minified single-line payload (hundreds of KB)
	// can't blow up the DOM before the editor is mounted on demand.
	const previewText = useMemo(() => {
		const head = rawLines.slice(0, preview).join("\n");
		return head.length > 4000 ? `${head.slice(0, 4000)}…` : head;
	}, [rawLines, preview]);
	const charTruncated = previewText.length >= 4000;
	const hasMore = lineOverflow || charTruncated;

	const formattedRef = useRef<string | null>(null);
	const getFormatted = () => {
		if (formattedRef.current == null) {
			try {
				formattedRef.current = JSON.stringify(JSON.parse(text), null, 2);
			} catch {
				formattedRef.current = text;
			}
		}
		return formattedRef.current;
	};

	const handleCopy = () => {
		copy(getFormatted());
	};

	const handleToggle = () => {
		setMounted((prev) => !prev);
	};

	const content = mounted ? (
		<CodeEditor
			className="z-0 w-full"
			shouldAdjustInitialHeight={true}
			maxHeight={maxHeight}
			wrap={wrap}
			code={getFormatted()}
			lang="json"
			readonly={true}
			options={{
				collapsibleBlocks: true,
				showIndentLines: false,
				disableHover: true,
				scrollBeyondLastLine: false,
				lineNumbers: "off",
				alwaysConsumeMouseWheel: false,
			}}
		/>
	) : (
		<>
			<div className={cn("custom-scrollbar max-h-[160px] overflow-y-auto", bordered ? "px-6 py-2" : "px-0 py-0")}>
				{mono ? (
					<pre className="font-mono text-xs break-words whitespace-pre-wrap">
						{previewText}
						{hasMore && "\n..."}
					</pre>
				) : (
					<div className="text-xs break-words whitespace-pre-wrap">
						{previewText}
						{hasMore && "\n..."}
					</div>
				)}
			</div>
			<div className={cn("flex items-center justify-between border-t", bordered ? "px-6 py-2" : "px-2 py-2")}>
				<button
					type="button"
					onClick={handleToggle}
					className="text-primary inline-flex items-center gap-1 text-[11.5px] font-medium hover:underline"
				>
					{mounted ? (
						<>
							<ChevronUp className="h-3 w-3" />
							{t("views.showLess")}
						</>
					) : (
						<>
							<ChevronDown className="h-3 w-3" />
							{t("detailView.viewFormattedJson")}
						</>
					)}
				</button>
				<div className="text-muted-foreground flex items-center gap-2 font-mono text-[10.5px]">
					<span>{t("detailView.lines", { count: rawLines.length })}</span>
					{lineOverflow && !mounted && (
						<button type="button" onClick={handleToggle} className="text-muted-foreground hover:text-foreground underline">
							{t("detailView.showMoreLines", { count: moreCount })}
						</button>
					)}
				</div>
			</div>
		</>
	);

	if (!bordered) {
		return content;
	}

	return (
		<div className={cn("w-full rounded-sm border", className)}>
			{title && (
				<div className="flex items-center justify-between border-b py-2 pl-6">
					<div className="text-sm font-medium">{title}</div>
					<Button
						variant="ghost"
						size="sm"
						className="text-muted-foreground mx-2 h-6 py-1 hover:bg-transparent hover:text-black dark:hover:text-white"
						onClick={handleCopy}
					>
						<Copy className="h-3 w-3" />
					</Button>
				</div>
			)}
			{content}
		</div>
	);
}

export const LazyJsonBlock = memo(LazyJsonBlockInner);