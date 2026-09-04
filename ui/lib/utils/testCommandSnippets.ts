import type { ClientPlatform } from "@/lib/types/platform";

export function defaultEndpoint(): string {
	if (typeof window === "undefined") return "http://localhost:8080/v1";
	return `${window.location.origin}/v1`;
}

// Exported so other modules can reuse the derivation without duplicating it.
export function resolveEndpointUrl(): string {
	return defaultEndpoint();
}

export interface TestCommandSnippets {
	curl: string;
	python: string;
	node: string;
	go: string;
}

// Side effect-free examples builder shared by onboarding's DoneStep and the
// home endpoint card so the two surfaces generate byte-identical snippets.
// `platform` is optional and only affects the curl flavor: agent-setup passes
// it so Windows users get a curl.exe / PowerShell continuation, while the
// onboarding + home surfaces keep the POSIX form.
export function buildExamples(baseUrl: string, model: string, vkValue: string | null, platform?: ClientPlatform): TestCommandSnippets {
	const url = baseUrl.replace(/\/$/, "");

	let curl: string;
	if (platform === "windows") {
		const lines = [`curl.exe ${url}/chat/completions \``];
		lines.push(`  -H "Content-Type: application/json" \``);
		if (vkValue) {
			lines.push(`  -H "Authorization: Bearer ${vkValue}" \``);
		}
		lines.push(`  -d '{`, `    "model": "${model}",`, `    "messages": [{"role": "user", "content": "Hello!"}]`, `  }'`);
		curl = lines.join("\n");
	} else {
		const curlLines = [`curl ${url}/chat/completions \\`, `  -H "Content-Type: application/json" \\`];
		if (vkValue) {
			curlLines.push(`  -H "Authorization: Bearer ${vkValue}" \\`);
		}
		curlLines.push(`  -d '{`, `    "model": "${model}",`, `    "messages": [{"role": "user", "content": "Hello!"}]`, `  }'`);
		curl = curlLines.join("\n");
	}

	const apiKeyLine = vkValue ? `    api_key="${vkValue}",` : null;
	const nodeApiKey = vkValue ? `  apiKey: "${vkValue}",` : null;
	const goApiKey = vkValue ? `\t\toption.WithAPIKey("${vkValue}"),` : null;

	const python = `from openai import OpenAI

client = OpenAI(
    base_url="${url}",${apiKeyLine ? `\n${apiKeyLine}` : ""}
)

resp = client.chat.completions.create(
    model="${model}",
    messages=[{"role": "user", "content": "Hello!"}],
)
print(resp.choices[0].message.content)`;

	const node = `import OpenAI from "openai";

const client = new OpenAI({${nodeApiKey ? `\n${nodeApiKey}` : ""}
  baseURL: "${url}",
});

const resp = await client.chat.completions.create({
  model: "${model}",
  messages: [{ role: "user", content: "Hello!" }],
});
console.log(resp.choices[0].message.content);`;

	const go = `package main

import (
	"context"
	"fmt"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	client := openai.NewClient(
		option.WithBaseURL("${url}"),${goApiKey ? `\n${goApiKey}` : ""}
	)
	resp, err := client.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
		Model: openai.F("${model}"),
		Messages: openai.F([]openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Hello!"),
		}),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(resp.Choices[0].Message.Content)
}`;

	return { curl, python, node, go };
}