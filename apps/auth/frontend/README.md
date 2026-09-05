# frontend

Vite + React UI for BioTron Auth.

- **My access** — see and request the capabilities defined by each product
- **Approvals** — pending queue for managers and superusers
- **Keys** — reveal or copy independently rotating product guest keys (managers and superusers)

```bash
cp ../.env.example ../.env
npm install
npm run dev            # http://localhost:5173
```

Vite reads public `VITE_*` configuration from the repository-root `.env`.
