import type * as React from 'react'

/**
 * A long table that, on a phone, shows each row as a card
 * (`styles/cards.css`). Each cell says its column in `data-label`; the
 * row's title cell goes without one, and its actions cell with
 * `data-actions`.
 */
export function TableCards({ children }: { children: React.ReactNode }) {
	return <div className="table-cards">{children}</div>
}
