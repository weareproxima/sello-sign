/** The countries the panel names, and the name of each one's tax id. */
export const COUNTRIES: Record<string, { name: string; taxId: string }> = {
	CR: { name: 'Costa Rica', taxId: 'Cédula' },
	SV: { name: 'El Salvador', taxId: 'NIT' },
	GT: { name: 'Guatemala', taxId: 'NIT' },
	PA: { name: 'Panamá', taxId: 'RUC' },
	DO: { name: 'República Dominicana', taxId: 'RNC' },
	EC: { name: 'Ecuador', taxId: 'RUC' },
	BO: { name: 'Bolivia', taxId: 'NIT' },
	PE: { name: 'Perú', taxId: 'RUC' },
	CO: { name: 'Colombia', taxId: 'NIT' },
	UY: { name: 'Uruguay', taxId: 'RUT' },
	BR: { name: 'Brasil', taxId: 'CNPJ' },
}

/**
 * The ones offered by default, as the API's `SIGN_COUNTRIES` default
 * (`SV,DO,CR,PA,GT`), in alphabetical order of their Spanish names.
 */
export const OFFERED_COUNTRIES = ['CR', 'SV', 'GT', 'PA', 'DO'] as const

export const countryName = (code: string) => COUNTRIES[code]?.name ?? code
export const taxIdLabel = (code: string) =>
	COUNTRIES[code]?.taxId ?? 'Identificación tributaria'

/** El Salvador signs JSON (JWS); every other country, XML. */
export const documentFormat = (code: string): 'json' | 'xml' =>
	code === 'SV' ? 'json' : 'xml'
