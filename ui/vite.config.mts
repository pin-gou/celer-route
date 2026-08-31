import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createGzip } from "node:zlib";
import { defineConfig, type Plugin } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Dev-server only: gzip-compress outgoing responses (HTML, JS, CSS, JSON) using
// Node's built-in zlib. Vite's dev server doesn't compress by default.
// Registered as a pre-hook (configureServer without a returned post hook), so it
// is installed BEFORE Vite's transform/static/proxy/index-html middlewares and
// wraps every response they write. The Content-Encoding header is applied lazily
// on the first body write, so status-only responses (304/204), SSE streams, and
// already-encoded/proxied responses are left untouched.
function gzipDevServer(): Plugin {
	return {
		name: "gzip-dev-server",
		configureServer(server) {
			server.middlewares.use((req, res, next) => {
				const method = req.method ?? "GET";
				if (method !== "GET" && method !== "HEAD") return next();
				const acceptEncoding = req.headers["accept-encoding"];
				if (!acceptEncoding?.toString().includes("gzip")) return next();
				if ((req.headers.upgrade ?? "").toString().toLowerCase() === "websocket") return next();

				const write = res.write.bind(res);
				const end = res.end.bind(res);
				// "idle" until the first body write decides: "compress" or "skip"
				// (status-only response, SSE, or already Content-Encoding'd).
				let mode: "idle" | "compress" | "skip" = "idle";
				let gzip: ReturnType<typeof createGzip> | null = null;
				let finished = false;

				const decide = () => {
					if (mode !== "idle" || finished) return;
					// Headers already flushed to the client (e.g. static files served
					// via res.writeHead + stream.pipe) can't get Content-Encoding
					// added retroactively, so leave those responses untouched.
					if (res.headersSent) {
						mode = "skip";
						return;
					}
					const code = res.statusCode || 200;
					const isStatusOnly = (code >= 100 && code < 200) || code === 204 || code === 304 || code === 205;
					const contentType = res.getHeader("Content-Type");
					if (
						isStatusOnly ||
						res.getHeader("Content-Encoding") ||
						(typeof contentType === "string" && contentType.startsWith("text/event-stream"))
					) {
						mode = "skip";
						return;
					}
					mode = "compress";
					gzip = createGzip();
					res.setHeader("Content-Encoding", "gzip");
					res.appendHeader?.("Vary", "Accept-Encoding");
					res.removeHeader("Content-Length");
					gzip.on("data", (chunk: Buffer) => write(chunk));
					gzip.on("end", () => {
						finished = true;
						end();
					});
					gzip.on("error", () => {
						if (finished || !gzip) return;
						finished = true;
						res.removeHeader("Content-Encoding");
						end();
					});
				};

				res.write = ((chunk: unknown, encodingOrCb?: unknown, cb?: unknown) => {
					if (typeof encodingOrCb === "function") {
						cb = encodingOrCb;
						encodingOrCb = undefined;
					}
					if (finished) return true;
					decide();
					if (mode !== "compress") return write(chunk as never, encodingOrCb as never, cb as never);
					if (typeof cb === "function") gzip?.once("drain", cb as () => void);
					return gzip?.write(chunk as never, encodingOrCb as never) ?? false;
				}) as typeof res.write;

				res.end = ((chunk?: unknown, encodingOrCb?: unknown, cb?: unknown) => {
					if (typeof encodingOrCb === "function") {
						cb = encodingOrCb;
						encodingOrCb = undefined;
					}
					if (finished) return res;
					decide();
					if (mode !== "compress") {
						finished = true;
						return end(chunk as never, encodingOrCb as never, cb as never);
					}
					if (chunk !== undefined && chunk !== null) {
						gzip?.write(chunk as never, encodingOrCb as never);
					}
					if (typeof cb === "function") gzip?.once("end", cb as () => void);
					gzip?.end();
					return res;
				}) as typeof res.end;

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
		gzipDevServer(),
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
	// Vite 8 的 dev server 对 client 端代码只应用 oxc/esbuild transform 的 define
	// (vite:define 插件对 client consumer 直接跳过), 顶层的 define 仅在 build 时生效.
	// 因此 dev + client 需要复述这里并传入同样的值.
	oxc: {
		define: {
			"process.env.NODE_ENV": JSON.stringify(process.env.NODE_ENV ?? "production"),
			"process.env.BIFROST_DISABLE_PROFILER": JSON.stringify(process.env.BIFROST_DISABLE_PROFILER ?? ""),
			"process.env.BIFROST_PORT": JSON.stringify(process.env.BIFROST_PORT ?? ""),
			"process.env.BUILD_TIME": JSON.stringify(new Date().toISOString()),
		},
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