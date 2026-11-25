import type { MigrationMode, Recipe } from '../../api/migrations'
import { Field } from '../kit'
import { ui } from '../ui'
import SqlEditor from './SqlEditor'

type Props = { value: Recipe; onChange: (r: Recipe) => void; readOnly?: boolean }

const modes: { key: MigrationMode; label: string }[] = [
  { key: 'normal', label: 'Normal' },
  { key: 'batch', label: 'Per-batch' },
]

const placeholders = {
  normal: { up: 'ALTER TABLE products ADD COLUMN sku text;', down: 'ALTER TABLE products DROP COLUMN sku;' },
  batch: {
    up: "UPDATE orders SET status = 'x' WHERE id > $1 AND id <= $2",
    down: "UPDATE orders SET status = 'y' WHERE id > $1 AND id <= $2",
  },
}

// The up SQL, and either its down SQL or the reason it cannot be rolled back.
export default function RecipeEditor({ value, onChange, readOnly }: Props) {
  const set = (patch: Partial<Recipe>) => onChange({ ...value, ...patch })
  const batch = value.mode === 'batch'
  const sample = placeholders[value.mode]
  return (
    <div className="space-y-4">
      <fieldset className="min-w-0" disabled={readOnly}>
        <legend className={ui.label}>Mode eksekusi</legend>
        <div className="flex flex-wrap gap-x-6 gap-y-2">
          {modes.map((m) => (
            <label key={m.key} className="flex items-center gap-2 text-sm">
              <input
                type="radio"
                name="migration-mode"
                checked={value.mode === m.key}
                onChange={() => set(m.key === 'normal' ? { mode: 'normal', batchTable: '', batchColumn: '' } : { mode: 'batch' })}
              />
              {m.label}
            </label>
          ))}
        </div>
      </fieldset>

      {batch && (
        <div className="space-y-3">
          <div className="grid gap-4 sm:grid-cols-2">
            <Field label="Tabel" hint="Tabel yang diubah per batch, boleh memakai schema.">
              <input
                className={`${ui.input} font-mono`}
                value={value.batchTable}
                readOnly={readOnly}
                onChange={(e) => set({ batchTable: e.target.value })}
                placeholder="orders atau schema.orders"
              />
            </Field>
            <Field label="Kolom" hint="Kolom unik yang dipakai untuk membagi batch, misalnya id.">
              <input
                className={`${ui.input} font-mono`}
                value={value.batchColumn}
                readOnly={readOnly}
                onChange={(e) => set({ batchColumn: e.target.value })}
                placeholder="id"
              />
            </Field>
          </div>
          <p className={ui.small}>
            SQL up dan down masing-masing satu statement yang memakai <code className="font-mono">$1</code> (batas bawah, tidak termasuk) dan{' '}
            <code className="font-mono">$2</code> (batas atas, termasuk), misalnya{' '}
            <code className="font-mono break-all">UPDATE orders SET status = 'x' WHERE id &gt; $1 AND id &lt;= $2</code>.
          </p>
        </div>
      )}

      <SqlEditor label="up.sql" value={value.sqlUp} readOnly={readOnly} onChange={(sqlUp) => set({ sqlUp })} placeholder={sample.up} />

      <label className="flex items-center gap-2 text-sm">
        <input type="checkbox" checked={value.irreversible} disabled={readOnly} onChange={(e) => set({ irreversible: e.target.checked })} />
        Irreversible: migration ini tidak bisa di-rollback
      </label>

      {value.irreversible ? (
        <Field label="Alasan irreversible" hint="Wajib diisi. Rollback akan ditolak untuk migration ini.">
          <input
            className={ui.input}
            value={value.irreversibleReason}
            readOnly={readOnly}
            onChange={(e) => set({ irreversibleReason: e.target.value })}
            placeholder="Kolom dihapus beserta datanya."
          />
        </Field>
      ) : (
        <SqlEditor label="down.sql" value={value.sqlDown} readOnly={readOnly} onChange={(sqlDown) => set({ sqlDown })} placeholder={sample.down} />
      )}
    </div>
  )
}
