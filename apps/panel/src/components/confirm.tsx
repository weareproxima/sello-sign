import * as React from 'react'
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from '#/components/ui/alert-dialog'

/**
 * A confirmation for what can't be undone: the title as a question and
 * «Esta acción no se puede deshacer.» when it applies.
 */
export function Confirm({
	open,
	onOpenChange,
	title,
	description,
	confirm,
	onConfirm,
	pending,
}: {
	open: boolean
	onOpenChange: (open: boolean) => void
	title: string
	description: React.ReactNode
	confirm: string
	onConfirm: () => void
	pending?: boolean
}) {
	return (
		<AlertDialog open={open} onOpenChange={onOpenChange}>
			<AlertDialogContent>
				<AlertDialogHeader>
					<AlertDialogTitle>{title}</AlertDialogTitle>
					<AlertDialogDescription>{description}</AlertDialogDescription>
				</AlertDialogHeader>
				<AlertDialogFooter>
					<AlertDialogCancel>Cancelar</AlertDialogCancel>
					<AlertDialogAction
						variant="destructive"
						disabled={pending}
						onClick={onConfirm}
					>
						{confirm}
					</AlertDialogAction>
				</AlertDialogFooter>
			</AlertDialogContent>
		</AlertDialog>
	)
}
