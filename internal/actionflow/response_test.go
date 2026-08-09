package actionflow

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"m31labs.dev/gosx/action"
)

func TestRedirectUsesOKForManagedFormRequests(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/save", nil)
	request.Header.Set("Accept", "application/json")
	recorder := httptest.NewRecorder()
	action.ServeHandler(recorder, request, func(served *action.Context) error {
		Redirect(served, "/done")
		return nil
	})
	if recorder.Code != http.StatusOK {
		t.Fatalf("managed redirect status = %d; want %d", recorder.Code, http.StatusOK)
	}
	if got := recorder.Body.String(); got == "" {
		t.Fatal("managed redirect should retain its JSON body")
	}
	var result action.Result
	if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode managed redirect: %v", err)
	}
	if !result.OK || result.Redirect != "/done" {
		t.Fatalf("managed redirect = %#v; want ok redirect to /done", result)
	}
}

func TestRedirectKeepsSeeOtherForNativeForms(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/save", nil)
	recorder := httptest.NewRecorder()

	action.ServeHandler(recorder, request, func(ctx *action.Context) error {
		Redirect(ctx, "/done")
		return nil
	})
	if recorder.Code != http.StatusSeeOther {
		t.Fatalf("native redirect status = %d; want %d", recorder.Code, http.StatusSeeOther)
	}
	if location := recorder.Header().Get("Location"); location != "/done" {
		t.Fatalf("native redirect location = %q; want /done", location)
	}
}
