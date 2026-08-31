import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createGzip } from "node:zlib";
import type { IncomingMessage, ServerResponse } from "node:http";
import type { Plugin } from "vite";
import { defineConfig } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Dev-server gzip compression (Vite's dev server doesn't gzip by default).
// Compress any response that asks for gzip and isn't already compressed.
function gzipDevCompression(): Plugin {
	return {
		name: "dev-gzip-compression",
		configureServer(server) {
			server.middlewares.use((req: IncomingMessage, res: ServerResponse, next) => {
				const accept = req.headers["accept-encoding"];
				if (!accept || !accept.includes("gzip")) {
					next();
					return;
				}
				const doCompress = () => {
					if (res.getHeader("Content-Encoding") || !res.hasHeader("Content-Length")) {
						return;
					}
					const bodyLength = Number(res.getHeader("Content-Length") ?? 0);
					if (bodyLength < 1024) {
						return;
					}
					res.setHeader("Content-Encoding", "gzip");
					res.setHeader("Vary", "Accept-Encoding");
					res.removeHeader("Content-Length");
					res.pipe(createGzip());
				};
				res.on("pipe", doCompress);
				next();
			});
		},
	};
}

export default defineConfig({
	plugins: [
		tanstackRouter({
			target: "react",
			routesDirectory: "./app",
			generatedRouteTree: "./app/routeTree.gen.ts",
			// All routes live in layout.tsx files. page.tsx files are pure view
			// components imported by their sibling layout.tsx (Next-style mental
			// model preserved for content, but routing config lives in one place).
			routeToken: "layout",
			// Treat ONLY layout.tsx / __root.tsx as routes; everything else under app/
			// (page.tsx, views, components, helpers) is ignored.
			// Directory entries have no extension and are not matched, so recursion still works.
			routeFileIgnorePattern: "^(?!layout\\.tsx$|__root\\.tsx$).+\\.(tsx|ts|jsx|js)$",
			autoCodeSplitting: true,
		}),
		react(),
		tailwindcss(),
		gzipDevCompression(),
	],
	resolve: {
		alias: {
			"@": path.resolve(__dirname),
		},
	},
	define: {
		"process.env.NODE_ENV": JSON.stringify(process.env.NODE_ENV ?? "production"),
		"process.env.BIFROST_DISABLE_PROFILER": JSON.stringify(process.env.BIFROST_DISABLE_PROFILER ?? ""),
		"process.env.BIFROST_PORT": JSON.stringify(process.env.BIFROST_PORT ?? ""),
		"process.env.BUILD_TIME": JSON.stringify(new Date().toISOString()),
	},
	server: {
		port: 3000,
		proxy: {
			"/api": {
				target: `http://localhost:${process.env.BIFROST_PORT ?? "8080"}`,
				changeOrigin: true,
			},
			"/v1": {
				target: `http://localhost:${process.env.BIFROST_PORT ?? "8080"}`,
				changeOrigin: true,
			},
			"/ws": {
				target: `http://localhost:${process.env.BIFROST_PORT ?? "8080"}`,
				ws: true,
			},
		},
	},
	build: {
		outDir: "out",
		emptyOutDir: true,
	},
});