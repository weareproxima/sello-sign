package notices

import (
	"fmt"
	"html"
	"strconv"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/mail"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// The texts of the notices: Spanish as Sello writes it (of usted, neutral,
// without marking who reads), plain text and the same in simple HTML,
// without images, tracking or anything that depends on when it's sent, so
// that a notice tried again is the same message (its idempotency key
// relies on it).

// certificateItem is a certificate and the threshold it's notified of.
type certificateItem struct {
	store.ExpiringCertificate
	Threshold string
}

// environmentName is an environment as the texts say it.
func environmentName(env model.Environment) string {
	if env == model.Prod {
		return "producción"
	}
	return "prueba"
}

var months = [...]string{"enero", "febrero", "marzo", "abril", "mayo", "junio", "julio", "agosto",
	"septiembre", "octubre", "noviembre", "diciembre"}

// formatDate is «1 de diciembre de 2026», in UTC.
func formatDate(t time.Time) string {
	t = t.UTC()
	return fmt.Sprintf("%d de %s de %d", t.Day(), months[t.Month()-1], t.Year())
}

// formatDateTime is «1 de diciembre de 2026 a las 14:05 UTC».
func formatDateTime(t time.Time) string {
	return formatDate(t) + " a las " + t.UTC().Format("15:04") + " UTC"
}

// formatNumber writes n as es-419 does: «10,000».
func formatNumber(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func itoa(n int) string { return strconv.Itoa(n) }

// doc writes a message's text and its HTML side by side.
type doc struct {
	text, html strings.Builder
}

// para is a paragraph.
func (d *doc) para(s string) {
	d.text.WriteString(s + "\n\n")
	d.html.WriteString("<p>" + html.EscapeString(s) + "</p>\n")
}

// link is a paragraph that leads to url, written out.
func (d *doc) link(lead, url string) {
	d.text.WriteString(lead + ":\n" + url + "\n\n")
	u := html.EscapeString(url)
	d.html.WriteString("<p>" + html.EscapeString(lead) + ":<br>\n<a href=\"" + u + "\">" + u + "</a></p>\n")
}

// list is a list whose items each lead to a url.
func (d *doc) list(items [][2]string) {
	d.html.WriteString("<ul>\n")
	for _, it := range items {
		d.text.WriteString("- " + it[0] + "\n  " + it[1] + "\n")
		u := html.EscapeString(it[1])
		d.html.WriteString("<li>" + html.EscapeString(it[0]) + "<br>\n<a href=\"" + u + "\">" + u + "</a></li>\n")
	}
	d.text.WriteString("\n")
	d.html.WriteString("</ul>\n")
}

func (d *doc) message(subject string) mail.Message {
	return mail.Message{Subject: subject, Text: strings.TrimRight(d.text.String(), "\n") + "\n", HTML: d.html.String()}
}

// footer says why the notice came.
func (d *doc) footer(space string) {
	d.para(fmt.Sprintf("Recibe este aviso porque administra el espacio «%s» en Sello Sign.", space))
}

func organizationURL(panelURL, workspaceID, organizationID string) string {
	return panelURL + "/w/" + workspaceID + "/organizations/" + organizationID
}

// certificateMessage is the notice of a space's certificates due, in the
// order given.
func certificateMessage(panelURL string, items []certificateItem) mail.Message {
	var d doc
	space := items[0].WorkspaceName
	if len(items) == 1 {
		c := items[0]
		env := environmentName(c.Environment)
		url := organizationURL(panelURL, c.WorkspaceID, c.OrganizationID)
		if c.Threshold == Expired {
			d.para(fmt.Sprintf("El certificado de %s de «%s», en el espacio «%s» de Sello Sign, venció el %s y sigue activo: Sello Sign ya no firma con él.",
				env, c.OrganizationName, space, formatDateTime(c.NotAfter)))
			d.link("Para volver a firmar, suba un certificado nuevo", url)
			d.footer(space)
			return d.message(fmt.Sprintf("El certificado de %s de «%s» venció", env, c.OrganizationName))
		}
		d.para(fmt.Sprintf("El certificado de %s de «%s», en el espacio «%s» de Sello Sign, vence el %s. Desde entonces, Sello Sign no firmará con él.",
			env, c.OrganizationName, space, formatDateTime(c.NotAfter)))
		d.link("Para seguir firmando, suba el certificado nuevo antes de esa fecha", url)
		d.footer(space)
		return d.message(fmt.Sprintf("El certificado de %s de «%s» vence el %s", env, c.OrganizationName, formatDate(c.NotAfter)))
	}

	expired := 0
	lines := make([][2]string, len(items))
	for i, c := range items {
		when := "vence el " + formatDateTime(c.NotAfter)
		if c.Threshold == Expired {
			expired++
			when = "venció el " + formatDateTime(c.NotAfter)
		}
		lines[i] = [2]string{fmt.Sprintf("«%s», %s: %s.", c.OrganizationName, environmentName(c.Environment), when),
			organizationURL(panelURL, c.WorkspaceID, c.OrganizationID)}
	}
	var subject string
	switch expired {
	case 0:
		subject = fmt.Sprintf("Certificados por vencer en el espacio «%s»", space)
		d.para(fmt.Sprintf("Estos certificados del espacio «%s» de Sello Sign vencen pronto. Sello Sign no firma con un certificado vencido: para seguir firmando, suba el nuevo de cada organización antes de que venza.", space))
	case len(items):
		subject = fmt.Sprintf("Certificados vencidos en el espacio «%s»", space)
		d.para(fmt.Sprintf("Estos certificados del espacio «%s» de Sello Sign vencieron y siguen activos: Sello Sign ya no firma con ellos. Para volver a firmar, suba el nuevo de cada organización.", space))
	default:
		subject = fmt.Sprintf("Certificados vencidos y por vencer en el espacio «%s»", space)
		d.para(fmt.Sprintf("Estos certificados del espacio «%s» de Sello Sign vencen pronto o ya vencieron. Sello Sign no firma con un certificado vencido: para seguir firmando, suba el nuevo de cada organización.", space))
	}
	d.list(lines)
	d.footer(space)
	return d.message(subject)
}

// usageMessage is the notice of a space's usage in an environment reaching
// threshold in month ("YYYY-MM").
func usageMessage(panelURL string, u store.SpaceUsage, threshold, month string) mail.Message {
	quota, cap := u.Quota, u.Cap
	if quota == nil {
		quota = cap
	}
	if cap == nil {
		cap = quota
	}
	q, c := formatNumber(int64(*quota)), formatNumber(int64(*cap))
	env := environmentName(u.Environment)
	resets := formatDate(nextMonth(month))
	// What the space has this month: its plan's or, without one, the
	// install's.
	allowance := "que incluye su plan"
	if u.Plan == nil {
		allowance = "que le permite la instalación"
	}
	var d doc
	var subject string
	switch threshold {
	case QuotaWarning:
		subject = fmt.Sprintf("El espacio «%s» usó el %d %% de sus firmas de %s del mes", u.Name, WarningPercent, env)
		p := fmt.Sprintf("El espacio «%s» de Sello Sign usó el %d %% de las %s firmas de %s de este mes %s.",
			u.Name, WarningPercent, q, env, allowance)
		switch {
		case *cap <= *quota:
			p += fmt.Sprintf(" Al llegar a %s, Sello Sign rechazará las siguientes hasta el %s, cuando la cuenta empieza de nuevo.", q, resets)
		case u.Environment == model.Prod:
			p += fmt.Sprintf(" Después de %s, cada firma de producción se cobra como excedente, hasta un tope de %s.", q, c)
		default:
			p += fmt.Sprintf(" Después de %s, puede firmar hasta %s; luego, Sello Sign rechazará las siguientes hasta el %s.", q, c, resets)
		}
		d.para(p)
	case QuotaReached:
		subject = fmt.Sprintf("El espacio «%s» usó todas las firmas de %s incluidas este mes", u.Name, env)
		p := fmt.Sprintf("El espacio «%s» de Sello Sign llegó a las %s firmas de %s de este mes %s.", u.Name, q, env, allowance)
		if u.Environment == model.Prod {
			p += fmt.Sprintf(" Las siguientes se cobran como excedente, hasta un tope de %s; después, Sello Sign las rechazará hasta el %s.", c, resets)
		} else {
			p += fmt.Sprintf(" Puede firmar hasta %s; después, Sello Sign rechazará las siguientes hasta el %s.", c, resets)
		}
		d.para(p)
	default:
		subject = fmt.Sprintf("El espacio «%s» alcanzó el límite de firmas de %s del mes", u.Name, env)
		p := fmt.Sprintf("Alcanzó el límite de %s firmas de %s de este mes en el espacio «%s» de Sello Sign. Sello Sign rechaza las siguientes hasta el %s, cuando la cuenta empieza de nuevo.",
			c, env, u.Name, resets)
		if u.Plan != nil {
			p += " Para firmar más este mes, puede cambiar el plan del espacio."
		}
		d.para(p)
	}
	d.link("Vea el uso del espacio en Sello Sign", panelURL+"/w/"+u.WorkspaceID)
	d.footer(u.Name)
	return d.message(subject)
}

// nextMonth is the first day of the month after month ("YYYY-MM"), when
// the monthly counts start again.
func nextMonth(month string) time.Time {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}
	}
	return t.AddDate(0, 1, 0)
}
