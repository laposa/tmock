package app_test

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/logx"
)

func TestUserValidation(t *testing.T) {
	t.Parallel()
	a, _, _ := newApp(t)
	ctx := context.Background()

	_, err := a.CreateUser(ctx, ann, app.UserInput{Name: " ", Email: "Bob <bob@example.com>", Password: "short"})
	e := appErr(t, err, app.KindInvalid)
	want := map[string]string{
		"name":     "is required",
		"email":    "must be an email address",
		"password": "must be at least 8 characters",
	}
	for k, v := range want {
		if e.Details[k] != v {
			t.Errorf("%s: %q, want %q", k, e.Details[k], v)
		}
	}
	_, err = a.CreateUser(ctx, ann, app.UserInput{Name: strings.Repeat("x", 256), Email: "a@b", Password: strings.Repeat("p", 1025)})
	e = appErr(t, err, app.KindInvalid)
	if e.Details["name"] != "must be 1–255 characters" || e.Details["password"] != "must be at most 1024 characters" || e.Details["email"] != "" {
		t.Errorf("limits: %v", e.Details)
	}
	// Characters, not bytes: eight two-byte characters are a valid password.
	if _, err := a.CreateUser(ctx, ann, app.UserInput{Name: "Ü", Email: "u@example.com", Password: "üüüüüüüü"}); err != nil {
		t.Errorf("eight characters: %v", err)
	}

	u := must(a.CreateUser(ctx, ann, app.UserInput{Name: "Bob", Email: "bob@example.com", Password: "password1"}))
	_, err = a.UpdateUser(ctx, ann, u.ID, app.UserPatch{Name: httpx.Null[string](), Email: httpx.Null[string](), Password: httpx.Null[string](), Admin: httpx.Null[bool]()})
	e = appErr(t, err, app.KindInvalid)
	if len(e.Details) != 4 {
		t.Errorf("null fields: %v", e.Details)
	}
	_, err = a.UpdateUser(ctx, ann, u.ID, app.UserPatch{Password: httpx.Some("1234567")})
	if e := appErr(t, err, app.KindInvalid); e.Details["password"] != "must be at least 8 characters" {
		t.Errorf("short password: %v", e.Details)
	}
	appErr(t, must2(a.UpdateUser(ctx, ann, 999999, app.UserPatch{Name: httpx.Some("x")})), app.KindNotFound)
	appErr(t, must2(a.DeleteUser(ctx, ann, 999999)), app.KindNotFound)
}

func TestUserCRUD(t *testing.T) {
	t.Parallel()
	a, users, _ := newApp(t)
	ctx := context.Background()
	admin := addUser(t, users, "ann@example.com", "password1", fast) // an admin
	me := app.UserActor(admin.ID)

	bob, err := a.CreateUser(ctx, me, app.UserInput{Name: " Bob ", Email: " Bob@Example.com ", Password: "password1"})
	if err != nil || bob.Name != "Bob" || bob.Email != "Bob@Example.com" || bob.Admin {
		t.Fatalf("create: %+v, %v", bob, err)
	}
	// A duplicate email is a Conflict next to the field, whatever its case.
	_, err = a.CreateUser(ctx, me, app.UserInput{Name: "B2", Email: "bob@EXAMPLE.com", Password: "password1"})
	if e := appErr(t, err, app.KindConflict); e.Field != "email" || e.Message != `A user with email "bob@EXAMPLE.com" already exists` {
		t.Errorf("duplicate: %+v", e)
	}
	_, err = a.UpdateUser(ctx, me, bob.ID, app.UserPatch{Email: httpx.Some("ANN@example.com")})
	if e := appErr(t, err, app.KindConflict); e.Field != "email" {
		t.Errorf("duplicate on update: %+v", e)
	}
	// Changing only the case of one's own email is not a duplicate.
	if u, err := a.UpdateUser(ctx, me, bob.ID, app.UserPatch{Email: httpx.Some("bob@example.com")}); err != nil || u.Email != "bob@example.com" {
		t.Errorf("own email in another case: %+v, %v", u, err)
	}

	// Only the fields present change; a new password works for login.
	u, err := a.UpdateUser(ctx, me, bob.ID, app.UserPatch{Admin: httpx.Some(true), Password: httpx.Some("new-password")})
	if err != nil || !u.Admin || u.Name != "Bob" || u.Email != "bob@example.com" {
		t.Fatalf("update: %+v, %v", u, err)
	}
	if _, err := a.Login(ctx, app.Credentials{Email: "bob@example.com", Password: "new-password"}, "127.0.0.1"); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	if _, err := a.Login(ctx, app.Credentials{Email: "bob@example.com", Password: "password1"}, "127.0.0.1"); err == nil {
		t.Error("the old password still works")
	}

	list := must(a.ListUsers(ctx))
	if len(list) != 2 || list[0].ID != admin.ID || list[1].ID != bob.ID {
		t.Errorf("list: %+v", list)
	}
	if got := must(a.GetUser(ctx, bob.ID)); got != u {
		t.Errorf("get: %+v", got)
	}
	if _, err := a.DeleteUser(ctx, me, bob.ID); err != nil {
		t.Fatal(err)
	}
	appErr(t, must2(a.GetUser(ctx, bob.ID)), app.KindNotFound)
}

