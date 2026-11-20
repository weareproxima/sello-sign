import { escapeHtml } from './email'

/**
 * The account e-mails better-auth sends (password reset, e-mail check): a
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
