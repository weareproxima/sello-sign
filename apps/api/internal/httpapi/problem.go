package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"proxima.la/sello/sign/internal/model"
)

// Problem is an error as the API answers it: application/problem+json (RFC
// 9457), with a stable slug as its type.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
	// The extensions of the limits' problems (rate-limited,
	// monthly-limit-reached, organization-limit-reached).
	Limit       *int              `json:"limit,omitempty"`
	AppliesTo   string            `json:"applies_to,omitempty"`
	Environment model.Environment `json:"environment,omitempty"`
	ResetsAt    *time.Time        `json:"resets_at,omitempty"`
}

// UnknownOrganizationType is the type of the 404 billing reads as «Sign
// has no space of this organization»: the URI Sello's billing already
// recognizes, the only type that isn't a slug.
const UnknownOrganizationType = "https://sello.tax/problems/sign-unknown-organization"

func (p *Problem) Error() string { return p.Type + ": " + p.Detail }

func problem(status int, slug, detail string) *Problem {
	return &Problem{Type: slug, Title: http.StatusText(status), Status: status, Detail: detail}
}

// The problems more than one route answers.
var (
	errUnauthorized      = problem(http.StatusUnauthorized, "unauthorized", "Falta una credencial válida: una clave en X-API-Key o el token del panel en Authorization.")
	errTokenExpired      = problem(http.StatusUnauthorized, "token-expired", "El token venció. Pida otro al panel.")
	errKeysUnavailable   = problem(http.StatusServiceUnavailable, "panel-keys-unavailable", "No se pudieron leer las claves del panel para verificar el token. Vuelva a intentarlo.")
	errWorkspaceNotFound = problem(http.StatusNotFound, "workspace-not-found", "No existe el espacio, o no tiene acceso a él.")
	errOrgNotFound       = problem(http.StatusNotFound, "organization-not-found", "No existe la organización, o no está al alcance de esta credencial.")
	errNotFound          = problem(http.StatusNotFound, "not-found", "No existe la ruta.")
	errInternal          = problem(http.StatusInternalServerError, "internal", "No se pudo completar la solicitud. Vuelva a intentarlo.")
	errTokenOnly         = problem(http.StatusForbidden, "forbidden", "Esta ruta es solo para el panel: pide el token de una persona, no una clave.")
	errKeyOnly           = problem(http.StatusForbidden, "forbidden", "Esta ruta pide una clave de API en X-API-Key.")
	errNotOwner          = problem(http.StatusForbidden, "not-organization-owner", "Solo el espacio dueño de la organización puede hacer esto.")
	errNotCloud          = problem(http.StatusNotFound, "not-cloud", "Esta ruta solo existe en la nube de Sello Sign.")
	errCountryNotEnabled = problem(http.StatusConflict, "country-not-enabled", "Este servidor no firma documentos del país de la organización.")
)

func forbidden(detail string) *Problem { return problem(http.StatusForbidden, "forbidden", detail) }

func invalid(detail string) *Problem {
	return problem(http.StatusBadRequest, "invalid-request", detail)
}

func writeProblem(w http.ResponseWriter, p *Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// list is every list's body.
type list[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"next_cursor"`
}

func listOf[T any](items []T, next *string) list[T] {
	if items == nil {
		items = []T{}
	}
	return list[T]{Items: items, NextCursor: next}
}