// The rules of spec §6.4: no deleting your own account, and the last admin can be neither
// deleted nor demoted.
func TestUserAdminRules(t *testing.T) {
	t.Parallel()
	a, users, _ := newApp(t)
	ctx := context.Background()
	ann := addUser(t, users, "ann@example.com", "password1", fast)
	bob := must(a.CreateUser(ctx, app.UserActor(ann.ID), app.UserInput{Name: "Bob", Email: "bob@example.com", Password: "password1"}))

	_, err := a.DeleteUser(ctx, app.UserActor(ann.ID), ann.ID)
	if e := appErr(t, err, app.KindInvalid); e.Message != "You cannot delete your own account" {
		t.Errorf("own account: %q", e.Message)
	}
	// Bob is not an admin, so Ann is the last one: Bob (were he allowed) could not delete
	// her, and nobody can take her flag.
	_, err = a.DeleteUser(ctx, app.UserActor(bob.ID), ann.ID)
	if e := appErr(t, err, app.KindInvalid); e.Message != "The last admin cannot be deleted" {
		t.Errorf("last admin: %q", e.Message)
	}
	_, err = a.UpdateUser(ctx, app.UserActor(ann.ID), ann.ID, app.UserPatch{Admin: httpx.Some(false)})
	if e := appErr(t, err, app.KindInvalid); e.Details["admin"] != "the last admin must stay an admin" {
		t.Errorf("demote last admin: %v", e.Details)
	}
	// Keeping the flag, or demoting a non-admin, is not a demotion.
	if _, err := a.UpdateUser(ctx, app.UserActor(ann.ID), ann.ID, app.UserPatch{Admin: httpx.Some(true), Name: httpx.Some("Ann")}); err != nil {
		t.Errorf("keep the flag: %v", err)
	}
	if _, err := a.UpdateUser(ctx, app.UserActor(ann.ID), bob.ID, app.UserPatch{Admin: httpx.Some(false)}); err != nil {
		t.Errorf("non-admin stays non-admin: %v", err)
	}
	// With a second admin, either can be demoted, and the other deleted.
	must(a.UpdateUser(ctx, app.UserActor(ann.ID), bob.ID, app.UserPatch{Admin: httpx.Some(true)}))
	if u, err := a.UpdateUser(ctx, app.UserActor(ann.ID), ann.ID, app.UserPatch{Admin: httpx.Some(false)}); err != nil || u.Admin {
		t.Errorf("demote one of two admins: %+v, %v", u, err)
	}
	if _, err := a.DeleteUser(ctx, app.UserActor(bob.ID), ann.ID); err != nil {
		t.Errorf("delete a non-admin: %v", err)
	}
	if us, _ := users.List(ctx); len(us) != 1 || !us[0].Admin {
		t.Errorf("one admin is left: %+v", us)
	}
}

// Two admins demoting each other, or deleting each other, at the same time: one wins and
// one admin is left (the rule is checked under a lock).
func TestUserLastAdminRace(t *testing.T) {
	t.Parallel()
	a, users, _ := newApp(t)
	ctx := context.Background()
	for round := range 5 {
		x := must(a.CreateUser(ctx, ann, app.UserInput{Name: "X", Email: "x" + itoa(int64(round)) + "@example.com", Password: "password1", Admin: true}))
		y := must(a.CreateUser(ctx, ann, app.UserInput{Name: "Y", Email: "y" + itoa(int64(round)) + "@example.com", Password: "password1", Admin: true}))
		// Only x and y are admins.
		for _, u := range must(users.List(ctx)) {
			if u.ID != x.ID && u.ID != y.ID && u.Admin {
				must(a.UpdateUser(ctx, ann, u.ID, app.UserPatch{Admin: httpx.Some(false)}))
			}
		}
		var wg sync.WaitGroup
		wg.Add(2)
		if round%2 == 0 {
			go func() {
				defer wg.Done()
				_, _ = a.UpdateUser(ctx, app.UserActor(x.ID), y.ID, app.UserPatch{Admin: httpx.Some(false)})
			}()
			go func() {
				defer wg.Done()
				_, _ = a.UpdateUser(ctx, app.UserActor(y.ID), x.ID, app.UserPatch{Admin: httpx.Some(false)})
			}()
		} else {
			go func() { defer wg.Done(); _, _ = a.DeleteUser(ctx, app.UserActor(x.ID), y.ID) }()
			go func() { defer wg.Done(); _, _ = a.DeleteUser(ctx, app.UserActor(y.ID), x.ID) }()
		}
		wg.Wait()
		if n := must(users.CountAdmins(ctx)); n != 1 {
			t.Fatalf("round %d: %d admins left, want 1", round, n)
		}
	}
}

