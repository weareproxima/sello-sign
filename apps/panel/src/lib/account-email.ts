import { escapeHtml } from './email'

/**
 * The account e-mails (the password reset, the change of e-mail): a
 * paragraph, a button to `url` and the address written out, for clients that
 * hide buttons. Written colours, because no mail client reads the tokens'
 * variables (`lint:design` leaves `*-email.ts` out).
 */
export function actionEmail({
	subject,
	intro,
	action,
	url,
	outro,
}: {
	subject: string
	intro: string
	action: string
	url: string
	outro: string
}): { subject: string; text: string; html: string } {
	const text = `${intro}\n\n${action}: ${url}\n\n${outro}\n\nSello Sign`
	const href = escapeHtml(url)
	const html = `<!doctype html><html lang="es"><body style="margin:0;padding:24px;background:#f6f6f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1b1b1a">
<div style="max-width:480px;margin:0 auto;background:#ffffff;border-radius:16px;padding:32px">
<p style="margin:0 0 24px;font-size:15px;line-height:1.5">${escapeHtml(intro)}</p>
<p style="margin:0 0 24px"><a href="${href}" style="display:inline-block;background:#1b1b1a;color:#ffffff;text-decoration:none;padding:10px 18px;border-radius:10px;font-size:14px;font-weight:500">${escapeHtml(action)}</a></p>
<p style="margin:0 0 8px;font-size:13px;line-height:1.5;color:#6b6b68">Si el botón no funciona, copie esta dirección en su navegador:</p>
<p style="margin:0 0 24px;font-size:13px;line-height:1.5;word-break:break-all"><a href="${href}" style="color:#1b1b1a">${href}</a></p>
<p style="margin:0;font-size:13px;line-height:1.5;color:#6b6b68">${escapeHtml(outro)}</p>
</div>
<p style="max-width:480px;margin:16px auto 0;font-size:12px;color:#8a8a86">Sello Sign</p>
</body></html>`
	return { subject, text, html }
}

/** The password reset e-mail. */
export function resetPasswordEmail(url: string) {
	return actionEmail({
		subject: 'Restablecer su contraseña de Sello Sign',
		intro:
			'Recibimos una solicitud para restablecer la contraseña de su cuenta de Sello Sign. El enlace vence en una hora.',
		action: 'Elegir una contraseña nueva',
		url,
		outro: 'Si no la solicitó, ignore este correo: su contraseña no cambia.',
	})
}

/** The link that changes the account's e-mail, sent to the new address
 * (`lib/account-email-change.ts`). */
export function changeEmailEmail(url: string) {
	return actionEmail({
		subject: 'Confirmar su nuevo correo de Sello Sign',
		intro:
			'Pidió usar este correo en su cuenta de Sello Sign. Confírmelo para terminar el cambio. El enlace vence en una hora.',
		action: 'Confirmar el correo',
		url,
		outro:
			'Si no lo pidió, ignore este correo: su cuenta de Sello Sign no cambia.',
	})
}

/** The notice to the current address that someone asked to change it. */
export function emailChangeNoticeEmail(newEmail: string, accountUrl: string) {
	return actionEmail({
		subject: 'Cambio de correo en su cuenta de Sello Sign',
		intro: `Se pidió cambiar el correo de su cuenta de Sello Sign a ${newEmail}. El cambio se hace cuando se abra el enlace que enviamos a esa dirección; hasta entonces, su cuenta sigue con este correo.`,
		action: 'Revisar su cuenta',
		url: accountUrl,
		outro:
			'Si no fue usted, cambie su contraseña y cierre las demás sesiones en «Su cuenta».',
	})
}
