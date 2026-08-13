package present

import (
	"strings"
	"testing"

	"github.com/m31-labs/rostrum/internal/domain"
)

func TestRenderCommunicationContextWithPortalURL(t *testing.T) {
	state := domain.State{Event: domain.Event{Name: "Open Systems Day"}}
	template := domain.EmailTemplate{
		Subject: "We received {{submission.title}}",
		Body:    "Hi {{speaker.first_name}}, open {{speaker.portal_url}}",
	}
	speaker := domain.Speaker{FirstName: "Ada", LastName: "Lovelace"}
	submission := domain.Submission{Title: "Inspectable compilers"}
	portalURL := "https://events.example/portal/spk_ada?key=signed-token"

	subject, body := RenderCommunicationContextWithPortalURL(state, template, speaker, domain.Session{}, submission, domain.Task{}, portalURL)
	if subject != "We received Inspectable compilers" {
		t.Fatalf("subject = %q", subject)
	}
	if body != "Hi Ada, open "+portalURL {
		t.Fatalf("body = %q", body)
	}
	if strings.Contains(subject+body, "{{") {
		t.Fatalf("render left a merge field behind: %q / %q", subject, body)
	}
}

func TestRenderCommunicationContextDoesNotInventPortalCredential(t *testing.T) {
	template := domain.EmailTemplate{Subject: "Receipt", Body: "Portal: {{speaker.portal_url}}"}
	_, body := RenderCommunicationContext(domain.State{}, template, domain.Speaker{}, domain.Session{}, domain.Submission{}, domain.Task{})
	if body != "Portal: " {
		t.Fatalf("ordinary render portal body = %q, want an empty credential", body)
	}
}