func TestUpdateProfile(t *testing.T) {
	t.Parallel()
	a, users, logs := newApp(t)
	ctx := context.Background()
	ann := addUser(t, users, "ann@example.com", "password1", fast)
	addUser(t, users, "bob@example.com", "password1", fast)

	// Name and email without a password; only the fields present change.
	u, err := a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{Name: httpx.Some(" Annie ")})
	if err != nil || u.Name != "Annie" || u.Email != "ann@example.com" || !u.Admin {
		t.Fatalf("name: %+v, %v", u, err)
	}
	_, err = a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{Email: httpx.Some("BOB@example.com")})
	if e := appErr(t, err, app.KindConflict); e.Field != "email" {
		t.Errorf("duplicate email: %+v", e)
	}

	// A new password needs the correct current one.
	_, err = a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{NewPassword: httpx.Some("new-password")})
	if e := appErr(t, err, app.KindInvalid); e.Details["currentPassword"] != "is required to set a new password" {
		t.Errorf("no current password: %v", e.Details)
	}
	_, err = a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{CurrentPassword: httpx.Some("wrong-password"), NewPassword: httpx.Some("new-password"), Name: httpx.Some("Changed")})
	if e := appErr(t, err, app.KindInvalid); e.Details["currentPassword"] != "is not correct" {
		t.Errorf("wrong current password: %v", e.Details)
	}
	if got := must(a.GetUser(ctx, ann.ID)); got.Name != "Annie" {
		t.Errorf("a refused change saved the name: %+v", got)
	}
	_, err = a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{CurrentPassword: httpx.Some("password1"), NewPassword: httpx.Some("short")})
	if e := appErr(t, err, app.KindInvalid); e.Details["newPassword"] != "must be at least 8 characters" {
		t.Errorf("short new password: %v", e.Details)
	}
	if _, err := a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{CurrentPassword: httpx.Some("password1"), NewPassword: httpx.Some("new-password")}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Login(ctx, app.Credentials{Email: "ann@example.com", Password: "new-password"}, "127.0.0.1"); err != nil {
		t.Errorf("login with the new password: %v", err)
	}
	// A current password alone changes nothing and is not an error.
	if _, err := a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{CurrentPassword: httpx.Some("anything")}); err != nil {
		t.Errorf("current password only: %v", err)
	}
	for _, r := range logs.All() {
		for _, secret := range []string{"password1", "new-password", "wrong-password"} {
			if strings.Contains(r.Text(), secret) {
				t.Errorf("a password was logged: %s", r.Text())
			}
		}
	}
}

func TestUserChangedEvents(t *testing.T) {
	t.Parallel()
	a, users, logs := newApp(t)
	ctx := context.Background()
	ann := addUser(t, users, "ann@example.com", "password1", fast)
	me := app.UserActor(ann.ID)
	bob := must(a.CreateUser(ctx, me, app.UserInput{Name: "Bob", Email: "bob@example.com", Password: "secret-password"}))
	must(a.UpdateUser(ctx, me, bob.ID, app.UserPatch{Name: httpx.Some("Robert")}))
	must(a.UpdateProfile(ctx, ann.ID, app.ProfilePatch{Name: httpx.Some("Annie")}))
	must(a.DeleteUser(ctx, me, bob.ID))

	events := logs.Events(logx.EventEntityChanged)
	want := []struct {
		action string
		id     int64
	}{{"create", bob.ID}, {"update", bob.ID}, {"update", ann.ID}, {"delete", bob.ID}}
	if len(events) != len(want) {
		t.Fatalf("events: %+v", events)
	}
	for i, e := range events {
		if e.Level != slog.LevelInfo {
			t.Errorf("%d: level %v", i, e.Level)
		}
		for k, v := range map[string]string{
			logx.AttrEntityType:   "user",
			logx.AttrEntityAction: want[i].action,
			logx.AttrEntityID:     itoa(want[i].id),
			logx.AttrActor:        "user",
			logx.AttrUserID:       itoa(ann.ID),
		} {
			if e.Attrs[k] != v {
				t.Errorf("%d: %s = %q, want %q", i, k, e.Attrs[k], v)
			}
		}
		if strings.Contains(e.Text(), "secret-password") || strings.Contains(e.Text(), "@example.com") {
			t.Errorf("%d: a password or an email was logged: %s", i, e.Text())
		}
	}
}
