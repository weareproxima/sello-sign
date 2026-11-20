package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// exportFormat names the shape of an export, for whoever reads it later.
const exportFormat = "sello-sign-export/1"

type exportedOrganizationView struct {
	organizationView
	// Certificates are all of it, public data only; null if a delegation
	// doesn't let the space read them.
	Certificates []certificateView `json:"certificates"`
	// ActiveCertificates are what organizationView calls certificates.
	ActiveCertificates activeCertificates `json:"active_certificates"`
	// Delegations are those it gave, for an organization the space owns;
	// null for a delegated one.
	Delegations []delegationView `json:"delegations"`
}

type exportedKeyView struct {
	apiKeyView
	CreatedBy *string    `json:"created_by"`
	RevokedAt *time.Time `json:"revoked_at"`
}

// exportView is an export without its log, which is streamed after it.
type exportView struct {
	Format        string                     `json:"format"`
	ExportedAt    time.Time                  `json:"exported_at"`
	Workspace     workspaceView              `json:"workspace"`
	Organizations []exportedOrganizationView `json:"organizations"`
	APIKeys       []exportedKeyView          `json:"api_keys"`
	Members       []memberView               `json:"members"`
	Invitations   []invitationView           `json:"invitations"`
	Usage         []usageLineView            `json:"usage"`
}

// exportWorkspace handles GET /workspaces/{w}/export: everything the space
// has, as one JSON made at the moment and never kept: the space, its
// organizations (with their certificates' public data, never their
// material, and their delegations), its keys (never their hash), its
// members, its open invitations (never their token), its usage, its audit
// log and its signing log as it stands, within their retention. Owners and
// admins. The export itself is recorded in the audit log before it's
// sent: if that fails, nothing is.
func (s *Server) exportWorkspace(w http.ResponseWriter, r *http.Request, c *call) {
	if c.user() == nil {
		writeProblem(w, errTokenOnly)
		return
	}
	if !c.role.Manages() {
		writeProblem(w, forbidden("Solo quien administra el espacio puede exportar sus datos."))
		return
	}
	ctx := r.Context()
	now := s.Now().UTC()
	out := exportView{Format: exportFormat, ExportedAt: now, Workspace: workspaceOf(c.ws, "")}

	orgs, err := s.Store.AllOrganizations(ctx, c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	views, err := s.organizationsOf(ctx, orgs)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Organizations = make([]exportedOrganizationView, len(orgs))
	for i, o := range orgs {
		e := exportedOrganizationView{organizationView: views[i], ActiveCertificates: views[i].Certificates}
		if o.Reach.Allows(model.ScopeCertificatesRead) {
			certs, err := s.Store.Certificates(ctx, o.ID)
			if err != nil {
				internalError(w, r, err)
				return
			}
			e.Certificates = nonNil(mapSlice(certs, certificateOf))
		}
		if o.Reach.Relation == model.Owned {
			ds, err := s.Store.Delegations(ctx, o.ID)
			if err != nil {
				internalError(w, r, err)
				return
			}
			e.Delegations = nonNil(mapSlice(ds, delegationOf))
		}
		out.Organizations[i] = e
	}

	keys, err := s.Store.AllAPIKeys(ctx, c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.APIKeys = nonNil(mapSlice(keys, func(k store.ExportedKey) exportedKeyView {
		return exportedKeyView{apiKeyView: apiKeyOf(k.APIKey), CreatedBy: k.CreatedBy, RevokedAt: utcp(k.RevokedAt)}
	}))
	members, err := s.Store.Members(ctx, c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Members = nonNil(mapSlice(members, func(m store.Member) memberView {
		return memberView{UserID: m.UserID, Email: m.Email, Name: m.Name, Role: m.Role}
	}))
	invitations, err := s.Store.Invitations(ctx, c.ws.ID)
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Invitations = nonNil(mapSlice(invitations, s.invitationOf))
	usage, err := s.Store.Usage(ctx, c.ws.ID, "0000-01", "9999-12")
	if err != nil {
		internalError(w, r, err)
		return
	}
	out.Usage = nonNil(mapSlice(usage, func(l store.UsageLine) usageLineView {
		return usageLineView{Month: l.Month, Environment: l.Environment, Signatures: l.Signatures}
	}))

	head, err := json.Marshal(out)
	if err != nil {
		internalError(w, r, err)
		return
	}
	if err := s.Store.RecordEvent(ctx, c.ws.ID, audit.WorkspaceExported, audit.TargetWorkspace, c.ws.ID, c.ws.Name, nil); err != nil {
		internalError(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	h.Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sello-sign-%s-%s.json"`, c.ws.ID, now.Format("2006-01-02")))
	w.WriteHeader(http.StatusOK)

	// The logs go last, streamed: they can be long.
	bw := bufio.NewWriterSize(w, 64<<10)
	bw.Write(bytes.TrimSuffix(head, []byte("}")))
	events, signatures := 0, 0
	bw.WriteString(`,"audit_events":[`)
	err = s.Store.EachAuditEvent(ctx, c.ws.ID, func(e store.AuditEvent) error {
		return writeItem(bw, &events, auditEventOf(e))
	})
	if err == nil {
		bw.WriteString(`],"signatures":[`)
		err = s.Store.EachSignature(ctx, c.ws.ID, func(e store.SignatureEntry) error {
			return writeItem(bw, &signatures, signatureOf(e))
		})
	}
	if err == nil {
		bw.WriteString("]}\n")
		err = bw.Flush()
	}
	if err != nil {
		// The status is gone: the download is cut instead of ending as a
		// JSON that looks whole.
		slog.ErrorContext(ctx, "export failed", "workspace_id", c.ws.ID, "error", err)
		panic(http.ErrAbortHandler)
	}
	slog.InfoContext(ctx, "workspace exported", "workspace_id", c.ws.ID, "audit_events", events, "signatures", signatures)
}

// writeItem writes v as the next item of a JSON array that already has n.
func writeItem(bw *bufio.Writer, n *int, v any) error {
	line, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if *n > 0 {
		bw.WriteByte(',')
	}
	*n++
	_, err = bw.Write(line)
	return err
}
