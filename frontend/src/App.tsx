export function App() {
  return (
    <main>
      <p className="eyebrow">BioTron operations</p>
      <h1>Logs and service health</h1>
      <p>
        Recent events and the latest health state will read from Redis; history will read from Postgres.
      </p>
    </main>
  );
}
