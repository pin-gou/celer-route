import tailwindcss from "@tailwindcss/vite";
import { tanstackRouter } from "@tanstack/router-plugin/vite";
import react from "@vitejs/plugin-react";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { defineConfig } from "vite";

const __dirname = path.dirname(fileURLToPath(import.meta.url));

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