package submissions

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/m31-labs/rostrum/internal/actionflow"
	"github.com/m31-labs/rostrum/internal/appstate"
	programcalendar "github.com/m31-labs/rostrum/internal/calendar"
	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/live"
	"github.com/m31-labs/rostrum/internal/mail"
	"github.com/m31-labs/rostrum/internal/present"
	"m31labs.dev/gosx/action"
	"m31labs.dev/gosx/route"
	"m31labs.dev/gosx/server"
	"m31labs.dev/gosx/session"
)

func init() {
	if err := route.RegisterFileModuleHere(route.FileModuleOptions{
		Load: func(ctx *route.RouteContext, page route.FilePage) (any, error) {
			return present.Submissions(appstate.MustGet().Snapshot(), ctx.Query("q"), ctx.Query("status"), ctx.Query("category")), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Submissions — Rostrum"}, Description: "Filter, route, and update proposal decisions."}, nil
		},
		Actions: route.FileActions{"updateStatus": updateStatus},
	}); err != nil {
		log.Fatal(err)
	}
}

// acceptanceSender is the process-wide mail transport this package sends
// acceptance communications through, resolved once on first use. It
// mirrors app/submit/page.server.go's confirmationSender: a deferred
// value, not a plain package var, because mail.FromEnv reads SMTP_* from
// the environment, which main() loads from .env only after package-level
// initialization has already run.
var acceptanceSender = sync.OnceValue(mail.FromEnv)

func updateStatus(ctx *action.Context) error {
	id := strings.TrimSpace(ctx.FormData["submission_id"])
	status := strings.TrimSpace(ctx.FormData["status"])
	valid := false
	for _, candidate := range domain.SubmissionStatuses {
		if candidate == status {
			valid = true
			break
		}
	}
	if !valid {
		return action.Validation("Choose a valid status.", map[string]string{"status": "Unknown submission status."}, ctx.FormData)
	}
	title := ""
	acceptedSessionID := ""
	var acceptedSpeakerIDs []string
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		submission, found := state.Submission(id)
		if !found {
			return fmt.Errorf("submission %s not found", id)
		}
		submission.Status = status
		title = submission.Title
		if status == domain.SubmissionAccepted {
			state.AddSessionForSubmission(id)
			state.AssignAcceptedOnlyTasks(submission.SpeakerIDs)
			if accepted, found := state.SessionBySubmission(submission.ID); found {
				acceptedSessionID = accepted.ID
				// Collect only the speakers who do not already carry an
				// acceptance Communication for this session -- the same
				// pair QueueAcceptanceCommunication is about to check --
				// so re-accepting an already-accepted submission (a no-op
				// status transition) never re-sends a message whose row
				// was already queued, sent, or failed.
				for _, speakerID := range submission.SpeakerIDs {
					if speakerID != "" && !state.HasAcceptanceCommunication(accepted.ID, speakerID) {
						acceptedSpeakerIDs = append(acceptedSpeakerIDs, speakerID)
					}
				}
				state.QueueAcceptanceCommunication(accepted.ID, submission.SpeakerIDs)
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Send the acceptance communication -- the merged tpl_acceptance text
	// plus, since that template has AttachCalendar set, the session's
	// calendar invite -- outside the store lock: Send is a network
	// operation and must never run while a lock is held (see internal/mail's
	// package doc comment). The row itself was already queued above by
	// QueueAcceptanceCommunication; sendAcceptanceInvite records the real
	// outcome onto that same row afterward.
	for _, speakerID := range acceptedSpeakerIDs {
		sendAcceptanceInvite(acceptedSessionID, speakerID)
	}

	session.AddFlash(ctx.Request, "notice", "“"+title+"” moved to "+present.StatusLabel(status)+".")
	live.Broadcast("submission:updated", map[string]string{"id": id, "status": status})
	// The list page and the detail page share this action. Send the
	// user back to the page that posted the form: the action URL is
	// "<page>/__actions/updateStatus", so the page path is its prefix.
	target := "/organizer/submissions"
	if origin := strings.TrimSuffix(ctx.Request.URL.Path, "/__actions/updateStatus"); strings.HasPrefix(origin, "/organizer/submissions/") {
		target = origin
	}
	actionflow.Redirect(ctx, target)
	return nil
}

// sendAcceptanceInvite sends the queued acceptance Communication
// QueueAcceptanceCommunication already appended for speakerID on
// sessionID: the merged tpl_acceptance template text, plus -- when the
// session carries a schedule -- a calendar invite attached as the
// message's text/calendar part (RFC 5545). It records the real outcome
// (sent or failed) back onto that same row through
// domain.State.MarkCommunicationSent. A missing speaker, session, or
// template is a no-op: updateStatus already validated the submission
// before calling this, so a miss here means nothing was queued to send.
func sendAcceptanceInvite(sessionID, speakerID string) {
	if sessionID == "" || speakerID == "" {
		return
	}
	snapshot := appstate.MustGet().Snapshot()
	speaker, found := snapshot.Speaker(speakerID)
	if !found {
		return
	}
	sessionItem, found := snapshot.Session(sessionID)
	if !found {
		return
	}
	template, found := emailTemplate(snapshot, domain.AcceptanceTemplateID)
	if !found {
		return
	}
	subject, body := present.RenderCommunication(snapshot, template, *speaker, *sessionItem)

	msg := mail.Message{To: speaker.Email, ToName: speaker.Name(), Subject: subject, TextBody: body}
	if template.AttachCalendar && sessionItem.Scheduled() {
		ics, err := programcalendar.Invite(snapshot, *sessionItem, *speaker, organizerEmail(template))
		if err != nil {
			// A session with no room, or an organizer address that never
			// got configured, never blocks the acceptance message itself
			// -- the speaker still gets the merged template text, just
			// without the attachment.
			log.Printf("submissions: could not build the calendar invite for speaker %s: %v", speakerID, err)
		} else {
			msg.Calendar = ics
		}
	}

	sender := acceptanceSender()
	sendErr := sender.Send(msg)
	if sendErr != nil {
		log.Printf("submissions: acceptance send to speaker %s failed: %v", speakerID, sendErr)
	}
	provider := "demo-outbox"
	if named, ok := sender.(mail.Named); ok {
		provider = named.Name()
	}
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		state.MarkCommunicationSent(domain.AcceptanceTemplateID, speakerID, sessionID, provider, sendErr)
		return nil
	}); err != nil {
		log.Printf("submissions: could not record the acceptance send outcome for speaker %s: %v", speakerID, err)
	}
}

// emailTemplate finds the EmailTemplate named id in state, if any. Mirrors
// app/organizer/communications/page.server.go's helper of the same name;
// duplicated rather than shared because a page.server.go package
// deliberately stays a self-contained action module (compare
// publicBaseURL, duplicated the same way between app/submit and
// app/organizer/review).
func emailTemplate(state domain.State, id string) (domain.EmailTemplate, bool) {
	for _, template := range state.EmailTemplates {
		if template.ID == id {
			return template, true
		}
	}
	return domain.EmailTemplate{}, false
}

// organizerEmail resolves the address a calendar invite's ORGANIZER line
// names: template's configured reply-to address when set (every seeded
// template carries one, for example "program@example.com"), otherwise the
// process-wide MAIL_FROM address reduced to a bare address. Mirrors
// app/organizer/communications/page.server.go's helper of the same name.
func organizerEmail(template domain.EmailTemplate) string {
	if replyTo := strings.TrimSpace(template.ReplyTo); replyTo != "" {
		return replyTo
	}
	return mail.AddressOnly(strings.TrimSpace(os.Getenv("MAIL_FROM")))
}
