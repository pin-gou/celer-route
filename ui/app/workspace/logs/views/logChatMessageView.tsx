import { Button } from "@/components/ui/button";
import { ChatMessage, ContentBlock } from "@/lib/types/logs";
import { cn } from "@/lib/utils";
import { isJson } from "@/lib/utils/validation";
import { Download } from "lucide-react";
import { format } from "date-fns";
import { memo } from "react";
import AudioPlayer from "./audioPlayer";
import CollapsibleBox from "./collapsibleBox";
import { LazyJsonBlock } from "./lazyJsonBlock";

interface LogChatMessageViewProps {
	message: ChatMessage;
	audioFormat?: string; // Optional audio format from request params
}

function isSafeHttpUrl(value: string) {
	try {
		const protocol = new URL(value).protocol;
		return protocol === "https:" || protocol === "http:";
	} catch {
		return false;
	}
}

function formatFileDataSize(fileData?: string) {
	if (!fileData) return undefined;
	const padding = fileData.endsWith("==") ? 2 : fileData.endsWith("=") ? 1 : 0;
	const bytes = Math.max(0, Math.floor((fileData.length * 3) / 4) - padding);
	if (bytes < 1024) return `${bytes} B`;
	if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
	return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

function downloadFileData(fileData: string, filename: string, fileType?: string) {
	try {
		const binary = atob(fileData);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i += 1) {
			bytes[i] = binary.charCodeAt(i);
		}
		const blob = new Blob([bytes], { type: fileType || "application/octet-stream" });
		const url = URL.createObjectURL(blob);
		const anchor = document.createElement("a");
		anchor.href = url;
		anchor.download = filename;
		document.body.appendChild(anchor);
		anchor.click();
		document.body.removeChild(anchor);
		setTimeout(() => URL.revokeObjectURL(url), 0);
	} catch {
		console.error("Failed to decode file data for download");
	}
}

export function LogChatFileBlockView({ block, className }: { block: ContentBlock; className?: string }) {
	const file = block.file;
	if (!file) return null;

	const title = file.filename || file.file_id || "Attached file";
	const size = formatFileDataSize(file.file_data);
	const details = [file.file_type, size, file.file_id ? `ID: ${file.file_id}` : undefined].filter(Boolean);
	const canDownload = !!file.file_data;

	return (
		<div className={cn("bg-muted/30 rounded border p-3 text-xs", className)}>
			<div className="flex items-start justify-between gap-3">
				<div className="min-w-0 font-medium break-all">{title}</div>
				{canDownload && (
					<Button
						type="button"
						variant="outline"
						size="sm"
						className="h-7 shrink-0 gap-1 px-2 text-xs"
						onClick={() => downloadFileData(file.file_data!, title, file.file_type)}
						data-testid="file-block-download-btn"
					>
						<Download className="h-3.5 w-3.5" />
						Download
					</Button>
				)}
			</div>
			{details.length > 0 && <div className="text-muted-foreground mt-1 break-all">{details.join(" · ")}</div>}
			{file.file_url && isSafeHttpUrl(file.file_url) && (
				<a
					href={file.file_url}
					target="_blank"
					rel="noreferrer"
					className="text-primary mt-2 inline-block hover:underline"
					data-testid="file-block-open-link"
				>
					Open file
				</a>
			)}
		</div>
	);
}

function ContentBlockView({ block }: { block: ContentBlock; index: number }) {
	const blockType = block.type.replaceAll("_", " ");

	// Handle text content
	if (block.text) {
		if (isJson(block.text)) {
			return <LazyJsonBlock title={blockType} text={block.text} />;
		}
		return (
			<CollapsibleBox title={blockType} onCopy={() => block.text || ""} collapsedHeight={100}>
				<div className="custom-scrollbar max-h-[400px] overflow-y-auto px-6 py-2 font-mono text-xs break-words whitespace-pre-wrap">
					{block.text}
				</div>
			</CollapsibleBox>
		);
	}

	// Handle image content
	if (block.image_url) {
		const src = block.image_url.url;
		if (src) {
			return <img src={src} alt="Attached image" className="max-w-full rounded border" />;
		}
	}

	// Handle file content
	if (block.file) {
		return (
			<CollapsibleBox title={blockType} onCopy={() => JSON.stringify(block.file, null, 2)} collapsedHeight={100}>
				<LogChatFileBlockView block={block} />
			</CollapsibleBox>
		);
	}

	// Handle audio content
	if (block.input_audio) {
		return <LazyJsonBlock title={blockType} text={JSON.stringify(block.input_audio, null, 2)} />;
	}

	return null;
}

