import { useState } from 'react'
import { explainMigration, type StatementPlan } from '../../api/migrations'
import { Alert } from '../kit'
import { ui } from '../ui'

const n = (v: number | null | undefined) => (v == null ? '-' : v.toLocaleString('id-ID', { maximumFractionDigits: 2 }))
const ms = (v: number | null | undefined) => (v == null ? '-' : `${n(v)} ms`)

// Analyzes the saved SQL in a rolled-back transaction, so the database is never changed.
export default function PlanPanel({ migrationId, canEdit, dirty }: { migrationId: string; canEdit: boolean; dirty: boolean }) {
  const [analyze, setAnalyze] = useState(false)
  const [shown, setShown] = useState<{ plans: StatementPlan[]; analyze: boolean } | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const run = async () => {
    setBusy(true)
    setError(null)
    try {
      const { plans } = await explainMigration(migrationId, analyze)
      setShown({ plans: plans ?? [], analyze })
    } catch (e) {
      setError((e as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <section aria-labelledby="explain" className="space-y-3 rounded-lg border border-line p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h2 id="explain" className="text-base font-semibold">Analisis execution plan</h2>
          <p className={ui.small}>
            EXPLAIN untuk setiap statement pada SQL yang tersimpan. Semuanya dijalankan dalam satu transaction lalu di-rollback, jadi database tidak berubah.
            {dirty && ' Simpan dulu agar perubahan ikut dianalisis.'}
          </p>
        </div>
        <div className="flex items-center gap-3">
          {canEdit && (
            <label className="flex items-center gap-2 text-sm">
              <input type="checkbox" checked={analyze} onChange={(e) => setAnalyze(e.target.checked)} className="size-4 accent-[var(--color-ink)]" /> ANALYZE
            </label>
          )}
          <button className={ui.button} disabled={busy} onClick={run}>{busy ? 'Menganalisis...' : 'Analisis'}</button>
        </div>
      </div>
      {error && <Alert>{error}</Alert>}
      {shown && (
        <>
          <h3 className="text-sm font-semibold">
            Hasil analisis{shown.analyze && ' (ANALYZE: query dijalankan sungguhan lalu di-rollback)'}
          </h3>
          {shown.plans.map((p, i) => (
            <Statement key={i} index={i + 1} plan={p} analyze={shown.analyze} />
          ))}
        </>
      )}
    </section>
  )
}

function Statement({ index, plan, analyze }: { index: number; plan: StatementPlan; analyze: boolean }) {
  return (
    <div className="rounded-lg bg-raised p-3">
      <p className="mb-2 flex items-baseline gap-2 text-xs text-muted">
        <span className="flex-none font-medium text-ink">Statement {index}</span>
        <code className="truncate font-mono text-[12px]">{plan.sql}</code>
      </p>
      {plan.error && <Alert>{plan.error}</Alert>}
      {plan.note && <p className={ui.small}>{plan.note}</p>}
      {plan.explained && (
        <>
          <div className="overflow-x-auto rounded-md bg-sheet">
            <table className={`${ui.table} min-w-[640px]`}>
              <thead>
                <tr>
                  <th className={`${ui.th} pl-3`}>Node</th>
                  <th className={ui.th}>Relation / index</th>
                  <th className={ui.th}>Join</th>
                  <th className={`${ui.th} ${ui.num}`}>Cost</th>
                  <th className={`${ui.th} ${ui.num}`}>Rows (est.)</th>
                  {analyze && (
                    <>
                      <th className={`${ui.th} ${ui.num}`}>Rows aktual</th>
                      <th className={`${ui.th} ${ui.num}`}>Waktu aktual</th>
                      <th className={`${ui.th} ${ui.num} pr-3`}>Loops</th>
                    </>
                  )}
                </tr>
              </thead>
              <tbody>
                {(plan.nodes ?? []).map((node, i) => (
                  <tr key={i}>
                    <td className={`${ui.td} pl-3 whitespace-nowrap`}>
                      <span style={{ paddingLeft: node.depth * 16 }} className="inline-block font-medium">
                        {node.nodeType}
                        {node.operation && <span className="font-normal text-muted"> ({node.operation})</span>}
                        {node.strategy && <span className="font-normal text-muted"> {node.strategy}</span>}
                      </span>
                    </td>
                    <td className={`${ui.td} ${ui.ident}`}>
                      {node.relation}
                      {node.index && <><span className="text-muted"> using </span>{node.index}</>}
                    </td>
                    <td className={ui.td}>{node.joinType}</td>
                    <td className={`${ui.td} ${ui.num}`}>{n(node.startupCost)}..{n(node.totalCost)}</td>
                    <td className={`${ui.td} ${ui.num}`}>{n(node.planRows)}</td>
                    {analyze && (
                      <>
                        <td className={`${ui.td} ${ui.num}`}>{n(node.actualRows)}</td>
                        <td className={`${ui.td} ${ui.num}`}>{ms(node.actualMs)}</td>
                        <td className={`${ui.td} ${ui.num} pr-3`}>{n(node.loops)}</td>
                      </>
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {analyze && <p className={`${ui.small} mt-2`}>Planning {ms(plan.planningMs)} · Eksekusi {ms(plan.executionMs)}</p>}
        </>
      )}
    </div>
  )
}
