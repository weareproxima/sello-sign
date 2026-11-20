// Package audit is the vocabulary of a space's audit log, without I/O: who
// acted (Actor), what they did (Action), on what (TargetType) and how long
// it's kept (RetentionDays). The store writes each event in the same
// transaction as the change it records (internal/store/audit.go).
//
// The actor travels in the request's context: the HTTP layer puts it there
// once per request, from the credential (WithActor), and every change the
// store makes in that request records it. A change made without one (a
// test, a job) records ActorSystem. Signing and verifying are never
// recorded: the signing log already has every signature.
package audit

import (
	"context"
	"time"
)

// RetentionDays is how long an event is kept, whatever the space's plan:
// the retention job deletes it after that, hourly. Events also go with
// their space.
const RetentionDays = 365

// Retention is RetentionDays as a duration.
const Retention = RetentionDays * 24 * time.Hour

// ActorType is who acted.
type ActorType string

const (
	// ActorUser is a person, with the panel's token.
	ActorUser ActorType = "user"
	// ActorAPIKey is an API key, by the REST API or the MCP server.
	ActorAPIKey ActorType = "api_key"
	// ActorBilling is Sello's billing pushing a space's state (the cloud).
	ActorBilling ActorType = "billing"
	// ActorConsole is the API's console (`api workspace create`…), run by
	// whoever runs the server.
	ActorConsole ActorType = "console"
	// ActorWorkspace is another space, on what it did to this one's
	// organizations or delegations: never its person or its key, which
	// this space has no business knowing.
	ActorWorkspace ActorType = "workspace"
	// ActorSystem is the API itself: a change without any of the above.
	ActorSystem ActorType = "system"
)

// ActorTypes are all of them, in the contract's order.
var ActorTypes = []ActorType{ActorUser, ActorAPIKey, ActorBilling, ActorConsole, ActorWorkspace, ActorSystem}

// ViaMCP marks a key that acted through the MCP server (POST /mcp).
const ViaMCP = "mcp"

// Actor is who made a change, as the API knew them at that moment. Never a
// credential: a person's `sub`, e-mail and name come from their token; a
// key is its id (the store reads its name); a space, its id (and name).
type Actor struct {
	Type ActorType
	// ID is the person's `sub`, the key's id or the space's id; "" for
	// billing, the console and the system.
	ID string
	// Email is a person's.
	Email string
	// Name is a person's; the store fills a key's and a space's.
	Name string
	// Via is ViaMCP for a key through the MCP server.
	Via string
}

// System is the actor of a change made without one.
var System = Actor{Type: ActorSystem}

type actorKey struct{}

// WithActor makes a the actor of the changes made with ctx.
func WithActor(ctx context.Context, a Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, a)
}

// ActorFrom is the actor WithActor put in ctx, or System.
func ActorFrom(ctx context.Context) Actor {
	if a, ok := ctx.Value(actorKey{}).(Actor); ok {
		return a
	}
	return System
}

// Space is the actor that is another space: the only thing a space is told
// of who, from another one, changed what it holds.
func Space(id string) Actor { return Actor{Type: ActorWorkspace, ID: id} }

// TargetType is what an event is about.
type TargetType string

const (
	TargetWorkspace    TargetType = "workspace"
	TargetMember       TargetType = "member"
	TargetInvitation   TargetType = "invitation"
	TargetOrganization TargetType = "organization"
	TargetCertificate  TargetType = "certificate"
	TargetDelegation   TargetType = "delegation"
	TargetAPIKey       TargetType = "api_key"
)

// TargetTypes are all of them, in the contract's order.
var TargetTypes = []TargetType{
	TargetWorkspace, TargetMember, TargetInvitation, TargetOrganization, TargetCertificate, TargetDelegation, TargetAPIKey,
}

// Action is what was done: `<target>.<verb>`, stable (the panel words each
// one, and an export keeps them).
type Action string

const (
	// The space: created; its name, default country or own signing rate
	// changed; its limits changed by the console (a space without a plan)
	// or its plan by billing; closed and reopened by billing; its data
	// exported.
	WorkspaceCreated       Action = "workspace.created"
	WorkspaceUpdated       Action = "workspace.updated"
	WorkspaceLimitsChanged Action = "workspace.limits_changed"
	WorkspacePlanChanged   Action = "workspace.plan_changed"
	WorkspaceClosed        Action = "workspace.closed"
	WorkspaceReopened      Action = "workspace.reopened"
	WorkspaceExported      Action = "workspace.exported"

	// Its members: added by the console, a role changed, taken out, or
	// gone by themselves (also by deleting their account).
	MemberAdded       Action = "member.added"
	MemberRoleChanged Action = "member.role_changed"
	MemberRemoved     Action = "member.removed"
	MemberLeft        Action = "member.left"

	// Its invitations: sent, sent again with a new link, revoked, accepted.
	InvitationCreated  Action = "invitation.created"
	InvitationRenewed  Action = "invitation.renewed"
	InvitationRevoked  Action = "invitation.revoked"
	InvitationAccepted Action = "invitation.accepted"

	// Its organizations: created, changed, deleted; brought from Sello's
	// platform space («Usar en Sello Sign»), and given back to it when
	// the space closes or is deleted.
	OrganizationCreated  Action = "organization.created"
	OrganizationUpdated  Action = "organization.updated"
	OrganizationDeleted  Action = "organization.deleted"
	OrganizationClaimed  Action = "organization.claimed"
	OrganizationReturned Action = "organization.returned"

	// The certificates of an organization.
	CertificateUploaded    Action = "certificate.uploaded"
	CertificateActivated   Action = "certificate.activated"
	CertificateDeactivated Action = "certificate.deactivated"
	CertificateDeleted     Action = "certificate.deleted"

	// A delegation, in the space that gives it and in the one that gets it.
	DelegationGranted Action = "delegation.granted"
	DelegationRemoved Action = "delegation.removed"

	// Its API keys.
	APIKeyCreated Action = "api_key.created"
	APIKeyRevoked Action = "api_key.revoked"
)

// Actions are all of them, in the contract's order.
var Actions = []Action{
	WorkspaceCreated, WorkspaceUpdated, WorkspaceLimitsChanged, WorkspacePlanChanged, WorkspaceClosed, WorkspaceReopened,
	WorkspaceExported,
	MemberAdded, MemberRoleChanged, MemberRemoved, MemberLeft,
	InvitationCreated, InvitationRenewed, InvitationRevoked, InvitationAccepted,
	OrganizationCreated, OrganizationUpdated, OrganizationDeleted, OrganizationClaimed, OrganizationReturned,
	CertificateUploaded, CertificateActivated, CertificateDeactivated, CertificateDeleted,
	DelegationGranted, DelegationRemoved,
	APIKeyCreated, APIKeyRevoked,
}

// Change is a field's value before and after.
type Change struct {
	From any `json:"from"`
	To   any `json:"to"`
}

// Changes collects what changed: Set records a field only if its value
// did change.
type Changes map[string]Change

// Set records field from → to, if they differ. Pointers compare by what
// they point to.
func (c Changes) Set(field string, from, to any) {
	from, to = deref(from), deref(to)
	if from == to {
		return
	}
	c[field] = Change{From: from, To: to}
}

func deref(v any) any {
	switch p := v.(type) {
	case *string:
		if p == nil {
			return nil
		}
		return *p
	case *int:
		if p == nil {
			return nil
		}
		return *p
	case *int64:
		if p == nil {
			return nil
		}
		return *p
	case *bool:
		if p == nil {
			return nil
		}
		return *p
	}
	return v
}
