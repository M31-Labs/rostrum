package present

import (
	"net/url"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/domain"
)

func Communications(state domain.State, templateID string) map[string]any {
	if templateID == "" && len(state.EmailTemplates) > 0 {
		templateID = state.EmailTemplates[1].ID
	}
	selected := state.EmailTemplates[0]
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

	sampleSpeaker := state.Speakers[0]
	sampleSession := state.Sessions[0]
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

	recipients := make([]map[string]string, 0, len(state.Speakers))
	for _, speaker := range state.Speakers {
		recipients = append(recipients, map[string]string{"id": speaker.ID, "name": speaker.Name(), "email": speaker.Email})
	}

	return map[string]any{
		"section":    "communications",
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
			"gmailURL":    gmail,
			"outlookURL":  outlook,
			"calendarURL": "/calendar/" + sampleSpeaker.ID + ".ics",
		},
	}
}

func renderCommunication(state domain.State, template domain.EmailTemplate, speaker domain.Speaker, item domain.Session) (string, string) {
	subject := template.Subject
	body := template.Body
	replacements := map[string]string{
		"{{event.name}}":         state.Event.Name,
		"{{speaker.first_name}}": speaker.FirstName,
		"{{speaker.name}}":       speaker.Name(),
		"{{session.title}}":      item.Title,
		"{{session.start_time}}": item.StartsAt.Format("Mon, Jan 02 at 15:04 MST"),
		"{{session.room}}":       RoomName(state, item.RoomID),
		"{{submission.title}}":   item.Title,
		"{{task.title}}":         "Confirm your public profile",
		"{{task.due_date}}":      time.Date(2026, time.September, 2, 17, 0, 0, 0, time.Local).Format("January 2"),
	}
	for key, value := range replacements {
		subject = strings.ReplaceAll(subject, key, value)
		body = strings.ReplaceAll(body, key, value)
	}
	return subject, body
}
