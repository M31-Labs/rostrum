package present

import (
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/m31-labs/rostrum/internal/domain"
	"github.com/m31-labs/rostrum/internal/mailtemplate"
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
			"id":         template.ID,
			"name":       template.Name,
			"audience":   StatusLabel(template.Audience),
			"audienceID": template.Audience,
			"subject":    template.Subject,
			"body":       template.Body,
			"replyTo":    template.ReplyTo,
			"calendar":   template.AttachCalendar,
			"system":     template.System,
			"class":      className,
		})
	}

	sampleSpeaker := selectRecipient(state, selectedRecipientID)
	sampleSession, hasSession := RecipientSession(state, sampleSpeaker.ID)
	if !hasSession {
		// Per spec: omit session placeholders rather than borrow an
		// unrelated speaker's session.
		sampleSession = domain.Session{}
	}
	subject, body := RenderCommunication(state, selected, sampleSpeaker, sampleSession)
	gmail := "https://mail.google.com/mail/?view=cm&fs=1&to=" + url.QueryEscape(sampleSpeaker.Email) + "&su=" + url.QueryEscape(subject) + "&body=" + url.QueryEscape(body)
	outlook := "https://outlook.office.com/mail/deeplink/compose?to=" + url.QueryEscape(sampleSpeaker.Email) + "&subject=" + url.QueryEscape(subject) + "&body=" + url.QueryEscape(body)

	outbox := make([]map[string]any, 0, len(state.Communications))
	queued, sent, failed, suppressed := 0, 0, 0, 0
	for index := len(state.Communications) - 1; index >= 0; index-- {
		item := state.Communications[index]
		when := item.ScheduledFor
		switch item.Status {
		case "sent":
			sent++
			when = item.SentAt
		case "failed":
			// A failed send was attempted, not queued: it counts toward
			// neither pill, but still shows its attempt time rather than
			// a stale ScheduledFor.
			when = item.SentAt
			failed++
		case domain.CommunicationSuppressed:
			suppressed++
			when = item.SuppressedAt
		default:
			queued++
		}
		recipient := strings.TrimSpace(item.RecipientEmail)
		if recipient == "" {
			recipient = SpeakerName(state, item.SpeakerID)
		}
		if recipient == "" {
			recipient = "Unavailable recipient"
		}
		canCancel := item.Status == domain.CommunicationQueued || item.Status == domain.CommunicationScheduled || item.Status == domain.CommunicationRetrying
		outbox = append(outbox, map[string]any{
			"id":        item.ID,
			"subject":   item.Subject,
			"speaker":   recipient,
			"status":    StatusLabel(item.Status),
			"tone":      StatusTone(item.Status),
			"provider":  item.Provider,
			"when":      DateTime(when),
			"attempts":  item.AttemptCount,
			"trigger":   item.Trigger,
			"canCancel": canCancel,
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

	revisions := templateRevisions(state, selected.ID)
	rules := notificationRuleRows(state)
	return map[string]any{
		"section":     "communications",
		"workspace":   WorkspaceIdentity(state),
		"templates":   templates,
		"recipients":  recipients,
		"outbox":      outbox,
		"counts":      map[string]any{"sent": sent, "queued": queued, "failed": failed, "suppressed": suppressed, "templates": len(templates)},
		"revisions":   revisions,
		"rules":       rules,
		"mergeFields": mailtemplate.Fields(),
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
			"audience":    selected.Audience,
			"bodySource":  selected.Body,
			"system":      selected.System,
		},
	}
}

func templateRevisions(state domain.State, templateID string) []map[string]any {
	rows := make([]domain.EmailTemplateRevision, 0)
	for _, revision := range state.TemplateRevisions {
		if revision.TemplateID == templateID {
			rows = append(rows, revision)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Revision > rows[j].Revision })
	result := make([]map[string]any, 0, len(rows))
	for _, revision := range rows {
		result = append(result, map[string]any{
			"id": revision.ID, "revision": revision.Revision, "actor": revision.Actor,
			"when": DateTime(revision.CreatedAt), "subject": revision.Subject,
		})
	}
	return result
}

func notificationRuleRows(state domain.State) []map[string]any {
	rows := make([]map[string]any, 0, len(state.NotificationRules))
	for _, rule := range state.NotificationRules {
		rows = append(rows, map[string]any{
			"id": rule.ID, "name": rule.Name, "trigger": rule.Trigger, "templateID": rule.TemplateID,
			"recipients": strings.Join(rule.RecipientEmails, ", "), "enabled": rule.Enabled,
			"retryLimit": rule.RetryLimit, "suppressMinutes": rule.SuppressMinutes,
		})
	}
	return rows
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

// RecipientSession returns the recipient's first session by membership
// (matching on Session.SpeakerIDs), so a preview or a merged send never
// carries a session placeholder that belongs to a different speaker.
// Exported so app/organizer/communications/page.server.go's queueMessage
// action can resolve the same session the preview pane already shows,
// rather than duplicating this lookup at the call site.
func RecipientSession(state domain.State, speakerID string) (domain.Session, bool) {
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

// RenderCommunication merges template's Subject and Body with speaker and
// item (the speaker's session, or a zero Session when none applies) and
// returns the merged subject and body. Exported so a real send path
// (app/organizer/communications/page.server.go's queueMessage, and the
// submission-acceptance flow in app/organizer/submissions/page.server.go)
// composes the exact text the preview pane already renders, instead of
// duplicating the merge-field map at each call site.
func RenderCommunication(state domain.State, template domain.EmailTemplate, speaker domain.Speaker, item domain.Session) (string, string) {
	return RenderCommunicationContext(state, template, speaker, item, domain.Submission{}, domain.Task{})
}

// RenderCommunicationContext is the deterministic merge engine shared by
// previews, manual messages, reminders, and notification-triggered outbox
// work. Its inputs are canonical records rather than request values, so a
// persisted Communication can be safely retried after a process restart.
func RenderCommunicationContext(state domain.State, template domain.EmailTemplate, speaker domain.Speaker, item domain.Session, submission domain.Submission, task domain.Task) (string, string) {
	return RenderCommunicationContextWithPortalURL(state, template, speaker, item, submission, task, "")
}

// RenderCommunicationContextWithPortalURL renders the same canonical merge
// context as RenderCommunicationContext and supplies the signed, absolute
// speaker portal URL that only a delivery boundary can safely construct.
// Keeping that capability explicit prevents previews and unrelated manual
// messages from inventing an unkeyed portal link.
func RenderCommunicationContextWithPortalURL(state domain.State, template domain.EmailTemplate, speaker domain.Speaker, item domain.Session, submission domain.Submission, task domain.Task, portalURL string) (string, string) {
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
	submissionTitle := submission.Title
	if submissionTitle == "" {
		submissionTitle = sessionTitle
	}
	taskTitle := task.Title
	if taskTitle == "" {
		taskTitle = "Confirm your public profile"
	}
	taskDue := ""
	if !task.DueAt.IsZero() {
		taskDue = task.DueAt.Format("January 2")
	}
	if taskDue == "" {
		taskDue = time.Date(2026, time.September, 2, 17, 0, 0, 0, time.Local).Format("January 2")
	}
	replacements := map[string]string{
		"{{event.name}}":         state.Event.Name,
		"{{speaker.first_name}}": speaker.FirstName,
		"{{speaker.name}}":       speaker.Name(),
		"{{speaker.portal_url}}": strings.TrimSpace(portalURL),
		"{{session.title}}":      sessionTitle,
		"{{session.start_time}}": sessionStart,
		"{{session.room}}":       sessionRoom,
		"{{submission.title}}":   submissionTitle,
		"{{task.title}}":         taskTitle,
		"{{task.due_date}}":      taskDue,
	}
	for key, value := range replacements {
		subject = strings.ReplaceAll(subject, key, value)
		body = strings.ReplaceAll(body, key, value)
	}
	return subject, body
}
