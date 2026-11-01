import path from "node:path";
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
export default defineConfig(({mode}) => ({
 base: mode === "electron" ? "./" : (process.env.VITE_BASE_PATH || "/"),
 resolve:{alias:{"@":path.resolve(__dirname,"src")}},
 define:{"process.env.DRAGGABLE_DEBUG":"false"},
 plugins:[react()],
 server:{host:"127.0.0.1",port:7777,headers:{"Cross-Origin-Opener-Policy":"same-origin","Cross-Origin-Embedder-Policy":"require-corp"}},
 build:{outDir:mode === "electron" ? "dist" : "dist-web",sourcemap:true,chunkSizeWarningLimit:1500}
}));
