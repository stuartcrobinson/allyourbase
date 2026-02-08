import { defineConfig } from "vitepress";

export default defineConfig({
  title: "AllYourBase",
  description: "Backend-as-a-Service for PostgreSQL",
  head: [["link", { rel: "icon", href: "/favicon.ico" }]],

  themeConfig: {
    nav: [
      { text: "Guide", link: "/guide/getting-started" },
      { text: "API Reference", link: "/guide/api-reference" },
      { text: "GitHub", link: "https://github.com/allyourbase/ayb" },
    ],

    sidebar: [
      {
        text: "Introduction",
        items: [
          { text: "Getting Started", link: "/guide/getting-started" },
          { text: "Configuration", link: "/guide/configuration" },
        ],
      },
      {
        text: "Features",
        items: [
          { text: "REST API", link: "/guide/api-reference" },
          { text: "Authentication", link: "/guide/authentication" },
          { text: "File Storage", link: "/guide/file-storage" },
          { text: "Realtime", link: "/guide/realtime" },
          { text: "Database RPC", link: "/guide/database-rpc" },
          { text: "Email", link: "/guide/email" },
          { text: "Admin Dashboard", link: "/guide/admin-dashboard" },
        ],
      },
      {
        text: "SDK & Tutorials",
        items: [
          { text: "JavaScript SDK", link: "/guide/javascript-sdk" },
          { text: "Quickstart: Todo App", link: "/guide/quickstart" },
        ],
      },
      {
        text: "Operations",
        items: [
          { text: "Deployment", link: "/guide/deployment" },
          { text: "Comparison", link: "/guide/comparison" },
        ],
      },
    ],

    socialLinks: [
      { icon: "github", link: "https://github.com/allyourbase/ayb" },
    ],

    footer: {
      message: "Released under the MIT License.",
    },

    search: {
      provider: "local",
    },
  },
});
