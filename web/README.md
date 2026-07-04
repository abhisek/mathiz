# Mathiz Web

The parent dashboard and kid learning terminal for [Mathiz SaaS mode](../docs/saas.md).

Vite + React + TypeScript. `npm run build` emits into
`../internal/saas/webui/dist`, which `mathiz serve` embeds — build via
`make web` from the repo root.

For development with hot reload, run `mathiz serve` on :8080 and then:

```sh
npm install
npm run dev
```

Vite proxies `/api` (including the terminal WebSocket) to the Go server.
