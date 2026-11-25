import type { Recipe } from '../../api/migrations'
import { Field } from '../kit'
import { ui } from '../ui'
import SqlEditor from './SqlEditor'

type Props = { value: Recipe; onChange: (r: Recipe) => void; readOnly?: boolean }

// The up SQL, and either its down SQL or the reason it cannot be rolled back.
export default function RecipeEditor({ value, onChange, readOnly }: Props) {
  const set = (patch: Partial<Recipe>) => onChange({ ...value, ...patch })
  return (
    <div className="space-y-4">
      <SqlEditor label="up.sql" value={value.sqlUp} readOnly={readOnly} onChange={(sqlUp) => set({ sqlUp })} placeholder="ALTER TABLE products ADD COLUMN sku text;" />

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
        <SqlEditor label="down.sql" value={value.sqlDown} readOnly={readOnly} onChange={(sqlDown) => set({ sqlDown })} placeholder="ALTER TABLE products DROP COLUMN sku;" />
      )}
    </div>
  )
}
