import { NativeSelect, NativeSelectOption } from '#/components/ui/native-select'
import { OFFERED_COUNTRIES, countryName } from '#/lib/countries'

export function CountrySelect({
	id,
	value,
	onChange,
	placeholder = 'Elija un país',
}: {
	id?: string
	value: string
	onChange: (value: string) => void
	placeholder?: string
}) {
	return (
		<NativeSelect
			id={id}
			className="w-full"
			value={value}
			onChange={(e) => onChange(e.target.value)}
			required
		>
			<NativeSelectOption value="" disabled>
				{placeholder}
			</NativeSelectOption>
			{OFFERED_COUNTRIES.map((code) => (
				<NativeSelectOption key={code} value={code}>
					{countryName(code)}
				</NativeSelectOption>
			))}
		</NativeSelect>
	)
}
