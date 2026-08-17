import React from "react";
import { cn } from "@/lib/utils";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./select";

export interface TimeValue {
	hour: number;
	minute: number;
}

interface TimePickerProps {
	value?: TimeValue;
	onChange?: (value: TimeValue) => void;
	className?: string;
	"aria-label"?: string;
}

export const TimePicker = React.forwardRef<HTMLDivElement, TimePickerProps>((props, forwardedRef) => {
	const { value, onChange, className } = props;

	const hour = value?.hour ?? 0;
	const minute = value?.minute ?? 0;

	const handleHourChange = (newHourStr: string) => {
		if (!onChange) return;
		onChange({ hour: Number(newHourStr), minute });
	};

	const handleMinuteChange = (newMinuteStr: string) => {
		if (!onChange) return;
		onChange({ hour, minute: Number(newMinuteStr) });
	};

	return (
		<div ref={forwardedRef} className={cn("inline-flex h-9 w-full items-center gap-1", className)}>
			<Select value={hour.toString()} onValueChange={handleHourChange}>
				<SelectTrigger size="sm" className="h-9 w-[70px]">
					<SelectValue placeholder="HH" />
				</SelectTrigger>
				<SelectContent>
					{Array.from({ length: 24 }, (_, i) => i).map((h) => (
						<SelectItem key={h} value={h.toString()}>
							{h.toString().padStart(2, "0")}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
			<span className="text-muted-foreground">:</span>
			<Select value={minute.toString()} onValueChange={handleMinuteChange}>
				<SelectTrigger size="sm" className="h-9 w-[70px]">
					<SelectValue placeholder="MM" />
				</SelectTrigger>
				<SelectContent>
					{Array.from({ length: 60 }, (_, i) => i).map((m) => (
						<SelectItem key={m} value={m.toString()}>
							{m.toString().padStart(2, "0")}
						</SelectItem>
					))}
				</SelectContent>
			</Select>
		</div>
	);
});

TimePicker.displayName = "TimePicker";