import { describe, expect, it } from 'vitest'
import {
	WORKSPACES_CLAIM,
	publicConfig,
	readConfig,
	workspacesIn,
} from './config'
import { selloFieldsOf, tokenPayload } from './auth-options'

describe('readConfig', () => {
	it('defaults to the local ports', () => {
		const config = readConfig({})
		expect(config.panelUrl).toBe('http://127.0.0.1:4440')
		expect(config.apiUrl).toBe('http://127.0.0.1:4441')
		expect(config.cloud).toBe(false)
		expect(config.oidc).toBeNull()
	})

	it('needs the issuer, the client and its secret for OIDC', () => {
		expect(
			readConfig({ OIDC_ISSUER: 'https://idp', OIDC_CLIENT_ID: 'c' }).oidc
		).toBeNull()
		const config = readConfig({
			OIDC_ISSUER: 'https://account.sello.tax/',
			OIDC_CLIENT_ID: 'c',
			OIDC_CLIENT_SECRET: 's',
			OIDC_ONLY: 'true',
			CLOUD: 'true',
		})
		expect(config.oidc).toMatchObject({
			issuer: 'https://account.sello.tax',
			providerId: 'sello',
			name: 'Sello',
			only: true,
		})
	})

	it('never shows a secret to the pages', () => {
		const config = readConfig({
			BETTER_AUTH_SECRET: 'x',
			OIDC_ISSUER: 'https://idp',
			OIDC_CLIENT_ID: 'c',
			OIDC_CLIENT_SECRET: 'secret',
		})
		expect(JSON.stringify(publicConfig(config))).not.toContain('secret')
		expect(publicConfig(config).oidc).toEqual({
			providerId: 'oidc',
			name: 'SSO',
			only: false,
		})
	})
})

describe('the claims of Sello', () => {
	const organizations = [
		{ id: 'a', name: 'Empresa Oc', country: 'SV', role: 'owner' },
		{ id: 'b', name: 'Sin rol', country: 'CR' },
		{ name: 'Sin id' },
	]

	it('reads the organizations of the ID token', () => {
		expect(workspacesIn({ [WORKSPACES_CLAIM]: organizations })).toEqual([
			{ id: 'a', name: 'Empresa Oc', country: 'SV', role: 'owner' },
			{ id: 'b', name: 'Sin rol', country: 'CR', role: 'member' },
		])
		expect(workspacesIn({})).toEqual([])
	})

	it('keeps them from the ID token on the user', () => {
		const part = (o: unknown) =>
			Buffer.from(JSON.stringify(o)).toString('base64url')
		const idToken = `${part({})}.${part({ sub: 'sello-1', [WORKSPACES_CLAIM]: organizations })}.x`
		const fields = selloFieldsOf(idToken)
		expect(fields?.selloSub).toBe('sello-1')
		expect(JSON.parse(fields!.selloWorkspaces)).toHaveLength(2)
		expect(selloFieldsOf(null)).toBeNull()
		expect(selloFieldsOf('not-a-jwt')).toBeNull()
	})

	it('puts them in the token for the API only for Sello accounts', () => {
		expect(tokenPayload({ email: 'a@b.c', name: 'Ana' })).toEqual({
			email: 'a@b.c',
			name: 'Ana',
		})
		expect(
			tokenPayload({
				email: 'a@b.c',
				name: 'Ana',
				selloSub: 'sello-1',
				selloWorkspaces: JSON.stringify([
					{ id: 'a', name: 'Oc', country: 'SV', role: 'owner' },
				]),
			})
		).toEqual({
			email: 'a@b.c',
			name: 'Ana',
			sello_sub: 'sello-1',
			sello_workspaces: [{ id: 'a', name: 'Oc', country: 'SV', role: 'owner' }],
		})
	})
})
