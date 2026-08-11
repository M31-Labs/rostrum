package communications

import (
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

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
			return present.Communications(appstate.MustGet().Snapshot(), ctx.Query("template"), ctx.Query("recipient")), nil
		},
		Metadata: func(ctx *route.RouteContext, page route.FilePage, data any) (server.Metadata, error) {
			return server.Metadata{Title: server.Title{Default: "Communications — Rostrum"}, Description: "Template, schedule, and preview speaker communications and calendar invites."}, nil
		},
		Actions: route.FileActions{"queueMessage": queueMessage},
	}); err != nil {
		log.Fatal(err)
	}
}

// messageSender is the process-wide mail transport this package sends
// through, resolved once on first use. It mirrors
// app/submit/page.server.go's confirmationSender: a deferred value, not a
// plain package var, because mail.FromEnv reads SMTP_* from the
// environment, which main() loads from .env only after package-level
// initialization has already run.
var messageSender = sync.OnceValue(mail.FromEnv)

// queueMessage handles the Communications workspace's send form. For the
// "configured" provider it sends now, through messageSender (mail.FromEnv):
// the demo OutboxSender when no transport is configured, a Resend sender
// when RESEND_API_KEY is configured, or SMTP when SMTP_HOST is configured.
// The same code path is demonstrable today and correct once an organizer
// configures a provider. The
// "gmail" and "outlook" providers stay queue-only: the organizer finishes
// delivery themselves through this page's deep-link compose buttons, so no
// automated Send call belongs on that path.
//
// Every path records the outcome as a Communication row: "sent" or
// "failed" for an attempted send, "queued" for a hand-off provider. A
// failed send never stores the raw provider error on the row (M8) --
// log.Printf carries the detail, the row keeps a sanitized category.
func queueMessage(ctx *action.Context) error {
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	provider := strings.TrimSpace(ctx.FormData["provider"])
	if provider != "configured" && provider != "demo-outbox" && provider != "gmail" && provider != "outlook" {
		return action.Validation("Choose a delivery provider.", map[string]string{"provider": "Unknown provider."}, ctx.FormData)
	}

	snapshot := appstate.MustGet().Snapshot()
	speaker, found := snapshot.Speaker(speakerID)
	if !found {
		return fmt.Errorf("speaker %s not found", speakerID)
	}
	template, found := emailTemplate(snapshot, templateID)
	if !found {
		return fmt.Errorf("template %s not found", templateID)
	}
	sessionItem, hasSession := present.RecipientSession(snapshot, speaker.ID)
	subject, body := present.RenderCommunication(snapshot, template, *speaker, sessionItem)

	item := domain.Communication{
		ID:           domain.NewID("comm"),
		TemplateID:   template.ID,
		SpeakerID:    speaker.ID,
		Subject:      subject,
		Status:       "queued",
		Provider:     provider,
		ScheduledFor: time.Now().UTC().Add(5 * time.Minute),
	}
	if hasSession {
		item.SessionID = sessionItem.ID
	}

	if provider == "configured" || provider == "demo-outbox" {
		msg := mail.Message{To: speaker.Email, ToName: speaker.Name(), Subject: subject, TextBody: body, IdempotencyKey: item.ID}
		if template.AttachCalendar && hasSession && sessionItem.Scheduled() {
			ics, err := programcalendar.Invite(snapshot, sessionItem, *speaker, organizerEmail(template))
			if err != nil {
				// A missing schedule or address on the invite side never
				// blocks the message itself -- the speaker still gets the
				// merged template text, just without the attachment.
				log.Printf("communications: could not build the calendar invite for speaker %s: %v", speaker.ID, err)
			} else {
				msg.Calendar = ics
			}
		}

		sender := messageSender()
		sendErr := sender.Send(msg)
		item.SentAt = time.Now().UTC()
		if sendErr != nil {
			item.Status = "failed"
			item.Error = "delivery failed"
			log.Printf("communications: send to speaker %s failed: %v", speaker.ID, sendErr)
		} else {
			item.Status = "sent"
		}
		if named, ok := sender.(mail.Named); ok {
			item.Provider = named.Name()
		}
	}

	if err := appstate.MustGet().Update(func(state *domain.State) error {
		state.Communications = append(state.Communications, item)
		return nil
	}); err != nil {
		return err
	}

	// TODO(reminders): tick queued communications. A background scheduler
	// belongs here (or beside it): promote each "queued" row past its
	// ScheduledFor -- a gmail/outlook hand-off recorded above, or a
	// reminder template's row seeded by internal/domain/seed.go -- to
	// "sent" once its window arrives. Out of scope for this change.
	notice := "Queued the selected template for " + speaker.Name() + " via " + provider + "."
	switch item.Status {
	case "sent":
		notice = "Sent the selected template to " + speaker.Name() + " via " + item.Provider + "."
	case "failed":
		notice = "Could not send the selected template to " + speaker.Name() + ". Check the mail transport configuration."
	}
	session.AddFlash(ctx.Request, "notice", notice)
	live.Broadcast("communication:queued", map[string]string{"speaker": speakerID, "provider": provider})
	actionflow.Redirect(ctx, "/organizer/communications?template="+templateID)
	return nil
}

// emailTemplate finds the EmailTemplate named id in state, if any.
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
// process-wide MAIL_FROM address reduced to a bare address.
func organizerEmail(template domain.EmailTemplate) string {
	if replyTo := strings.TrimSpace(template.ReplyTo); replyTo != "" {
		return replyTo
	}
	return mail.AddressOnly(strings.TrimSpace(os.Getenv("MAIL_FROM")))
}
