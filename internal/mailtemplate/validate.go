// Package mailtemplate defines the safe, provider-neutral grammar shared by
// the organizer template editor and the communications renderer.
package mailtemplate

import (
	"fmt"
	stdmail "net/mail"
	"sort"
	"strings"
)

// AllowedMergeFields is the intentionally small vocabulary accepted in an
// organizer-authored template. Rendering only ever replaces these canonical
// values; unknown or malformed braces are rejected at save time instead of
// leaving a surprise token in a production email.
var AllowedMergeFields = map[string]bool{
	"{{event.name}}":         true,
	"{{speaker.first_name}}": true,
	"{{speaker.name}}":       true,
	"{{speaker.portal_url}}": true,
	"{{session.title}}":      true,
	"{{session.start_time}}": true,
	"{{session.room}}":       true,
	"{{submission.title}}":   true,
	"{{task.title}}":         true,
	"{{task.due_date}}":      true,
}

// Validate checks editable template content without altering it. Blank
// ReplyTo is allowed, but a supplied address is validated before it reaches a
// generated message.
func Validate(name, audience, subject, body, replyTo string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("template name is required")
	}
	if strings.TrimSpace(audience) == "" {
		return fmt.Errorf("template audience is required")
	}
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("template subject is required")
	}
	if strings.ContainsAny(subject, "\r\n") {
		return fmt.Errorf("template subject must be a single line")
	}
	if strings.TrimSpace(body) == "" {
		return fmt.Errorf("template body is required")
	}
	if len([]rune(subject)) > 240 || len([]rune(body)) > 20_000 {
		return fmt.Errorf("template content is too long")
	}
	if value := strings.TrimSpace(replyTo); value != "" {
		parsed, err := stdmail.ParseAddress(value)
		if err != nil || parsed.Address == "" {
			return fmt.Errorf("reply-to must be a valid email address")
		}
	}
	for _, value := range []string{subject, body} {
		if err := validateMergeFields(value); err != nil {
			return err
		}
	}
	return nil
}

func validateMergeFields(value string) error {
	for remaining := value; ; {
		start := strings.Index(remaining, "{{")
		if start < 0 {
			if strings.ContainsAny(remaining, "{}") {
				return fmt.Errorf("template contains malformed merge-field braces")
			}
			return nil
		}
		if strings.ContainsAny(remaining[:start], "{}") {
			return fmt.Errorf("template contains malformed merge-field braces")
		}
		rest := remaining[start+2:]
		end := strings.Index(rest, "}}")
		if end < 0 {
			return fmt.Errorf("template contains an unclosed merge field")
		}
		field := "{{" + rest[:end] + "}}"
		if !AllowedMergeFields[field] {
			return fmt.Errorf("template uses unsupported merge field %s", field)
		}
		remaining = rest[end+2:]
	}
}

func Fields() []string {
	fields := make([]string, 0, len(AllowedMergeFields))
	for field := range AllowedMergeFields {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}
