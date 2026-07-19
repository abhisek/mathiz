package server

import (
	"context"
	"testing"

	"github.com/abhisek/mathiz/ent/quest"
)

// TestCoParentFlowAndAuthzMatrix drives the co-parent lifecycle end to end
// (invite → /me banner → accept) and then checks the full permission matrix:
// owner and co-parent share children/stats/invites/quests/parent-roster
// reads; billing and parent management are owner-only (404 for co-parents);
// strangers get 404 everywhere.
func TestCoParentFlowAndAuthzMatrix(t *testing.T) {
	e := newTestEnv(t)
	owner := parentToken(t, "sb-owner", "owner@example.com")
	coParent := parentToken(t, "sb-cop", "cop@example.com")
	stranger := parentToken(t, "sb-stranger", "stranger@example.com")

	// Owner bootstraps the family.
	var space spaceJSON
	resp := e.call(t, "POST", "/api/v1/family", owner, map[string]string{"name": "The Sharks"}, &space)
	expectStatus(t, resp, 201, "create family")
	var child childJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/children", owner,
		map[string]any{"name": "Finn", "grade": 3}, &child)
	expectStatus(t, resp, 201, "add child")

	// /me: owner sees role owner; the invitee has no family and no banner yet.
	var me struct {
		Family        *spaceJSON     `json:"family"`
		Role          string         `json:"role"`
		PendingInvite map[string]any `json:"pendingInvite"`
	}
	resp = e.call(t, "GET", "/api/v1/me", owner, nil, &me)
	expectStatus(t, resp, 200, "owner me")
	if me.Family == nil || me.Role != "owner" {
		t.Fatalf("owner me = %+v", me)
	}
	me = struct {
		Family        *spaceJSON     `json:"family"`
		Role          string         `json:"role"`
		PendingInvite map[string]any `json:"pendingInvite"`
	}{}
	resp = e.call(t, "GET", "/api/v1/me", coParent, nil, &me)
	expectStatus(t, resp, 200, "invitee me before invite")
	if me.Family != nil || me.PendingInvite != nil {
		t.Fatalf("invitee me before invite = %+v", me)
	}

	// Only the owner can invite a co-parent; email matching is case-blind.
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/parents", stranger,
		map[string]string{"email": "x@example.com"}, nil)
	expectStatus(t, resp, 404, "stranger invites parent")
	var invite parentInviteJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/parents", owner,
		map[string]string{"email": "COP@example.com"}, &invite)
	expectStatus(t, resp, 201, "owner invites parent")
	if invite.Email != "cop@example.com" || invite.Status != "pending" {
		t.Fatalf("invite = %+v", invite)
	}

	// The invitee's /me now carries the accept banner.
	me.PendingInvite = nil
	resp = e.call(t, "GET", "/api/v1/me", coParent, nil, &me)
	expectStatus(t, resp, 200, "invitee me")
	if me.PendingInvite == nil || me.PendingInvite["id"] != invite.ID ||
		me.PendingInvite["familyName"] != "The Sharks" {
		t.Fatalf("pending invite = %+v", me.PendingInvite)
	}

	// A different signed-in account cannot accept someone else's invite.
	resp = e.call(t, "POST", "/api/v1/invites/parent/"+invite.ID+"/accept", stranger, nil, nil)
	expectStatus(t, resp, 404, "stranger accepts invite")

	var accepted struct {
		Family spaceJSON `json:"family"`
		Role   string    `json:"role"`
	}
	resp = e.call(t, "POST", "/api/v1/invites/parent/"+invite.ID+"/accept", coParent, nil, &accepted)
	expectStatus(t, resp, 200, "accept invite")
	if accepted.Family.ID != space.ID || accepted.Role != "parent" {
		t.Fatalf("accepted = %+v", accepted)
	}
	me = struct {
		Family        *spaceJSON     `json:"family"`
		Role          string         `json:"role"`
		PendingInvite map[string]any `json:"pendingInvite"`
	}{}
	resp = e.call(t, "GET", "/api/v1/me", coParent, nil, &me)
	expectStatus(t, resp, 200, "co-parent me")
	if me.Family == nil || me.Family.ID != space.ID || me.Role != "parent" {
		t.Fatalf("co-parent me = %+v", me)
	}

	// ---- The matrix ----
	type probe struct {
		label          string
		method, path   string
		body           any
		owner, cop, st int
	}
	probes := []probe{
		{"children list", "GET", "/api/v1/family/" + space.ID + "/children", nil, 200, 200, 404},
		{"child stats", "GET", "/api/v1/children/" + child.ID + "/stats", nil, 200, 200, 404},
		{"child update", "PATCH", "/api/v1/children/" + child.ID, map[string]any{"name": "Finnley"}, 200, 200, 404},
		{"join-code mint", "POST", "/api/v1/family/" + space.ID + "/invites", map[string]any{}, 201, 201, 404},
		{"join-code list", "GET", "/api/v1/family/" + space.ID + "/invites", nil, 200, 200, 404},
		{"quest list", "GET", "/api/v1/family/" + space.ID + "/quests", nil, 200, 200, 404},
		{"parents list", "GET", "/api/v1/family/" + space.ID + "/parents", nil, 200, 200, 404},
		{"billing state", "GET", "/api/v1/family/" + space.ID + "/billing", nil, 200, 404, 404},
		{"billing checkout", "POST", "/api/v1/family/" + space.ID + "/billing/checkout", map[string]string{"planId": "topup-100"}, 200, 404, 404},
		{"billing portal", "POST", "/api/v1/family/" + space.ID + "/billing/portal", map[string]any{}, 200, 404, 404},
	}
	for _, pr := range probes {
		resp := e.call(t, pr.method, pr.path, owner, pr.body, nil)
		expectStatus(t, resp, pr.owner, "owner "+pr.label)
		resp = e.call(t, pr.method, pr.path, coParent, pr.body, nil)
		expectStatus(t, resp, pr.cop, "co-parent "+pr.label)
		resp = e.call(t, pr.method, pr.path, stranger, pr.body, nil)
		expectStatus(t, resp, pr.st, "stranger "+pr.label)
	}

	// Quest creation works for both members and records who authored it.
	var ownerQuest, copQuest questJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/quests", owner,
		map[string]string{"name": "Owner quest"}, &ownerQuest)
	expectStatus(t, resp, 201, "owner quest create")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/quests", coParent,
		map[string]string{"name": "Co-parent quest"}, &copQuest)
	expectStatus(t, resp, 201, "co-parent quest create")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/quests", stranger,
		map[string]string{"name": "Stranger quest"}, nil)
	expectStatus(t, resp, 404, "stranger quest create")

	// Resolve account IDs from the roster; created_by must match the author.
	var roster struct {
		Parents []parentMemberJSON `json:"parents"`
		Invites []parentInviteJSON `json:"invites"`
	}
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/parents", coParent, nil, &roster)
	expectStatus(t, resp, 200, "roster")
	if len(roster.Parents) != 2 {
		t.Fatalf("roster = %+v", roster.Parents)
	}
	var ownerID, copID string
	for _, m := range roster.Parents {
		switch m.Role {
		case "owner":
			ownerID = m.AccountID
		case "parent":
			copID = m.AccountID
		}
	}
	if ownerID == "" || copID == "" {
		t.Fatalf("roster roles = %+v", roster.Parents)
	}
	ctx := context.Background()
	for _, q := range []struct{ uid, wantBy string }{
		{ownerQuest.ID, ownerID},
		{copQuest.ID, copID},
	} {
		row, err := e.st.Client().Quest.Query().Where(quest.UID(q.uid)).Only(ctx)
		if err != nil {
			t.Fatalf("load quest: %v", err)
		}
		if row.CreatedBy != q.wantBy {
			t.Errorf("quest %s created_by = %q, want %q", q.uid, row.CreatedBy, q.wantBy)
		}
	}

	// Parent management is owner-only: the co-parent can't revoke invites or
	// remove members — not even themselves.
	var invite2 parentInviteJSON
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/parents", owner,
		map[string]string{"email": "third@example.com"}, &invite2)
	expectStatus(t, resp, 201, "second invite")
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/parents", coParent,
		map[string]string{"email": "fourth@example.com"}, nil)
	expectStatus(t, resp, 404, "co-parent invites parent")
	resp = e.call(t, "DELETE", "/api/v1/parent-invites/"+invite2.ID, coParent, nil, nil)
	expectStatus(t, resp, 404, "co-parent revokes invite")
	resp = e.call(t, "DELETE", "/api/v1/parent-invites/"+invite2.ID, stranger, nil, nil)
	expectStatus(t, resp, 404, "stranger revokes invite")
	resp = e.call(t, "DELETE", "/api/v1/family/"+space.ID+"/parents/"+copID, coParent, nil, nil)
	expectStatus(t, resp, 404, "co-parent removes member")
	resp = e.call(t, "DELETE", "/api/v1/family/"+space.ID+"/parents/"+copID, stranger, nil, nil)
	expectStatus(t, resp, 404, "stranger removes member")

	// Owner can: revoke the invite, never remove itself, remove the co-parent.
	resp = e.call(t, "DELETE", "/api/v1/parent-invites/"+invite2.ID, owner, nil, nil)
	expectStatus(t, resp, 204, "owner revokes invite")
	resp = e.call(t, "DELETE", "/api/v1/family/"+space.ID+"/parents/"+ownerID, owner, nil, nil)
	expectStatus(t, resp, 400, "owner removes itself")
	resp = e.call(t, "DELETE", "/api/v1/family/"+space.ID+"/parents/"+copID, owner, nil, nil)
	expectStatus(t, resp, 204, "owner removes co-parent")

	// The removed co-parent has lost all access.
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/children", coParent, nil, nil)
	expectStatus(t, resp, 404, "removed co-parent children list")
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/billing", coParent, nil, nil)
	expectStatus(t, resp, 404, "removed co-parent billing")
}
