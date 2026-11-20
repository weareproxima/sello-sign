package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"time"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/store"
)

type auditActorView struct {
	Type audit.ActorType `json:"type"`
	// ID is the person's `sub`, the key's id or the space's id.
	ID    *string `json:"id"`
	Email *string `json:"email"`
	// Name is the person's, the key's or the space's, at that moment.
	Name *string `json:"name"`
	// Via is "mcp" for a key through the MCP server.
	Via *string `json:"via"`
}

type auditTargetView struct {
	Type  audit.TargetType `json:"type"`
	ID    *string          `json:"id"`
	Label string           `json:"label"`
}

type auditEventView struct {
	ID        string          `json:"id"`
	Action    audit.Action    `json:"action"`
	Actor     auditActorView  `json:"actor"`
	Target    auditTargetView `json:"target"`
	Details   json.RawMessage `json:"details"`
	CreatedAt time.Time       `json:"created_at"`
}

func orNull(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func auditEventOf(e store.AuditEvent) auditEventView {
	details := e.Details
	if len(details) == 0 {
		details = json.RawMessage("{}")
	}
	return auditEventView{
		ID: e.ID, Action: e.Action,
		Actor: auditActorView{Type: e.Actor.Type, ID: orNull(e.Actor.ID), Email: orNull(e.Actor.Email),
			Name: orNull(e.Actor.Name), Via: orNull(e.Actor.Via)},
		Target:    auditTargetView{Type: e.TargetType, ID: e.TargetID, Label: e.TargetLabel},
		Details:   details,
		CreatedAt: utc(e.CreatedAt),
	}
}

// listAuditEvents handles GET /workspaces/{w}/audit-events: the space's
// audit log, newest first, for its owners and admins, with the token. Not
// for a key, and so not in the MCP server: it names people (members and
// invited addresses), which no key reads.
func (s *Server) listAuditEvents(w http.ResponseWriter, r *http.Request, c *call) {
	if c.user() == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio ve su actividad."))
		return
	}
	pg, p := page(r)
	if p != nil {
		writeProblem(w, p)
		return
	}
	q := r.URL.Query()
	var f store.AuditFilter
	if v := q.Get("action"); v != "" {
		if !slices.Contains(audit.Actions, audit.Action(v)) {
			writeProblem(w, invalid("«action» no es una de las acciones del registro de actividad."))
			return
		}
		f.Action = audit.Action(v)
	}
	if v := q.Get("target_type"); v != "" {
		if !slices.Contains(audit.TargetTypes, audit.TargetType(v)) {
			writeProblem(w, invalid("«target_type» debe ser workspace, member, invitation, organization, certificate, delegation o api_key."))
			return
		}
		f.TargetType = audit.TargetType(v)
	}
	if v := q.Get("actor_type"); v != "" {
		if !slices.Contains(audit.ActorTypes, audit.ActorType(v)) {
			writeProblem(w, invalid("«actor_type» debe ser user, api_key, billing, console, workspace o system."))
			return
		}
		f.ActorType = audit.ActorType(v)
	}
	for name, dst := range map[string]*string{"actor_id": &f.ActorID, "target_id": &f.TargetID} {
		if v := q.Get(name); v != "" {
			if len(v) > 200 {
				writeProblem(w, invalid("«"+name+"» es demasiado largo."))
				return
			}
			*dst = v
		}
	}
	for name, dst := range map[string]**time.Time{"from": &f.From, "to": &f.To} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeProblem(w, invalid("«"+name+"» debe ser una fecha RFC 3339 (2026-11-19T00:00:00Z)."))
				return
			}
			*dst = &t
		}
	}
	items, next, err := s.Store.AuditEvents(r.Context(), c.ws.ID, f, pg)
	if err != nil {
		internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, listOf(mapSlice(items, auditEventOf), next))
}
