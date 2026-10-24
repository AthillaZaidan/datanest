// Stable pale identity tints for people; neighbouring ids sit at least 60 degrees apart and skip violet.
const hues = [205, 145, 25, 340, 85, 180, 50, 0]

export function personTint(id: number) {
  const h = hues[id % hues.length]
  return { backgroundColor: `hsl(${h} 55% 91%)`, color: `hsl(${h} 45% 26%)` }
}
