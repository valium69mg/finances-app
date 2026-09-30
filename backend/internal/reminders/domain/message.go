package domain

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// Message is the subject and the two bodies of an email, in Spanish.
type Message struct {
	Subject string
	HTML    string
	Text    string
}

// Compose builds the message of a bill email. appURL is the link to the app.
func Compose(e Email, appURL string) Message {
	switch e.Kind {
	case KindTaxDueSoon, KindTaxDueToday, KindTaxOverdue:
		return taxMessage(e, appURL)
	case KindSalaryInvoice:
		return salaryMessage(e.Month, appURL)
	case KindWeekly:
		return weeklyMessage(e.Items, appURL)
	case KindBillOverdue:
		return billMessage("Pago vencido: ", e.Items[0], appURL)
	default:
		return billMessage("Pago próximo: ", e.Items[0], appURL)
	}
}

// DiskMessage builds the disk alert message.
func DiskMessage(usedPct float64, thresholdPct int) Message {
	text := fmt.Sprintf("El disco de datos está al %.0f%% de su capacidad (el aviso se activa al %d%%).", usedPct, thresholdPct)
	return Message{
		Subject: fmt.Sprintf("Alerta: el disco de datos está al %.0f%%", usedPct),
		Text: "Hola,\n\n" + text + "\nConviene ampliar el volumen antes de que se llene. " +
			"Si sigue por encima del umbral, este aviso se repite cada 7 días.\n",
		HTML: "<p>Hola,</p><p>" + html.EscapeString(text) + "</p>" +
			"<p>Conviene ampliar el volumen antes de que se llene. " +
			"Si sigue por encima del umbral, este aviso se repite cada 7 días.</p>",
	}
}

var monthNames = [...]string{
	"Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
	"Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre",
}

// monthLabel renders a YYYY-MM month as "Septiembre 2026"; an unparsable value
// is kept.
func monthLabel(month string) string {
	t, err := time.Parse("2006-01", month)
	if err != nil {
		return month
	}
	return monthNames[t.Month()-1] + " " + t.Format("2006")
}

// taxMessage builds the email of a pending tax filing: which period, the
// deadline and how many days are left or have passed.
func taxMessage(e Email, appURL string) Message {
	t := e.Tax
	label := monthLabel(t.Period)
	var subject, lead string
	switch e.Kind {
	case KindTaxOverdue:
		subject = "Declaración vencida: " + label
		lead = "Tu declaración mensual de " + label + " está vencida y aún no la has registrado."
	case KindTaxDueToday:
		subject = "Declaración vence hoy: " + label
		lead = "Tu declaración mensual de " + label + " vence hoy."
	default:
		subject = "Declaración próxima a vencer: " + label
		lead = "Tu declaración mensual de " + label + " está próxima a vencer."
	}
	detail := "Fecha límite: " + dateText(t.DueDate) + " (" + whenText(t.DaysUntilDue) + ")."
	hint := "Presenta la declaración en el portal del SAT y regístrala en la app."
	return Message{
		Subject: subject,
		Text:    "Hola,\n\n" + lead + "\n" + detail + "\n" + hint + "\n\nAbre la app: " + appURL + "\n",
		HTML: "<p>Hola,</p><p>" + html.EscapeString(lead) + "</p><p>" + html.EscapeString(detail) + "</p><p>" +
			html.EscapeString(hint) + "</p>" + linkHTML(appURL),
	}
}

// salaryMessage builds the end-of-month reminder to issue the salary invoice.
func salaryMessage(month, appURL string) Message {
	label := monthLabel(month)
	lead := "Hoy es el último día del mes: emite la factura de tu sueldo de " + label + "."
	return Message{
		Subject: "Emite la factura de tu sueldo de " + label,
		Text:    "Hola,\n\n" + lead + "\n\nAbre la app: " + appURL + "\n",
		HTML:    "<p>Hola,</p><p>" + html.EscapeString(lead) + "</p>" + linkHTML(appURL),
	}
}

func billMessage(subjectPrefix string, it Item, appURL string) Message {
	lead := "Tu pago está próximo a vencer."
	if it.Overdue {
		lead = "Tienes un pago vencido."
	}
	text := "Hola,\n\n" + lead + "\n\n" + itemText(it) + "\n\nAbre la app: " + appURL + "\n"
	htmlBody := "<p>Hola,</p><p>" + lead + "</p><ul><li>" + itemHTML(it) + "</li></ul>" + linkHTML(appURL)
	return Message{Subject: subjectPrefix + it.Name, Text: text, HTML: htmlBody}
}

func weeklyMessage(items []Item, appURL string) Message {
	var overdue, upcoming []Item
	for _, it := range items {
		if it.Overdue {
			overdue = append(overdue, it)
		} else {
			upcoming = append(upcoming, it)
		}
	}
	var text, htmlBody strings.Builder
	text.WriteString("Hola,\n\nEste es tu resumen semanal de pagos.\n")
	htmlBody.WriteString("<p>Hola,</p><p>Este es tu resumen semanal de pagos.</p>")
	section := func(title string, list []Item) {
		if len(list) == 0 {
			return
		}
		text.WriteString("\n" + title + "\n")
		htmlBody.WriteString("<p><strong>" + title + "</strong></p><ul>")
		for _, it := range list {
			text.WriteString("- " + itemText(it) + "\n")
			htmlBody.WriteString("<li>" + itemHTML(it) + "</li>")
		}
		htmlBody.WriteString("</ul>")
	}
	section("Vencidos", overdue)
	section("Próximos 7 días", upcoming)
	text.WriteString("\nAbre la app: " + appURL + "\n")
	htmlBody.WriteString(linkHTML(appURL))
	return Message{Subject: "Resumen semanal de pagos", Text: text.String(), HTML: htmlBody.String()}
}

func linkHTML(appURL string) string {
	return `<p><a href="` + html.EscapeString(appURL) + `">Abrir la app</a></p>`
}

// itemText is the one-line description of a payment, for the text bodies.
func itemText(it Item) string {
	return it.Name + ": " + amountText(it) + ", vence el " + dateText(it.DueDate) + " (" + whenText(it.DaysUntilDue) + ")"
}

// itemHTML is itemText with the bill name escaped.
func itemHTML(it Item) string {
	return "<strong>" + html.EscapeString(it.Name) + "</strong>: " + html.EscapeString(amountText(it)) +
		", vence el " + dateText(it.DueDate) + " (" + whenText(it.DaysUntilDue) + ")"
}

// amountText shows the amount and its currency, or "monto variable" when the
// bill has no fixed amount.
func amountText(it Item) string {
	if it.Amount == "" {
		return "monto variable"
	}
	return it.Amount + " " + it.Currency
}

// dateText renders a YYYY-MM-DD date as DD/MM/YYYY; an unparsable value is kept.
func dateText(date string) string {
	t, err := time.Parse(dateLayout, date)
	if err != nil {
		return date
	}
	return t.Format("02/01/2006")
}

// whenText says how far the due date is: today, tomorrow, in N days or N days ago.
func whenText(days int) string {
	switch {
	case days == 0:
		return "hoy"
	case days == 1:
		return "mañana"
	case days > 1:
		return fmt.Sprintf("en %d días", days)
	case days == -1:
		return "venció hace 1 día"
	default:
		return fmt.Sprintf("venció hace %d días", -days)
	}
}
