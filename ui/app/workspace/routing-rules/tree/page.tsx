/**
 * Routing Tree Page
 * Full-canvas read-only routing rules decision tree visualizer.
 */

import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { RoutingTreeView } from "./views/routingTreeView";

export default function RoutingTreePage() {
	const { t } = useTranslation("routing");

	// Static `metadata` export is not locale-aware; replicate its effect here so the
	// document title and meta description follow the active language.
	useEffect(() => {
		document.title = t("tree.pageTitle");
		const description = t("tree.pageDescription");
		let meta = document.querySelector('meta[name="description"]');
		if (!meta) {
			meta = document.createElement("meta");
			meta.setAttribute("name", "description");
			document.head.appendChild(meta);
		}
		meta.setAttribute("content", description);
	}, [t]);

	return (
		<div className="no-padding-parent no-border-parent h-[calc(100dvh_)] w-full">
			<RoutingTreeView />
		</div>
	);
}