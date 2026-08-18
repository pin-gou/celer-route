import { Button } from "@/components/ui/button";
import { Pause, Play, Download } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";

interface AudioPlayerProps {
	src: string;
	format?: string; // Optional format: "mp3", "wav", "pcm16", etc.
}

const AudioPlayer = ({ src, format }: AudioPlayerProps) => {
	const { t } = useTranslation("logs");
	const [isPlaying, setIsPlaying] = useState(false);
	const audioRef = useRef<HTMLAudioElement | null>(null);
	if (audioRef.current === null && typeof window !== "undefined") {
		audioRef.current = new Audio();
	}
	const [error, setError] = useState<string | null>(null);
	// Tracks the object URL currently assigned to <audio>.src so we can revoke
	// it on the next play, on play() rejection, and on unmount. Without this
	// guard every new play leaks one Blob URL — and so does any path that
	// interrupts playback before onended fires (pause, source switch, error,
	// unmount).
	const currentUrlRef = useRef<string | null>(null);

	const revokeCurrentUrl = () => {
		const url = currentUrlRef.current;
		if (url) {
			URL.revokeObjectURL(url);
			currentUrlRef.current = null;
		}
	};

	useEffect(() => {
		return () => {
			const audio = audioRef.current;
			if (audio) {
				audio.onended = null;
				audio.pause();
				audio.removeAttribute("src");
				audio.load();
			}
			revokeCurrentUrl();
		};
	}, []);

	// Convert PCM16 to WAV format
	const convertPCM16ToWAV = (pcmData: Uint8Array, sampleRate: number = 24000, numChannels: number = 1): Uint8Array => {
		const bitsPerSample = 16;
		const byteRate = (sampleRate * numChannels * bitsPerSample) / 8;
		const blockAlign = (numChannels * bitsPerSample) / 8;
		const dataSize = pcmData.length;
		const fileSize = 36 + dataSize;

		const wavBuffer = new ArrayBuffer(44 + dataSize);
		const view = new DataView(wavBuffer);

		// RIFF header
		const writeString = (offset: number, string: string) => {
			for (let i = 0; i < string.length; i++) {
				view.setUint8(offset + i, string.charCodeAt(i));
			}
		};

		writeString(0, "RIFF");
		view.setUint32(4, fileSize, true);
		writeString(8, "WAVE");

		// fmt subchunk
		writeString(12, "fmt ");
		view.setUint32(16, 16, true); // Subchunk1Size
		view.setUint16(20, 1, true); // AudioFormat (1 = PCM)
		view.setUint16(22, numChannels, true); // NumChannels
		view.setUint32(24, sampleRate, true); // SampleRate
		view.setUint32(28, byteRate, true); // ByteRate
		view.setUint16(32, blockAlign, true); // BlockAlign
		view.setUint16(34, bitsPerSample, true); // BitsPerSample

		// data subchunk
		writeString(36, "data");
		view.setUint32(40, dataSize, true);

		// Copy PCM data
		const wavArray = new Uint8Array(wavBuffer);
		wavArray.set(pcmData, 44);

		return wavArray;
	};

	const createAudioBlob = (base64Data: string, audioFormat?: string): Blob | null => {
		try {
			const binaryString = atob(base64Data);
			const pcmData = Uint8Array.from(binaryString, (c) => c.charCodeAt(0));

			// Handle PCM16 format - convert to WAV
			if (audioFormat === "pcm16" || audioFormat === "pcm_s16le_16") {
				const wavData = convertPCM16ToWAV(pcmData);
				// Create a new ArrayBuffer to ensure proper type
				const buffer = new ArrayBuffer(wavData.length);
				new Uint8Array(buffer).set(wavData);
				return new Blob([buffer], {
					type: "audio/wav",
				});
			}

			// Handle other formats
			let mimeType = "audio/mpeg"; // Default to MP3
			if (audioFormat === "wav") {
				mimeType = "audio/wav";
			} else if (audioFormat === "ogg") {
				mimeType = "audio/ogg";
			} else if (audioFormat === "webm") {
				mimeType = "audio/webm";
			}

			return new Blob([pcmData], {
				type: mimeType,
			});
		} catch (err) {
			console.error("Failed to decode audio data:", err);
			setError("Failed to decode audio data. The audio file may be corrupted.");
			return null;
		}
	};

	const handlePlayPause = () => {
		const audio = audioRef.current;
		if (!audio || !src) return;

		if (isPlaying) {
			audio.pause();
			setIsPlaying(false);
			return;
		}

		const audioBlob = createAudioBlob(src, format);
		if (!audioBlob) return;

		// Replace any previously-issued blob URL — otherwise the old one leaks
		// when we overwrite audio.src below.
		revokeCurrentUrl();
		const audioUrl = URL.createObjectURL(audioBlob);
		currentUrlRef.current = audioUrl;
		audio.src = audioUrl;
		audio.play().catch((err) => {
			console.error("Failed to play audio:", err);
			setError("Failed to play audio. Please try again.");
			setIsPlaying(false);
			// play() rejection skips onended entirely, so the url would leak
			// without this guard.
			if (audio.src === audioUrl) audio.removeAttribute("src");
			revokeCurrentUrl();
		});
		setIsPlaying(true);

		audio.onended = () => {
			setIsPlaying(false);
			revokeCurrentUrl();
			if (audio.src === audioUrl) audio.removeAttribute("src");
		};
	};

	const handleDownload = () => {
		if (!src) return;

		const audioBlob = createAudioBlob(src, format);
		if (!audioBlob) return;

		const audioUrl = URL.createObjectURL(audioBlob);

		// Determine file extension based on format
		let extension = "mp3";
		if (format === "pcm16" || format === "pcm_s16le_16") {
			extension = "wav";
		} else if (format === "wav") {
			extension = "wav";
		} else if (format === "ogg") {
			extension = "ogg";
		} else if (format === "webm") {
			extension = "webm";
		}

		const a = document.createElement("a");
		a.href = audioUrl;
		a.download = `speech-output.${extension}`;
		document.body.appendChild(a);
		a.click();
		document.body.removeChild(a);
		URL.revokeObjectURL(audioUrl);
	};

	return (
		<div className="flex flex-col gap-2">
			<div className="flex items-center gap-2">
				<Button onClick={handlePlayPause} variant="outline" size="sm" className="flex items-center gap-2" disabled={!!error}>
					{isPlaying ? <Pause className="h-4 w-4" /> : <Play className="h-4 w-4" />}
					{isPlaying ? t("views.pause") : t("views.play")}
				</Button>

				<Button onClick={handleDownload} variant="outline" size="sm" className="flex items-center gap-2" disabled={!!error}>
					<Download className="h-4 w-4" />
					{t("views.download")}
				</Button>
			</div>
			{error && <div className="text-sm text-red-500">{error}</div>}
		</div>
	);
};

export default AudioPlayer;