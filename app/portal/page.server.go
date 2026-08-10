package portal

import (
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/present"
	"github.com/m31-labs/rostrum/internal/token"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

// portalSessionKey names the GoSX session value that binds a browser
// session to one speaker ID once a visitor presents a valid portal token.
const portalSessionKey = "portal_speaker"

func init() {
	_, thisFile, _, _ := runtime.Caller(0)
	source := filepath.Join(filepath.Dir(thisFile), "[speaker]", "page.gsx")
	if err := route.RegisterFileModule(route.FileModuleFor(source, route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return loadPortal(ctx)
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Speaker portal — Rostrum"}, Description: "Update your profile, complete event tasks, and review your schedule."}, nil
		},
		Actions: route.FileActions{
			"updateProfile": updateProfile,
			"completeTask":  completeTask,
		},
	})); err != nil {
		log.Fatal(err)
	}
}

// loadPortal binds the visiting session to a speaker identity and renders
// that speaker's portal.
//
// A `?key=<token>` query parameter carries a signed token.New().Verify
// result; a valid one binds its speaker ID into the session so later
// requests need no query key. The page only renders when the bound speaker
// matches the path speaker and that speaker exists in the store.
//
// An unknown speaker in the path and a missing or invalid key both render
// the identical friendly page (L8): neither response tells a visitor
// whether a given speaker ID exists.
func loadPortal(ctx *route.RouteContext) (any, error) {
	requested := strings.TrimSpace(ctx.Param("speaker"))
	store := session.Current(ctx.Request)
	bound := store.String(portalSessionKey)

	if key := strings.TrimSpace(ctx.Query("key")); key != "" {
		if id, ok := token.New().Verify(key); ok {
			bound = id
			store.Set(portalSessionKey, id)
		}
	}

	if bound != "" && bound == requested {
		snapshot := appstate.MustGet().Snapshot()
		if data, err := present.SpeakerPortal(snapshot, bound, ctx.Query("submitted") == "1"); err == nil {
			data["available"] = true
			return data, nil
		}
	}
	return portalUnavailable(), nil
}

// portalUnavailable is the identical friendly response for an unbound
// session, an invalid or expired key, and an unknown speaker (L8).
func portalUnavailable() map[string]any {
	return map[string]any{"available": false}
}

// boundSpeaker returns the speaker ID bound to the visiting session by
// loadPortal, or empty when no portal session is bound.
//
// Actions never derive identity from a posted speaker_id field (H1 fix): a
// forged or mismatched speaker_id in the request body is inert because
// nothing here reads it.
func boundSpeaker(ctx *action.Context) string {
	return session.Current(ctx.Request).String(portalSessionKey)
}

func updateProfile(ctx *action.Context) error {
	speakerID := boundSpeaker(ctx)
	if speakerID == "" {
		return action.Validation("Your portal session expired. Reopen your portal link and try again.", nil, ctx.FormData)
	}
	if len(strings.TrimSpace(ctx.FormData["biography"])) < 40 {
		return action.Validation("Add a little more detail to your biography.", map[string]string{"biography": "Use at least 40 characters."}, ctx.FormData)
	}
	// SE-4: reject any scheme other than http or https at write time, so a
	// javascript: URL can never reach an href on this page or the public
	// /api/v1/speakers JSON. A blank value stays allowed -- it clears the
	// link -- but anything non-blank must be an absolute http(s) URL.
	linkedin := strings.TrimSpace(ctx.FormData["linkedin"])
	website := strings.TrimSpace(ctx.FormData["website"])
	urlErrors := map[string]string{}
	if !present.AllowedURL(linkedin) {
		urlErrors["linkedin"] = "Use a link that starts with http:// or https://."
	}
	if !present.AllowedURL(website) {
		urlErrors["website"] = "Use a link that starts with http:// or https://."
	}
	if len(urlErrors) > 0 {
		return action.Validation("Use http:// or https:// links only.", urlErrors, ctx.FormData)
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		speaker, found := state.Speaker(speakerID)
		if !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		speaker.Pronouns = strings.TrimSpace(ctx.FormData["pronouns"])
		speaker.Role = strings.TrimSpace(ctx.FormData["role"])
		speaker.Company = strings.TrimSpace(ctx.FormData["company"])
		speaker.Biography = strings.TrimSpace(ctx.FormData["biography"])
		speaker.City = strings.TrimSpace(ctx.FormData["city"])
		speaker.LinkedInURL = linkedin
		speaker.WebsiteURL = website
		speaker.UpdatedAt = time.Now().UTC()
		upsertCompletion(state, "task_profile", speakerID, domain.TaskSubmitted, nil)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Your public profile is updated and awaiting program approval.")
	live.Broadcast("speaker:updated", map[string]string{"speaker": speakerID})
	actionflow.Redirect(ctx, "/portal/"+speakerID)
	return nil
}

func completeTask(ctx *action.Context) error {
	speakerID := boundSpeaker(ctx)
	taskID := strings.TrimSpace(ctx.FormData["task_id"])
	if speakerID == "" || taskID == "" {
		return action.Validation("Task identity is missing.", map[string]string{"task": "Reload the portal and try again."}, ctx.FormData)
	}
	values := make(map[string]string)
	for key, value := range ctx.FormData {
		if key != "csrf_token" && key != "task_id" && key != "speaker_id" {
			values[key] = value
		}
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		if _, found := state.Speaker(speakerID); !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		if _, found := state.Task(taskID); !found {
			return fmt.Errorf("task %s not found", taskID)
		}
		upsertCompletion(state, taskID, speakerID, domain.TaskSubmitted, values)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Task submitted. The program team can review it now.")
	live.Broadcast("task:submitted", map[string]string{"speaker": speakerID, "task": taskID})
	actionflow.Redirect(ctx, "/portal/"+speakerID+"#tasks")
	return nil
}

func upsertCompletion(state *domain.State, taskID, speakerID, status string, values map[string]string) {
	if item, found := state.Completion(taskID, speakerID); found {
		item.Status = status
		item.Values = values
		item.UpdatedAt = time.Now().UTC()
		if status == domain.TaskSubmitted || status == domain.TaskApproved {
			item.CompletedAt = time.Now().UTC()
		}
		return
	}
	now := time.Now().UTC()
	state.TaskCompletions = append(state.TaskCompletions, domain.TaskCompletion{ID: domain.NewID("done"), TaskID: taskID, SpeakerID: speakerID, Status: status, Values: values, CompletedAt: now, UpdatedAt: now})
}
