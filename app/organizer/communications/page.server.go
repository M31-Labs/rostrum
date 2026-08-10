package communications

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/odvcencio/programma/internal/actionflow"
	"github.com/odvcencio/programma/internal/appstate"
	"github.com/odvcencio/programma/internal/domain"
	"github.com/odvcencio/programma/internal/live"
	"github.com/odvcencio/programma/internal/present"
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
			return server.Metadata{Title: server.Title{Default: "Communications — Programma"}, Description: "Template, schedule, and preview speaker communications and calendar invites."}, nil
		},
		Actions: route.FileActions{"queueMessage": queueMessage},
	}); err != nil {
		log.Fatal(err)
	}
}

func queueMessage(ctx *action.Context) error {
	templateID := strings.TrimSpace(ctx.FormData["template_id"])
	speakerID := strings.TrimSpace(ctx.FormData["speaker_id"])
	provider := strings.TrimSpace(ctx.FormData["provider"])
	if provider != "demo-outbox" && provider != "gmail" && provider != "outlook" {
		return action.Validation("Choose a delivery provider.", map[string]string{"provider": "Unknown provider."}, ctx.FormData)
	}
	name := ""
	if err := appstate.MustGet().Update(func(state *domain.State) error {
		speaker, found := state.Speaker(speakerID)
		if !found {
			return fmt.Errorf("speaker %s not found", speakerID)
		}
		var template *domain.EmailTemplate
		for index := range state.EmailTemplates {
			if state.EmailTemplates[index].ID == templateID {
				template = &state.EmailTemplates[index]
				break
			}
		}
		if template == nil {
			return fmt.Errorf("template %s not found", templateID)
		}
		name = speaker.Name()
		now := time.Now().UTC()
		status := "queued"
		if provider == "demo-outbox" {
			status = "sent"
		}
		item := domain.Communication{
			ID:           domain.NewID("comm"),
			TemplateID:   template.ID,
			SpeakerID:    speaker.ID,
			Subject:      template.Subject,
			Status:       status,
			Provider:     provider,
			ScheduledFor: now.Add(5 * time.Minute),
		}
		if status == "sent" {
			item.SentAt = now
		}
		state.Communications = append(state.Communications, item)
		return nil
	}); err != nil {
		return err
	}
	session.AddFlash(ctx.Request, "notice", "Queued the selected template for "+name+" via "+provider+".")
	live.Broadcast("communication:queued", map[string]string{"speaker": speakerID, "provider": provider})
	actionflow.Redirect(ctx, "/organizer/communications?template="+templateID)
	return nil
}
