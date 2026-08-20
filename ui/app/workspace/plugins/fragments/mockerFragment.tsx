import { Button } from "@/components/ui/button";
import { CodeEditor } from "@/components/ui/codeEditor";
import { useUpdatePluginMutation } from "@/lib/store/apis/pluginsApi";
import { RbacOperation, RbacResource, useRbac } from "@/lib/rbac";
import { MOCKER_PLUGIN, mockerConfigSchema, pluginFragmentLabels, type MockerConfig, type Plugin } from "@/lib/types/plugins";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

function validate(text: string): { ok: boolean; data?: MockerConfig } {
	try {
		const data = JSON.parse(text);
		const result = mockerConfigSchema.safeParse(data);
		if (!result.success) {
			return { ok: false };
		}
		return { ok: true, data: result.data };
	} catch {
		return { ok: false };
	}
}

export function ConfigForm({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	const hasUpdateAccess = useRbac(RbacResource.Plugins, RbacOperation.Update);
	const [updatePlugin, { isLoading }] = useUpdatePluginMutation();

	const [jsonText, setJsonText] = useState<string>(() => JSON.stringify(plugin.config ?? {}, null, 2));
	const [parsingError, setParsingError] = useState<string | null>(null);
	const [validationError, setValidationError] = useState<string | null>(null);
	const [isDirty, setIsDirty] = useState(false);
	const submitRef = useRef<() => void>(() => {});

	const handleChange = (value: string) => {
		setJsonText(value);
		setIsDirty(true);

		try {
			const data = JSON.parse(value);
			const result = mockerConfigSchema.safeParse(data);
			if (!result.success) {
				setValidationError(result.error.issues[0]?.message ?? t("mockerConfig.invalidJsonError"));
				setParsingError(null);
			} else {
				setValidationError(null);
				setParsingError(null);
			}
		} catch {
			setParsingError(t("mockerConfig.invalidJsonError"));
			setValidationError(null);
		}
	};

	submitRef.current = async () => {
		if (!hasUpdateAccess) return;
		const { ok, data } = validate(jsonText);
		if (!ok) {
			toast.error(t("mockerConfig.invalidJsonError"));
			return;
		}
		try {
			await updatePlugin({
				name: MOCKER_PLUGIN,
				data: {
					enabled: plugin.enabled,
					config: data,
				},
			}).unwrap();
			toast.success(t("mockerConfig.savedToast"));
			setIsDirty(false);
		} catch {
			toast.error(t("mockerConfig.saveFailedToast"));
		}
	};

	return (
		<div className="space-y-4">
			<div className="rounded-sm border">
				<CodeEditor
					className="z-0 w-full"
					minHeight={300}
					maxHeight={600}
					wrap={true}
					code={jsonText}
					lang="json"
					onChange={handleChange}
					options={{
						scrollBeyondLastLine: false,
						collapsibleBlocks: true,
						lineNumbers: "on",
						alwaysConsumeMouseWheel: false,
					}}
				/>
			</div>

			{parsingError && (
				<p className="text-destructive text-sm" data-testid="mocker-json-error">
					{parsingError}
				</p>
			)}
			{validationError && (
				<p className="text-destructive text-sm" data-testid="mocker-json-error">
					{validationError}
				</p>
			)}

			<p className="text-muted-foreground text-xs">{t("mockerConfig.editorDescription")}</p>

			<div className="flex justify-end">
				<Button
					type="button"
					data-testid="mocker-save-button"
					onClick={() => void submitRef.current()}
					disabled={isLoading || !isDirty || !hasUpdateAccess}
				>
					{isLoading ? t("mockerConfig.saving") : t("mockerConfig.saveConfiguration")}
				</Button>
			</div>
		</div>
	);
}

export function MockerFragment({ plugin }: { plugin: Plugin }) {
	const { t } = useTranslation("plugins");
	return (
		<div data-testid="mocker-fragment" className="space-y-8">
			<h3 className="text-lg font-semibold">{t(pluginFragmentLabels.mocker)}</h3>
			<div className="rounded-lg border p-4">
				<h4 className="mb-4 text-sm font-medium">{t("mockerConfig.settingsTitle")}</h4>
				<ConfigForm plugin={plugin} />
			</div>
		</div>
	);
}

export default MockerFragment;