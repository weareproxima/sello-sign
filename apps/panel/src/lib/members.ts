/**
 * The rules of the members page, as the API applies them
 * (`model.CanInvite` and `model.CanSetRole` of the API): owners and admins
 * invite and change roles, and only an owner gives or takes the owner role.
 * The API has the last word; this only keeps the page from offering what it
 * would refuse.
 */

import type { Role } from './api'

const ROLES: Role[] = ['owner', 'admin', 'member']

/** The roles someone with role `mine` can invite as or give. */
export function rolesToGive(mine: Role | null): Role[] {
	if (mine === 'owner') return ROLES
	if (mine === 'admin') return ['admin', 'member']
	return []
}

/** Whether someone with role `mine` can change the role of a member who
 * has `theirs`. */
export function canChangeRole(mine: Role | null, theirs: Role): boolean {
	if (mine !== 'owner' && mine !== 'admin') return false
	return theirs !== 'owner' || mine === 'owner'
}

/** Whether someone with role `mine` can take a member who has `theirs` out
 * of the space (`model.CanRemove`): anyone leaves by themselves; owners and
 * admins remove others, and only an owner removes an owner. That the space
 * keeps an owner is the API's to say (`last-owner`). */
export function canRemove(
	mine: Role | null,
	theirs: Role,
	self: boolean
): boolean {
	if (self) return mine !== null
	return canChangeRole(mine, theirs)
}

/**
 * Where the members of a space of an organization of Sello are invited and
 * changed: «Su cuenta › Organización › Miembros y acceso», under Sello Sign,
 * for that organization.
 */
export function membersPageUrl(
	accountCenter: string,
	organization: string
): string {
	const url = new URL('/u/0/sign/members', `${accountCenter}/`)
	url.searchParams.set('organization', organization)
	return url.toString()
}
