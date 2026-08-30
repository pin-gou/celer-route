import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useState } from "react";

import { CodeSnippetBlock, type CodeSnippetBlockProps } from "./CodeSnippetBlock";

export interface TestCommandTab extends Omit<CodeSnippetBlockProps, "code"> {
	id: string;
	label: string;
	code: string;
}

export interface TestCommandTabsProps {
	tabs: TestCommandTab[];
	defaultTab?: string;
	testIdPrefix?: string;
	tabsListClassName?: string;
}

export function TestCommandTabs({ tabs, defaultTab, testIdPrefix, tabsListClassName }: TestCommandTabsProps) {
	const [tab, setTab] = useState(defaultTab ?? tabs[0]?.id ?? "");

	return (
		<Tabs value={tab} onValueChange={setTab} {...(testIdPrefix ? { "data-testid": `${testIdPrefix}-tabs` } : {})}>
			<TabsList className={tabsListClassName}>
				{tabs.map((tabItem) => (
					<TabsTrigger
						key={tabItem.id}
						value={tabItem.id}
						{...(testIdPrefix ? { "data-testid": `${testIdPrefix}-tab-${tabItem.id}` } : {})}
					>
						{tabItem.label}
					</TabsTrigger>
				))}
			</TabsList>
			{tabs.map((tabItem) => (
				<TabsContent key={tabItem.id} value={tabItem.id}>
					<CodeSnippetBlock
						code={tabItem.code}
						copyLabel={tabItem.copyLabel}
						copiedLabel={tabItem.copiedLabel}
						copyText={tabItem.copyText}
						copySuccessMessage={tabItem.copySuccessMessage}
						toastOnCopy={tabItem.toastOnCopy}
						showCopiedState={tabItem.showCopiedState}
						maxHeight={tabItem.maxHeight}
						testId={testIdPrefix ? `${testIdPrefix}-${tabItem.id}` : tabItem.testId}
						copyTestId={testIdPrefix ? `${testIdPrefix}-copy-${tabItem.id}` : tabItem.copyTestId}
					/>
				</TabsContent>
			))}
		</Tabs>
	);
}