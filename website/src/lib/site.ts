export const site = {
  name: "evil-mygo",
  url: (import.meta.env?.VITE_SITE_URL || (typeof process !== "undefined" && process.env.VITE_SITE_URL) || "http://localhost:3000").replace(/\/$/, ""),
  title: "evil-mygo — A MyGo fork for native GPU scenes",
  description:
    "An independently versioned fork of MyGo, focused on WebGPU scenes, shared GPU textures and interactive native UI on three-dimensional surfaces. Includes the shared MyGo framework documentation.",
  repo: "https://github.com/caelyreth/evil-mygo",
}
