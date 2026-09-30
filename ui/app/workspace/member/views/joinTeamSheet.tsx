import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import { Textarea } from "@/components/ui/textarea";
import { getErrorMessage, useSubmitKeyRequestMutation } from "@/lib/store";
import { Loader2, UserPlus } from "lucide-react";
import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

// Join-team sheet — opened from /workspace/member when a member wants
// to request access to a team they don't belong to. The backend handler
// at /api/member/key-requests enforces the membership state and rejects
// the request if the caller is already an active member (US3.2: avoid
// duplicate join requests).
export interface JoinTeamSheetProps {
	open: boolean;
	onOpenChange: (open: boolean) => void;
	defaultTeamId?: string;
	onSubmitted?: () => void;
}

export function JoinTeamSheet({ open, onOpenChange, defaultTeamId = "", onSubmitted }: JoinTeamSheetProps) {
	const { t } = useTranslation("governance-ui");
	const [teamId, setTeamId] = useState(defaultTeamId);
	const [purpose, setPurpose] = useState("");
	const [submit, { isLoading }] = useSubmitKeyRequestMutation();

	useEffect(() => {
		if (open) {
			setTeamId(defaultTeamId);
			setPurpose("");
		}
	}, [open, defaultTeamId]);

	const onSubmit = async () => {
		const trimmedTeam = teamId.trim();
		const trimmedPurpose = purpose.trim();
		if (!trimmedTeam || !trimmedPurpose) {
			toast.error(t("users.keyRequests.validation"));
			return;
		}
		try {
			await submit({
				kind: "join_team",
				team_id: trimmedTeam,
				purpose: trimmedPurpose,
			}).unwrap();
			toast.success(t("users.keyRequests.submitted"));
			onOpenChange(false);
			onSubmitted?.();
		} catch (e) {
			toast.error(getErrorMessage(e));
		}
	};

	return (
		<Sheet open={open} onOpenChange={onOpenChange}>
			<SheetContent data-testid="member-join-team-sheet">
				<SheetHeader>
					<SheetTitle className="flex items-center gap-2">
						<UserPlus className="h-4 w-4" />
						{t("users.keyRequests.joinTitle")}
					</SheetTitle>
					<SheetDescription>{t("users.keyRequests.joinDesc")}</SheetDescription>
				</SheetHeader>

				<div className="mt-6 space-y-4">
					<div className="space-y-2">
						<label htmlFor="join-team-id" className="text-sm font-medium">
							{t("users.keyRequests.teamId")}
						</label>
						<Input
							id="join-team-id"
							value={teamId}
							onChange={(e) => setTeamId(e.target.value)}
							placeholder={t("users.keyRequests.teamIdPlaceholder")}
							data-testid="join-team-id-input"
						/>
					</div>

					<div className="space-y-2">
						<label htmlFor="join-purpose" className="text-sm font-medium">
							{t("users.keyRequests.purpose")}
						</label>
						<Textarea
							id="join-purpose"
							value={purpose}
							onChange={(e) => setPurpose(e.target.value)}
							placeholder={t("users.keyRequests.purposePlaceholder")}
							rows={4}
							data-testid="join-purpose-input"
						/>
						<p className="text-muted-foreground text-xs">{t("users.keyRequests.purposeHint")}</p>
					</div>
				</div>

				<SheetFooter>
					<Button variant="outline" onClick={() => onOpenChange(false)} data-testid="join-team-cancel">
						{t("users.keyRequests.cancel")}
					</Button>
					<Button onClick={onSubmit} isLoading={isLoading} data-testid="join-team-submit">
						{isLoading && <Loader2 className="mr-1 h-3 w-3 animate-spin" />}
						{t("users.keyRequests.submit")}
					</Button>
				</SheetFooter>
			</SheetContent>
		</Sheet>
	);
}