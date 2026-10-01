package app

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/valium69mg/finances-app/backend/internal/expenserequests/domain"
)

// The emails are best-effort: a failure is logged and never fails the call
// (the request is already stored or decided), like the budget feedback.

func (s *Service) link() string {
	return strings.TrimRight(s.AppBaseURL, "/") + "/peticiones"
}

func money(r domain.Request) string { return "$" + r.Amount.StringFixed(2) + " MXN" }

// send delivers one message with its own bounded context, detached from the
// request so a client that hangs up does not drop the email.
func (s *Service) send(ctx context.Context, op, to, subject string, lines []string) {
	if s.Mailer == nil || to == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mailTimeout)
	defer cancel()
	var text, markup strings.Builder
	for _, l := range lines {
		text.WriteString(l + "\n")
		markup.WriteString("<p>" + html.EscapeString(l) + "</p>")
	}
	if err := s.Mailer.Send(ctx, to, subject, markup.String(), text.String()); err != nil {
		s.Logger.Warn("expense request email failed", "op", op, "error", err)
	}
}

// notifyOwners tells every active owner that a request arrived.
func (s *Service) notifyOwners(ctx context.Context, r domain.Request) {
	owners, err := s.Repo.OwnerEmails(ctx)
	if err != nil {
		s.Logger.Warn("expense request email failed: owners could not be read", "error", err)
		return
	}
	lines := []string{
		"Hay una nueva petición de gasto.",
		fmt.Sprintf("%s pide %s: %s.", r.RequesterEmail, money(r), r.Description),
		"Revísala en " + s.link(),
	}
	for _, to := range owners {
		s.send(ctx, "notify owner", to, "Nueva petición de gasto", lines)
	}
}

// notifyRequester tells the requester how the owner decided.
func (s *Service) notifyRequester(ctx context.Context, r domain.Request) {
	var subject string
	lines := []string{fmt.Sprintf("Tu petición de %s (%s) fue revisada.", money(r), r.Description)}
	switch r.Status {
	case domain.StatusApproved:
		subject = "Tu petición de gasto fue aprobada"
		if r.ResultKind == domain.DestinationFuture {
			lines = append(lines, "Aprobada: se movió a gastos futuros.")
		} else {
			lines = append(lines, "Aprobada: se registró como gasto.")
		}
	case domain.StatusRejected:
		subject = "Tu petición de gasto fue rechazada"
		lines = append(lines, "Rechazada. Comentario: "+r.DecisionComment)
	default:
		return
	}
	lines = append(lines, "Ver tus peticiones: "+s.link())
	s.send(ctx, "notify requester", r.RequesterEmail, subject, lines)
}

// notifyReverted tells the requester that the approval was undone and the
// request is pending again.
func (s *Service) notifyReverted(ctx context.Context, r domain.Request) {
	lines := []string{
		fmt.Sprintf("Tu petición de %s (%s) volvió a solicitada.", money(r), r.Description),
		"Se deshizo la aprobación anterior; el titular la revisará de nuevo.",
		"Ver tus peticiones: " + s.link(),
	}
	s.send(ctx, "notify reverted", r.RequesterEmail, "Tu petición de gasto volvió a solicitada", lines)
}
