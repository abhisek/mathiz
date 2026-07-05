package server

import (
	"net/http"
	"net/url"
	"testing"
)

// TestBillingLifecycle drives the money path with the fake provider:
// starter credits → subscribe → credits granted → top-up → webhook replay
// safety → cross-tenant denial.
func TestBillingLifecycle(t *testing.T) {
	e := newTestEnv(t)
	parentA := parentToken(t, "sb-parent-a", "a@example.com")
	parentB := parentToken(t, "sb-parent-b", "b@example.com")

	var space spaceJSON
	resp := e.call(t, "POST", "/api/v1/family", parentA, map[string]string{"name": "Fam"}, &space)
	expectStatus(t, resp, 201, "create family")

	// Starter credits arrived with the free space.
	var bill struct {
		Balance int    `json:"balance"`
		Plan    string `json:"plan"`
		Status  string `json:"status"`
		Plans   []struct {
			ID             string `json:"id"`
			MonthlyCredits int    `json:"monthlyCredits"`
			TopupCredits   int    `json:"topupCredits"`
		} `json:"plans"`
	}
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/billing", parentA, nil, &bill)
	expectStatus(t, resp, 200, "billing")
	if bill.Balance != 30 || bill.Status != "none" {
		t.Fatalf("fresh billing = %+v, want 30 starter credits", bill)
	}
	if len(bill.Plans) == 0 {
		t.Fatal("catalog missing")
	}

	// Parent B can't see A's billing.
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/billing", parentB, nil, nil)
	expectStatus(t, resp, 404, "cross-tenant billing")

	// Subscribe to Explorer via fake checkout.
	var checkout struct {
		URL string `json:"url"`
	}
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/billing/checkout", parentA,
		map[string]string{"planId": "explorer"}, &checkout)
	expectStatus(t, resp, 200, "checkout")
	if checkout.URL == "" {
		t.Fatal("no checkout URL")
	}

	// "Pay": follow the fake completion URL (don't follow its redirect).
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	completeResp, err := client.Get(e.ts.URL + pathOf(t, checkout.URL))
	if err != nil {
		t.Fatalf("complete checkout: %v", err)
	}
	completeResp.Body.Close()
	if completeResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("complete status = %d", completeResp.StatusCode)
	}

	// Balance = 30 starter + 150 explorer; plan active.
	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/billing", parentA, nil, &bill)
	expectStatus(t, resp, 200, "billing after subscribe")
	if bill.Balance != 180 || bill.Plan != "explorer" || bill.Status != "active" {
		t.Fatalf("after subscribe = %+v, want 180/explorer/active", bill)
	}

	// A used fake checkout token can't be replayed to double-grant.
	replayResp, err := client.Get(e.ts.URL + pathOf(t, checkout.URL))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	replayResp.Body.Close()
	if replayResp.StatusCode != http.StatusBadRequest {
		t.Errorf("replay status = %d, want 400", replayResp.StatusCode)
	}

	// Top-up adds 100 non-expiring credits.
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/billing/checkout", parentA,
		map[string]string{"planId": "topup-100"}, &checkout)
	expectStatus(t, resp, 200, "topup checkout")
	topupResp, err := client.Get(e.ts.URL + pathOf(t, checkout.URL))
	if err != nil {
		t.Fatalf("topup complete: %v", err)
	}
	topupResp.Body.Close()

	resp = e.call(t, "GET", "/api/v1/family/"+space.ID+"/billing", parentA, nil, &bill)
	expectStatus(t, resp, 200, "billing after topup")
	if bill.Balance != 280 {
		t.Errorf("after topup = %d, want 280", bill.Balance)
	}

	// Unknown plan is rejected.
	resp = e.call(t, "POST", "/api/v1/family/"+space.ID+"/billing/checkout", parentA,
		map[string]string{"planId": "yacht"}, nil)
	expectStatus(t, resp, 400, "unknown plan")
}

// pathOf strips the fake provider's base URL, keeping path?query for use
// against the httptest server.
func pathOf(t *testing.T, rawURL string) string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse checkout url %q: %v", rawURL, err)
	}
	if u.RawQuery == "" {
		return u.Path
	}
	return u.Path + "?" + u.RawQuery
}
