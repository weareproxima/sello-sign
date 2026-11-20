package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"proxima.la/sello/sign/internal/audit"
	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

// consoleContext is the context of a console command: what it changes is
// recorded in the audit log as the console's.
func consoleContext() context.Context {
	return audit.WithActor(context.Background(), audit.Actor{Type: audit.ActorConsole})
}

// workspaceCreate creates a space: one to try the API locally, a
// self-hosted install's first one, or Sello's platform space in the cloud
// (SIGN_PLATFORM_WORKSPACE_ID).
func workspaceCreate(args []string) error {
	fs := flag.NewFlagSet("workspace create", flag.ContinueOnError)
	name := fs.String("name", "", "the space's name")
	country := fs.String("country", "", "its default country (ISO 3166-1 alpha-2)")
	owner := fs.String("owner", "", "the panel user id (the token's sub) who owns it")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*name) == "" {
		return errors.New("-name is required")
	}
	var cc *string
	if *country != "" {
		up := strings.ToUpper(*country)
		if !model.ValidCountry(up) {
			return errors.New("-country must be ISO 3166-1 alpha-2")
		}
		cc = &up
	}
	ctx := consoleContext()
	db, done, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer done()
	ws, err := db.CreateWorkspace(ctx, strings.TrimSpace(*name), cc, *owner)
	if err != nil {
		return err
	}
	fmt.Println(ws.ID)
	return nil
}

// limitFlag is a limit of the console: a number, or "none" to remove it.
type limitFlag struct {
	set   bool
	value *int
}

func (l *limitFlag) String() string { return "" }

func (l *limitFlag) Set(v string) error {
	l.set = true
	if strings.EqualFold(v, "none") {
		l.value = nil
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 || n > 1<<31-1 {
		return errors.New(`a whole number, or "none"`)
	}
	l.value = &n
	return nil
}

func (l *limitFlag) patch() **int {
	if !l.set {
		return nil
	}
	return &l.value
}

// workspaceLimits sets the limits of a space without a plan: a self-hosted
// install's (with none, it has no limits but SIGN_RATE_LIMIT_PER_MINUTE and
// SIGN_LOG_RETENTION_DAYS), or one made with the console. In the cloud,
// billing pushes them.
func workspaceLimits(args []string) error {
	fs := flag.NewFlagSet("workspace limits", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "the space's id")
	var orgs, prod, test, members, rpm, retention limitFlag
	fs.Var(&orgs, "organizations", "the organizations it may own")
	fs.Var(&prod, "prod", "the production signatures a month")
	fs.Var(&test, "test", "the test signatures a month")
	fs.Var(&members, "members", "its members")
	fs.Var(&rpm, "rpm", "the signatures a minute")
	fs.Var(&retention, "retention", "the days the signing log keeps an entry")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if !ids.Valid(*workspace) {
		return errors.New("-workspace (an id) is required")
	}
	for name, f := range map[string]*limitFlag{"rpm": &rpm, "retention": &retention} {
		if f.value != nil && *f.value == 0 {
			return fmt.Errorf("-%s must be at least 1, or none", name)
		}
	}
	ctx := consoleContext()
	db, done, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer done()
	ws, err := db.SetLimits(ctx, *workspace, store.LimitsPatch{
		Organizations: orgs.patch(), ProdSignatures: prod.patch(), TestSignatures: test.patch(),
		Members: members.patch(), RequestsPerMinute: rpm.patch(), RetentionDays: retention.patch(),
	})
	if errors.Is(err, store.ErrHasPlan) {
		return errors.New("the space has a plan: billing pushes its limits")
	}
	if err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	show := func(p *int) string {
		if p == nil {
			return "none"
		}
		return strconv.Itoa(*p)
	}
	l := ws.Limits
	fmt.Printf("organizations=%s prod=%s test=%s members=%s rpm=%s retention=%s\n",
		show(l.Organizations), show(l.ProdSignatures), show(l.TestSignatures), show(l.Members),
		show(l.RequestsPerMinute), show(l.RetentionDays))
	return nil
}

// memberAdd makes a panel user a member of a space.
func memberAdd(args []string) error {
	fs := flag.NewFlagSet("member add", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "the space's id")
	user := fs.String("user", "", "the panel user id (the token's sub)")
	roleFlag := fs.String("role", "owner", "owner, admin or member")
	if err := fs.Parse(args); err != nil {
		return err
	}
	role, ok := model.ParseRole(*roleFlag)
	if !ok || !ids.Valid(*workspace) || *user == "" {
		return errors.New("-workspace (an id), -user and a valid -role are required")
	}
	ctx := consoleContext()
	db, done, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer done()
	if _, err := db.Workspace(ctx, *workspace); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	return db.AddMember(ctx, *workspace, *user, role)
}

// keyCreate mints an API key and prints it once.
func keyCreate(args []string) error {
	fs := flag.NewFlagSet("key create", flag.ContinueOnError)
	workspace := fs.String("workspace", "", "the space's id")
	envFlag := fs.String("environment", "test", "test or prod")
	name := fs.String("name", "console", "the key's name")
	scopesFlag := fs.String("scopes", "", "comma-separated scopes; all of them if empty")
	org := fs.String("organization", "", "limit the key to one organization (its id)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	env, ok := model.ParseEnvironment(*envFlag)
	if !ok || !ids.Valid(*workspace) {
		return errors.New("-workspace (an id) and -environment test|prod are required")
	}
	scopes := model.AllScopes
	if *scopesFlag != "" {
		parsed, ok := model.ParseScopes(strings.Split(*scopesFlag, ","), model.AllScopes)
		if !ok {
			return errors.New("-scopes has an unknown scope")
		}
		scopes = parsed
	}
	ctx := consoleContext()
	db, done, err := openDB(ctx)
	if err != nil {
		return err
	}
	defer done()
	if _, err := db.Workspace(ctx, *workspace); err != nil {
		return fmt.Errorf("workspace: %w", err)
	}
	var orgID *string
	if *org != "" {
		o, err := db.ResolveOrganization(ctx, *workspace, *org, "")
		if err != nil {
			return fmt.Errorf("organization: %w", err)
		}
		orgID = &o.ID
	}
	key, hash, prefix := auth.NewKey(env)
	k, err := db.CreateAPIKey(ctx, store.NewAPIKey{
		WorkspaceID: *workspace, OrganizationID: orgID, Environment: env, Name: *name, Scopes: scopes,
		Hash: hash, Prefix: prefix, CreatedBy: "console",
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "api key", k.ID, "created; it won't be shown again:")
	fmt.Println(key)
	return nil
}
