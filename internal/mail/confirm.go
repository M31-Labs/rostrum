package mail

import (
	"fmt"
	"strings"
)

// Recipient is the speaker a confirmation email addresses. It carries only
// what this package needs, so mail never depends on internal/domain; a
// caller maps a domain.Speaker into a Recipient at the call site.
type Recipient struct {
	SpeakerID string // used to build the portal link, for example "spk_01"
	Name      string
	Email     string
}

// Submission is the proposal a confirmation email describes. It carries
// only what this package needs; a caller maps a domain.Submission into one
// at the call site.
type Submission struct {
	Title string
}

// SendConfirmation renders and sends the submission-confirmation email: it
// thanks the speaker by name, names the submitted title, and links to
// their portal at PortalURL(base, speaker.SpeakerID, portalKey).
//
// Call this after the durable mutation (the store Update that records the
// submission) has completed and committed, and outside any store lock:
// Send is a network operation and must never run while a lock is held.
// Pass portalKey from token.New().Sign(speaker.SpeakerID) so the link opens
// the portal without a separate login step.
//
// The one call an integrator wires into submitProposal, after
// appstate.MustGet().Update(...) returns nil and before the redirect:
//
//	err := mail.SendConfirmation(sender, publicBaseURL, mail.Recipient{
//		SpeakerID: speakerID,
//		Name:      speaker.Name(),
//		Email:     speaker.Email,
//	}, mail.Submission{Title: submission.Title}, token.New().Sign(speakerID))
//
// Record the outcome as a Communication row: Status "sent" on a nil error,
// "failed" with a sanitized category otherwise (never the raw err text).
func SendConfirmation(sender Sender, base string, speaker Recipient, submission Submission, portalKey string) error {
	if sender == nil {
		return fmt.Errorf("mail: SendConfirmation requires a Sender")
	}
	msg := Message{
		To:       speaker.Email,
		ToName:   speaker.Name,
		Subject:  confirmationSubject(submission),
		TextBody: confirmationBody(base, speaker, submission, portalKey),
	}
	return sender.Send(msg)
}

// PortalURL builds the portal link a confirmation email, or any other
// speaker-facing notice, should carry: {base}/portal/{speakerID}?key={portalKey}.
// base may or may not carry a trailing slash; PortalURL normalizes it.
func PortalURL(base, speakerID, portalKey string) string {
	return strings.TrimRight(base, "/") + "/portal/" + speakerID + "?key=" + portalKey
}

func confirmationSubject(submission Submission) string {
	title := strings.TrimSpace(submission.Title)
	if title == "" {
		return "We received your proposal"
	}
	return "We received " + title
}

func confirmationBody(base string, speaker Recipient, submission Submission, portalKey string) string {
	name := strings.TrimSpace(speaker.Name)
	if name == "" {
		name = "there"
	}
	title := strings.TrimSpace(submission.Title)
	if title == "" {
		title = "your proposal"
	}
	var body strings.Builder
	fmt.Fprintf(&body, "Hi %s,\n\n", name)
	fmt.Fprintf(&body, "Thanks for submitting %q. We received it and it is now in review.\n\n", title)
	fmt.Fprintf(&body, "Track your submission and complete any speaker tasks from your portal:\n%s\n\n", PortalURL(base, speaker.SpeakerID, portalKey))
	body.WriteString("See you there,\nThe program team\n")
	return body.String()
}
