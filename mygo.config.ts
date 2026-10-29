import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "Game Launcher",
  identifier: "com.shen.gamelauncher",
  version: "0.1.0",
  publisher: "Shen",
  copyright: "© 2026 Shen",
  icon: "resources/icon.png",
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "bun run dev:web",
  buildCommand: "bun run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",

  macos: {
    // color-mix()/backdrop-filter 这些视觉特性要 macOS 14 才稳
    minimumSystemVersion: "14.0",
  },
});
