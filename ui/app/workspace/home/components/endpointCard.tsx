import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Copy } from "lucide-react";
import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "sonner";

interface Props {
	endpointUrl?: string;
}

export default function EndpointCard({ endpointUrl }: Props) {
	const { t } = useTranslation("common");
	const url = endpointUrl ?? defaultEndpoint();
	const [tab, setTab] = useState("curl");
	const examples = useMemo(() => buildExamples(url), [url]);

	const handleCopy = async (text: string) => {
		try {
			await navigator.clipboard.writeText(text);
			toast.success(t("home.endpointCard.copied"));
		} catch {
			toast.error("Copy failed");
		}
	};

	return (
		<Card className="bg-card gap-0 border py-0 shadow-sm" data-testid="home-endpoint-card">
			<CardHeader className="flex flex-row items-start justify-between gap-2 border-b px-6 py-4">
				<div className="space-y-1">
					<CardTitle className="text-base font-semibold">{t("home.endpointCard.title")}</CardTitle>
					<p className="text-muted-foreground text-xs">{t("home.endpointCard.subtitle")}</p>
				</div>
			</CardHeader>
			<CardContent className="space-y-3 px-6 py-4">
				<div className="bg-muted/40 flex items-center gap-2 rounded-md border p-3 font-mono text-sm">
					<code className="flex-1 truncate" data-testid="home-endpoint-url">
						{url}
					</code>
					<Button size="sm" variant="outline" onClick={() => void handleCopy(url)}>
						<Copy className="mr-1 h-4 w-4" />
						{t("home.endpointCard.copyLabel")}
					</Button>
				</div>

				<Tabs value={tab} onValueChange={setTab}>
					<TabsList>
						<TabsTrigger value="curl">{t("home.endpointCard.codeTabs.curl")}</TabsTrigger>
						<TabsTrigger value="python">{t("home.endpointCard.codeTabs.python")}</TabsTrigger>
						<TabsTrigger value="node">{t("home.endpointCard.codeTabs.node")}</TabsTrigger>
						<TabsTrigger value="go">{t("home.endpointCard.codeTabs.go")}</TabsTrigger>
					</TabsList>
					<TabsContent value="curl">
						<CodeBlock code={examples.curl} onCopy={() => void handleCopy(examples.curl)} />
					</TabsContent>
					<TabsContent value="python">
						<CodeBlock code={examples.python} onCopy={() => void handleCopy(examples.python)} />
					</TabsContent>
					<TabsContent value="node">
						<CodeBlock code={examples.node} onCopy={() => void handleCopy(examples.node)} />
					</TabsContent>
					<TabsContent value="go">
						<CodeBlock code={examples.go} onCopy={() => void handleCopy(examples.go)} />
					</TabsContent>
				</Tabs>
			</CardContent>
		</Card>
	);
}

function CodeBlock({ code, onCopy }: { code: string; onCopy: () => void }) {
	return (
		<div className="relative">
			<pre className="max-h-72 overflow-auto rounded-md border bg-zinc-950 px-4 py-3 text-xs text-zinc-100">
				<code>{code}</code>
			</pre>
			<button
				type="button"
				onClick={onCopy}
				className="absolute top-2 right-2 rounded p-1 text-zinc-300 hover:bg-zinc-800 hover:text-white"
				aria-label="Copy"
			>
				<Copy className="h-4 w-4" />
			</button>
		</div>
	);
}

function defaultEndpoint(): string {
	if (typeof window === "undefined") return "http://localhost:8080/v1";
	return `${window.location.origin}/v1`;
}

// Exported so other modules (e.g. onboarding Done step) can compose the same URL
// without duplicating the derivation logic.
export function resolveEndpointUrl(): string {
	return defaultEndpoint();
}

// Side effect-free examples builder (also reused by the onboarding Done step).
export function buildExamples(baseUrl: string) {
	const url = baseUrl.replace(/\/$/, "");
	return {
		curl: `curl ${url}/chat/completions \\
  -H "Content-Type: application/json" \\
  -H "Authorization: Bearer $PG_GATEWAY_API_KEY" \\
  -d '{
    "model": "gpt-4o-mini",
    "messages": [{"role": "user", "content": "Hello!"}]
  }'`,
		python: `from openai import OpenAI

client = OpenAI(
    base_url="${url}",
    api_key="YOUR_VIRTUAL_KEY",
)

resp = client.chat.completions.create(
    model="gpt-4o-mini",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)`,
		node: `import OpenAI from "openai";

const client = new OpenAI({
  baseURL: "${url}",
  apiKey: process.env.PG_GATEWAY_API_KEY,
});

const resp = await client.chat.completions.create({
  model: "gpt-4o-mini",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(resp.choices[0].message.content);`,
		go: `package main

import (
	"context"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	client := openai.NewClient(
		option.WithBaseURL("${url}"),
		option.WithAPIKey("YOUR_VIRTUAL_KEY"),
	)
	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("gpt-4o-mini"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello!"),
		}),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Choices[0].Message.Content)
}`,
	};
}