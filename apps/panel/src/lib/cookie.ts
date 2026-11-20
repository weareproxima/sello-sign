/**
 * Las cookies que comparten los hosts de Sello: la del modo de color
 * (`theme.ts`) y la pista de sesión del IdP (`@workspace/identity/hint`).
 * Sin DOM, porque la segunda la escribe también el Worker del IdP.
 */

/** Los sitios cuya cookie comparten sus subdominios; en local, el host solo. */
const SITES = ['sello.tax', 'sello.test']

/**
 * El atributo `domain` de una cookie que tienen que leer todos los hosts del
 * sitio (`; domain=sello.tax`), o nada fuera de él: `localhost` no comparte
 * cookies por dominio, y sin el atributo la cookie ya es de todos sus puertos.
 */
export function cookieDomain(host: string): string {
	const site = SITES.find((s) => host === s || host.endsWith(`.${s}`))
	return site ? `; domain=${site}` : ''
}