const LogChatMessageView = memo(function LogChatMessageView({ message, audioFormat }: LogChatMessageViewProps) {
	return (
		<div className="flex w-full flex-col gap-2">
			{/* Role header */}
			<div className="flex items-center gap-2">
				<span className="text-sm font-medium capitalize">{message.role}</span>
				{message.tool_call_id && <span className="text-muted-foreground text-xs">Tool Call ID: {message.tool_call_id}</span>}
			</div>

			{/* Handle reasoning content */}
			{message.reasoning && (
				<>
					{isJson(message.reasoning) ? (
						<LazyJsonBlock title="Reasoning" text={message.reasoning} mono={false} />
					) : (
						<CollapsibleBox title="Reasoning" onCopy={() => message.reasoning || ""} collapsedHeight={100}>
							<div className="custom-scrollbar text-muted-foreground max-h-[400px] overflow-y-auto px-6 py-2 font-mono text-xs break-words whitespace-pre-wrap italic">
								{message.reasoning}
							</div>
						</CollapsibleBox>
					)}
				</>
			)}

			{/* Handle refusal content */}
			{message.refusal && (
				<>
					{isJson(message.refusal) ? (
						<LazyJsonBlock title="Refusal" text={message.refusal} mono={false} />
					) : (
						<CollapsibleBox title="Refusal" onCopy={() => message.refusal || ""} collapsedHeight={100}>
							<div className="custom-scrollbar max-h-[400px] overflow-y-auto px-6 py-2 font-mono text-xs break-words whitespace-pre-wrap text-red-800">
								{message.refusal}
							</div>
						</CollapsibleBox>
					)}
				</>
			)}

			{/* Handle content */}
			{message.content && (
				<>
					{typeof message.content === "string" ? (
						<>
							{isJson(message.content) ? (
								<LazyJsonBlock title="Content" text={message.content as string} mono={false} />
							) : (
								<CollapsibleBox title="Content" onCopy={() => (message.content as string) || ""} collapsedHeight={100}>
									<div className="custom-scrollbar max-h-[400px] overflow-y-auto px-6 py-2 font-mono text-xs break-words whitespace-pre-wrap">
										{message.content}
									</div>
								</CollapsibleBox>
							)}
						</>
					) : (
						Array.isArray(message.content) &&
						message.content.map((block, blockIndex) => <ContentBlockView key={blockIndex} block={block} index={blockIndex} />)
					)}
				</>
			)}

			{/* Handle tool calls */}
			{message.tool_calls && message.tool_calls.length > 0 && (
				<>
					{message.tool_calls.map((toolCall, index) => (
						<LazyJsonBlock
							key={index}
							title={`Tool Call: ${toolCall.function?.name || `#${index + 1}`}`}
							text={JSON.stringify(toolCall, null, 2)}
						/>
					))}
				</>
			)}

			{/* Handle annotations */}
			{message.annotations && message.annotations.length > 0 && (
				<LazyJsonBlock title="Annotations" text={JSON.stringify(message.annotations, null, 2)} />
			)}

			{/* Handle audio output */}
			{message.audio && (
				<CollapsibleBox title="Audio Output" collapsedHeight={150}>
					<div className="space-y-4 px-6 py-4">
						{message.audio.transcript && (
							<div className="space-y-2">
								<div className="text-muted-foreground text-xs font-medium">Transcript:</div>
								<div className="font-mono text-xs break-words whitespace-pre-wrap">{message.audio.transcript}</div>
							</div>
						)}
						{message.audio.data && (
							<div className="space-y-2">
								<div className="text-muted-foreground text-xs font-medium">Audio:</div>
								<AudioPlayer src={message.audio.data} format={audioFormat} />
							</div>
						)}
						{message.audio.id && (
							<div className="text-muted-foreground text-xs">
								ID: {message.audio.id} | Expires:{" "}
								{message.audio.expires_at && Number.isFinite(message.audio.expires_at)
									? format(new Date(message.audio.expires_at * 1000), "yyyy-MM-dd HH:mm:ss")
									: "N/A"}
							</div>
						)}
					</div>
				</CollapsibleBox>
			)}
		</div>
	);
});

export default LogChatMessageView;