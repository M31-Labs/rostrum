package present

import (
	"net/url"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
)

// Communications renders the communications workspace: the template
// library, the merge preview, the outbox, and the recipient picker.
//
// recipientID selects which speaker's address feeds the preview and the
// Gmail/Outlook compose deep links; pass "" (or omit it) to fall back to
// the first available speaker. It is variadic so existing single-recipient
// call sites stay source compatible; callers that resolve a recipient from
// a request (for example a "recipient" query parameter) should pass it as
// the third argument.
func Communications(state domain.State, templateID string, recipientID ...string) map[string]any {
	selectedRecipientID := ""
	if len(recipientID) > 0 {
		selectedRecipientID = recipientID[0]
	}

	if templateID == "" && len(state.EmailTemplates) > 0 {
		templateID = state.EmailTemplates[0].ID
	}
	var selected domain.EmailTemplate
	if len(state.EmailTemplates) > 0 {
		selected = state.EmailTemplates[0]
	}
	for _, template := range state.EmailTemplates {
		if template.ID == templateID {
			selected = template
		}
	}
	templateID = selected.ID
	templates := make([]map[string]any, 0, len(state.EmailTemplates))
	for _, template := range state.EmailTemplates {
		className := "template-item"
		if template.ID == templateID {
			className += " active"
		}
		templates = append(templates, map[string]any{
			"id":       template.ID,
			"name":     template.Name,
			"audience": StatusLabel(template.Audience),
			"subject":  template.Subject,
			"calendar": template.AttachCalendar,
			"class":    className,
		})
	}

	sampleSpeaker := selectRecipient(state, selectedRecipientID)
	sampleSession, hasSession := recipientSession(state, sampleSpeaker.ID)
	if !hasSession {
		// Per spec: omit session placeholders rather than borrow an
		// unrelated speaker's session.
		sampleSession = domain.Session{}
	}
	subject, body := renderCommunication(state, selected, sampleSpeaker, sampleSession)
	gmail := "https://mail.google.com/mail/?view=cm&fs=1&to=" + url.QueryEscape(sampleSpeaker.Email) + "&su=" + url.QueryEscape(subject) + "&body=" + url.QueryEscape(body)
	outlook := "https://outlook.office.com/mail/deeplink/compose?to=" + url.QueryEscape(sampleSpeaker.Email) + "&subject=" + url.QueryEscape(subject) + "&body=" + url.QueryEscape(body)

	outbox := make([]map[string]any, 0, len(state.Communications))
	queued, sent := 0, 0
	for index := len(state.Communications) - 1; index >= 0; index-- {
		item := state.Communications[index]
		when := item.ScheduledFor
		if item.Status == "sent" {
			sent++
			when = item.SentAt
		} else {
			queued++
		}
		outbox = append(outbox, map[string]any{
			"id":       item.ID,
			"subject":  item.Subject,
			"speaker":  SpeakerName(state, item.SpeakerID),
			"status":   StatusLabel(item.Status),
			"tone":     StatusTone(item.Status),
			"provider": item.Provider,
			"when":     DateTime(when),
		})
	}

	recipients := make([]map[string]any, 0, len(state.Speakers))
	for _, speaker := range state.Speakers {
		className := "template-item"
		if speaker.ID == sampleSpeaker.ID {
			className += " active"
		}
		recipients = append(recipients, map[string]any{
			"id":    speaker.ID,
			"name":  speaker.Name(),
			"email": speaker.Email,
			"class": className,
		})
	}

	return map[string]any{
		"section":    "communications",
		"demoMode":   DemoMode(),
		"workspace":  WorkspaceIdentity(state),
		"templates":  templates,
		"recipients": recipients,
		"outbox":     outbox,
		"counts":     map[string]any{"sent": sent, "queued": queued, "templates": len(templates)},
		"preview": map[string]any{
			"id":          selected.ID,
			"name":        selected.Name,
			"subject":     subject,
			"body":        body,
			"replyTo":     selected.ReplyTo,
			"calendar":    selected.AttachCalendar,
			"speaker":     sampleSpeaker.Name(),
			"email":       sampleSpeaker.Email,
			"recipientId": sampleSpeaker.ID,
			"gmailURL":    gmail,
			"outlookURL":  outlook,
			"calendarURL": "/calendar/" + sampleSpeaker.ID + ".ics",
		},
	}
}

// selectRecipient resolves the sample speaker for the preview pane: the
// requested recipientID when it names a real speaker in the workspace,
// otherwise the first speaker, otherwise a zero Speaker so a workspace with
// no speakers renders instead of panicking on an unguarded index.
func selectRecipient(state domain.State, recipientID string) domain.Speaker {
	if recipientID != "" {
		if found, ok := state.Speaker(recipientID); ok {
			return *found
		}
	}
	if len(state.Speakers) > 0 {
		return state.Speakers[0]
	}
	return domain.Speaker{}
}

// recipientSession returns the recipient's first session by membership
// (matching on Session.SpeakerIDs), so the preview never shows a session
// placeholder that belongs to a different speaker.
func recipientSession(state domain.State, speakerID string) (domain.Session, bool) {
	if speakerID == "" {
		return domain.Session{}, false
	}
	for _, session := range state.Sessions {
		for _, id := range session.SpeakerIDs {
			if id == speakerID {
				return session, true
			}
		}
	}
	return domain.Session{}, false
}

func renderCommunication(state domain.State, template domain.EmailTemplate, speaker domain.Speaker, item domain.Session) (string, string) {
	subject := template.Subject
	body := template.Body
	sessionTitle, sessionStart, sessionRoom := "", "", ""
	if item.ID != "" {
		sessionTitle = item.Title
		sessionRoom = RoomName(state, item.RoomID)
		if !item.StartsAt.IsZero() {
			sessionStart = item.StartsAt.Format("Mon, Jan 02 at 15:04 MST")
		}
	}
	replacements := map[string]string{
		"{{event.name}}":         state.Event.Name,
		"{{speaker.first_name}}": speaker.FirstName,
		"{{speaker.name}}":       speaker.Name(),
		"{{session.title}}":      sessionTitle,
		"{{session.start_time}}": sessionStart,
		"{{session.room}}":       sessionRoom,
		"{{submission.title}}":   sessionTitle,
		"{{task.title}}":         "Confirm your public profile",
		"{{task.due_date}}":      time.Date(2026, time.September, 2, 17, 0, 0, 0, time.Local).Format("January 2"),
	}
	for key, value := range replacements {
		subject = strings.ReplaceAll(subject, key, value)
		body = strings.ReplaceAll(body, key, value)
	}
	return subject, body
}
