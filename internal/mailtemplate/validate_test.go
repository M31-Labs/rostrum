package mailtemplate

import "testing"

func TestValidateAllowsOnlyKnownMergeFields(t *testing.T) {
	if err := Validate("Reminder", "speaker", "Hi {{speaker.first_name}}", "{{task.title}} is due {{task.due_date}}. Open {{speaker.portal_url}}.", "program@example.com"); err != nil {
		t.Fatalf("known template rejected: %v", err)
	}
	for _, value := range []string{"Hello {{speaker.email}}", "Hello {{speaker.name}", "Hello {speaker.name}"} {
		if err := Validate("Bad", "speaker", value, "Body", ""); err == nil {
			t.Fatalf("invalid merge content %q was accepted", value)
		}
	}
}

func TestValidateRejectsMultilineSubject(t *testing.T) {
	if err := Validate("Bad", "speaker", "Proposal received\nBcc: attacker@example.com", "Body", ""); err == nil {
		t.Fatal("multiline subject was accepted")
	}
}
