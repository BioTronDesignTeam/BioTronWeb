# frontend

Vite + React UI for BioTron Auth.

- **My access** — see grants, request `read` / `write` / `admin` per app
- **Approvals** — pending queue for managers and superusers
- **Keys** — reveal or copy independently rotating product guest keys (managers and superusers)

```bash
cp ../.env.example ../.env
npm install
npm run dev            # http://localhost:5173
```

Vite reads public `VITE_*` configuration from the repository-root `.env`.
