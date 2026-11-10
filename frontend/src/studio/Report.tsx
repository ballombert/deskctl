import type { Execution } from "../api";
import { resultStatus } from "./labels";

export function Report({ execution, onClose }: { execution: Execution; onClose(): void }) {
  const results = execution.report.results ?? [];
  const recovery = execution.report.recovery;
  return (
    <section className="report" aria-labelledby="report-title">
      <header>
        <h2 id="report-title">Bilan du lot</h2>
        <button onClick={onClose} aria-label="Fermer le bilan">✕</button>
      </header>
      {execution.error && <p role="alert" className="error">{execution.error}</p>}
      {results.length === 0 && <p className="muted">Aucune action réalisée.</p>}
      <ul>
        {results.map((result, index) => (
          <li key={`${result.id}-${result.kind ?? ""}-${index}`} className={`result ${result.status}`}>
            <span className="badge">{resultStatus[result.status] ?? result.status}</span>
            <span className="kind">{result.kind === "layout" ? `Disposition${result.engine ? ` · ${result.engine}` : ""}` : "Bureau"}</span>
            <strong>{result.title || "(sans titre)"}</strong>
            <span className="muted">{result.message}</span>
          </li>
        ))}
      </ul>
      {recovery != null && (
        <label className="recovery">
          Données de récupération
          <textarea readOnly rows={6} value={JSON.stringify(recovery, null, 2)} />
        </label>
      )}
    </section>
  );
}
