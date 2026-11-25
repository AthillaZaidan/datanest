import type { Migration, Recipe } from '../../api/migrations'

export const emptyRecipe: Recipe = { sqlUp: '', sqlDown: '', irreversible: false, irreversibleReason: '' }

export const recipeOf = (m: Migration): Recipe => ({
  sqlUp: m.sqlUp,
  sqlDown: m.sqlDown,
  irreversible: m.irreversible,
  irreversibleReason: m.irreversibleReason ?? '',
})

export const sameRecipe = (a: Recipe, b: Recipe) =>
  a.sqlUp === b.sqlUp && a.sqlDown === b.sqlDown && a.irreversible === b.irreversible && a.irreversibleReason === b.irreversibleReason

// Mirrors the backend rule so the save button is not offered for a recipe it would reject.
export const recipeComplete = (r: Recipe) => r.sqlUp.trim() !== '' && (!r.irreversible || r.irreversibleReason.trim() !== '')
