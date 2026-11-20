package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/ids"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/store"
)

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
	ctx := context.Background()
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
	ctx := context.Background()
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
	ctx := context.Background()
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
