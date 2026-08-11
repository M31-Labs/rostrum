package domain

import "time"

// Seed returns a complete demo workspace. The records intentionally include a
// few incomplete tasks and schedule collisions so the primary workflows have
// visible, testable state on first run.
func Seed(now time.Time) State {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		location = time.FixedZone("PDT", -7*60*60)
	}
	at := func(year int, month time.Month, day, hour, minute int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, location)
	}

	acceptedSpeakers := []string{
		"spk_maya", "spk_theo", "spk_lina", "spk_priya",
		"spk_samira", "spk_elliot", "spk_noor", "spk_ben",
	}

	state := State{
		SchemaVersion: CurrentSchemaVersion,
		Event: Event{
			ID:          "evt_m31_forum_2026",
			Name:        "M31 Systems Forum 2026",
			Slug:        "m31-systems-forum-2026",
			Type:        "Conference",
			WebsiteURL:  "https://m31labs.dev",
			Location:    "San Francisco, California",
			TimeZone:    "America/Los_Angeles",
			StartsAt:    at(2026, time.October, 14, 9, 0),
			EndsAt:      at(2026, time.October, 16, 18, 0),
			Theme:       "Tools for governed, collaborative intelligence",
			Description: "Three days for builders making agents, interfaces, and infrastructure that remain understandable under pressure.",
			Tracks: []Track{
				{ID: "track-agents", Name: "Agents in Production", Color: "blue", Description: "Agent runtime, evaluation, memory, and operations."},
				{ID: "track-governance", Name: "Governed Systems", Color: "teal", Description: "Rules, safety, provenance, and accountable outcomes."},
				{ID: "track-interfaces", Name: "New Interfaces", Color: "violet", Description: "Human-agent interaction across web, terminal, and spatial surfaces."},
				{ID: "track-infra", Name: "Open Infrastructure", Color: "ochre", Description: "Databases, models, compilers, and deployment foundations."},
			},
			Rooms: []Room{
				{ID: "room-meridian", Name: "Meridian Hall", Capacity: 420},
				{ID: "room-kepler", Name: "Kepler Room", Capacity: 180},
				{ID: "room-tycho", Name: "Tycho Lab", Capacity: 110},
				{ID: "room-atrium", Name: "Atrium Stage", Capacity: 260},
			},
			Categories: []Category{
				{ID: "agents", Name: "Agents in Production", OwnerName: "Maya Chen", OwnerEmail: "maya@example.com", TrackID: "track-agents"},
				{ID: "governance", Name: "Governed Systems", OwnerName: "Theo Okafor", OwnerEmail: "theo@example.com", TrackID: "track-governance"},
				{ID: "interfaces", Name: "New Interfaces", OwnerName: "Lina Martinez", OwnerEmail: "lina@example.com", TrackID: "track-interfaces"},
				{ID: "infrastructure", Name: "Open Infrastructure", OwnerName: "Priya Nair", OwnerEmail: "priya@example.com", TrackID: "track-infra"},
			},
			Formats: []string{"Talk", "Panel", "Workshop", "Lightning talk"},
			Levels:  []string{"Introductory", "Intermediate", "Advanced"},
		},
		Forms: []SubmissionForm{
			{
				ID:                   "form_cfp_2026",
				EventID:              "evt_m31_forum_2026",
				Name:                 "2026 Call for Speakers",
				ExternalTitle:        "Build what comes after the chatbot",
				Slug:                 "systems-forum-cfp",
				Kind:                 "abstract",
				Status:               "open",
				WelcomeHeading:       "Bring us the system behind the demo",
				WelcomeBody:          "We want evidence-rich sessions from people building dependable agents, interfaces, governed decisions, and open infrastructure. Proposals are reviewed in two rounds.",
				CloseAt:              at(2026, time.August, 28, 23, 59),
				RedirectToPortal:     true,
				SendConfirmation:     true,
				ConfirmationTemplate: "tpl_submission_confirmation",
				RuleFile:             "rules/cfp-routing.arb",
				Fields: []FormField{
					{ID: "title", Section: "proposal", Label: "Session title", Type: "text", Required: true, Locked: true, Placeholder: "A concrete title that tells us what changes", MaxLength: 120},
					{ID: "abstract", Section: "proposal", Label: "Abstract", Type: "textarea", Required: true, Help: "Describe the problem, the approach, and what attendees will be able to do afterward.", MaxLength: 1600},
					{ID: "format", Section: "proposal", Label: "Format", Type: "select", Required: true, Options: []string{"Talk", "Panel", "Workshop", "Lightning talk"}},
					{ID: "category", Section: "proposal", Label: "Category", Type: "select", Required: true, Options: []string{"Agents in Production", "Governed Systems", "New Interfaces", "Open Infrastructure"}},
					{ID: "level", Section: "proposal", Label: "Audience level", Type: "select", Required: true, Options: []string{"Introductory", "Intermediate", "Advanced"}},
					{ID: "topics", Section: "proposal", Label: "Three takeaways", Type: "textarea", Required: true, Help: "One takeaway per line.", MaxLength: 500},
					{ID: "workshop_needs", Section: "proposal", Label: "Workshop logistics", Type: "textarea", Required: false, Help: "Tell us about room setup, facilitation, materials, and participant limits.", MaxLength: 800},
					{ID: "first_name", Section: "participant", Label: "First name", Type: "text", Required: true, Locked: true, MaxLength: 80},
					{ID: "last_name", Section: "participant", Label: "Last name", Type: "text", Required: true, Locked: true, MaxLength: 80},
					{ID: "email", Section: "participant", Label: "Email", Type: "email", Required: true, Locked: true, MaxLength: 160},
					{ID: "role", Section: "participant", Label: "Role", Type: "text", Required: false, MaxLength: 120},
					{ID: "company", Section: "participant", Label: "Company or project", Type: "text", Required: false, MaxLength: 120},
					{ID: "biography", Section: "participant", Label: "Biography", Type: "textarea", Required: false, Help: "You can finish this in the portal after submission.", MaxLength: 800},
				},
				QuestionRules: []QuestionRule{
					{ID: "rule_workshop_needs", SourceFieldID: "format", Operator: "equals", Value: "Workshop", TargetFieldID: "workshop_needs", Effect: "show", Description: "Collect logistics only for workshop proposals."},
				},
			},
		},
		Speakers: []Speaker{
			{ID: "spk_maya", FirstName: "Maya", LastName: "Chen", Email: "maya@example.com", Pronouns: "she/her", Role: "Principal systems engineer", Company: "Northstar Research", Biography: "Maya builds long-running agent systems with observable memory and failure recovery.", LinkedInURL: "https://www.linkedin.com", WebsiteURL: "https://example.com/maya", City: "Vancouver", CreatedAt: now.Add(-65 * 24 * time.Hour), UpdatedAt: now.Add(-3 * 24 * time.Hour)},
			{ID: "spk_theo", FirstName: "Theo", LastName: "Okafor", Email: "theo@example.com", Pronouns: "he/him", Role: "Decision systems lead", Company: "Common Thread", Biography: "Theo designs governed decision systems that product teams can inspect and change safely.", LinkedInURL: "https://www.linkedin.com", City: "London", CreatedAt: now.Add(-62 * 24 * time.Hour), UpdatedAt: now.Add(-9 * time.Hour)},
			{ID: "spk_lina", FirstName: "Lina", LastName: "Martinez", Email: "lina@example.com", Pronouns: "she/they", Role: "Staff product engineer", Company: "Fathom Tools", Biography: "Lina works on server-first interfaces and interaction models that do not begin with a JavaScript application.", WebsiteURL: "https://example.com/lina", City: "Madrid", CreatedAt: now.Add(-58 * 24 * time.Hour), UpdatedAt: now.Add(-4 * 24 * time.Hour)},
			{ID: "spk_priya", FirstName: "Priya", LastName: "Nair", Email: "priya@example.com", Pronouns: "she/her", Role: "Infrastructure founder", Company: "Needlepoint", Biography: "Priya builds compact retrieval infrastructure for systems that must work at the edge.", LinkedInURL: "https://www.linkedin.com", WebsiteURL: "https://example.com/priya", City: "Singapore", CreatedAt: now.Add(-55 * 24 * time.Hour), UpdatedAt: now.Add(-18 * time.Hour)},
			{ID: "spk_samira", FirstName: "Samira", LastName: "Haddad", Email: "samira@example.com", Pronouns: "she/her", Role: "Evaluation researcher", Company: "Signal & Measure", Biography: "Samira studies how teams evaluate model and agent behavior under realistic operating constraints.", WebsiteURL: "https://example.com/samira", City: "Toronto", CreatedAt: now.Add(-48 * 24 * time.Hour), UpdatedAt: now.Add(-7 * 24 * time.Hour)},
			{ID: "spk_jonah", FirstName: "Jonah", LastName: "Williams", Email: "jonah@example.com", Pronouns: "he/they", Role: "Collaboration engineer", Company: "Coastline", Biography: "Jonah builds shared workspaces for people and agents.", City: "Oakland", CreatedAt: now.Add(-42 * 24 * time.Hour), UpdatedAt: now.Add(-42 * 24 * time.Hour)},
			{ID: "spk_noor", FirstName: "Noor", LastName: "Al-Sayed", Email: "noor@example.com", Pronouns: "they/them", Role: "Applied AI lead", Company: "Field Notes", Biography: "Noor turns evaluation rubrics into repeatable model-assisted review workflows.", LinkedInURL: "https://www.linkedin.com", City: "New York", CreatedAt: now.Add(-38 * 24 * time.Hour), UpdatedAt: now.Add(-2 * 24 * time.Hour)},
			{ID: "spk_elliot", FirstName: "Elliot", LastName: "Park", Email: "elliot@example.com", Pronouns: "he/him", Role: "GPU compiler engineer", Company: "Prism Forge", Biography: "Elliot works on practical compiler and GPU paths for teams that prefer Go.", WebsiteURL: "https://example.com/elliot", City: "Seoul", CreatedAt: now.Add(-34 * 24 * time.Hour), UpdatedAt: now.Add(-8 * 24 * time.Hour)},
			{ID: "spk_grace", FirstName: "Grace", LastName: "Kim", Email: "grace@example.com", Pronouns: "she/her", Role: "Developer experience lead", Company: "Lantern", Biography: "Grace helps infrastructure teams make complex tooling easier to learn.", City: "Seattle", CreatedAt: now.Add(-28 * 24 * time.Hour), UpdatedAt: now.Add(-13 * 24 * time.Hour)},
			{ID: "spk_ben", FirstName: "Ben", LastName: "Ito", Email: "ben@example.com", Pronouns: "he/him", Role: "Community programs director", Company: "Open Assembly", Biography: "Ben runs contributor programs and speaker operations for open-source events.", LinkedInURL: "https://www.linkedin.com", City: "Portland", CreatedAt: now.Add(-24 * 24 * time.Hour), UpdatedAt: now.Add(-20 * time.Hour)},
		},
		Reviewers: []Reviewer{
			{ID: "rev_ada", Name: "Ada Romero", Email: "ada@example.com", Company: "Tidal Systems", Expertise: []string{"agents", "evaluation"}, Kind: "human"},
			{ID: "rev_marcus", Name: "Marcus Lee", Email: "marcus@example.com", Company: "Forgeworks", Expertise: []string{"infrastructure", "compilers"}, Kind: "human"},
			{ID: "rev_ines", Name: "Ines Silva", Email: "ines@example.com", Company: "Civic Interface Lab", Expertise: []string{"interfaces", "accessibility"}, Kind: "human"},
			{ID: "rev_virtual_practitioner", Name: "Practitioner lens", Email: "", Expertise: []string{"applicability", "evidence"}, Kind: "virtual"},
		},
		ReviewPlans: []ReviewPlan{
			{
				ID: "plan_round_one", Name: "Round 1 - editorial screen", Round: 1, Status: "closed",
				Instructions: "Score the proposal as written. Reward evidence and a clear audience outcome.",
				DueAt:        at(2026, time.August, 7, 17, 0), Anonymous: true, WeeklyReminders: true,
				ReviewerIDs:   []string{"rev_ada", "rev_marcus", "rev_ines"},
				SubmissionIDs: []string{"sub_memory", "sub_rules", "sub_browser", "sub_vectors", "sub_eval", "sub_workspace", "sub_ai_review", "sub_gpu", "sub_demo", "sub_portals", "sub_open_models", "sub_accessibility"},
				Criteria: []RubricCriterion{
					{ID: "relevance", Name: "Relevance", Description: "Fit for the event audience and theme.", Weight: 30, MaxScore: 5},
					{ID: "clarity", Name: "Clarity", Description: "Problem, approach, and takeaway are specific.", Weight: 25, MaxScore: 5},
					{ID: "originality", Name: "Originality", Description: "Adds a useful perspective beyond common guidance.", Weight: 25, MaxScore: 5},
					{ID: "applicability", Name: "Applicability", Description: "Attendees can use the material after the event.", Weight: 20, MaxScore: 5},
				},
				EvaluationsPerItem: 2,
			},
			{
				ID: "plan_round_two", Name: "Round 2 - program fit", Round: 2, Status: "open",
				Instructions: "Balance the full program. Prefer concrete evidence, complementary viewpoints, and responsible claims.",
				DueAt:        at(2026, time.August, 22, 17, 0), Anonymous: false, WeeklyReminders: true, IncludeFiles: true,
				ReviewerIDs:   []string{"rev_ada", "rev_marcus", "rev_ines", "rev_virtual_practitioner"},
				SubmissionIDs: []string{"sub_eval", "sub_workspace", "sub_ai_review", "sub_open_models", "sub_accessibility"},
				Criteria: []RubricCriterion{
					{ID: "program_fit", Name: "Program fit", Description: "Strengthens the program as a whole.", Weight: 35, MaxScore: 5},
					{ID: "evidence", Name: "Evidence", Description: "Claims are supported by lived or measured results.", Weight: 25, MaxScore: 5},
					{ID: "novelty", Name: "Novelty", Description: "Offers a perspective not already represented.", Weight: 20, MaxScore: 5},
					{ID: "audience_value", Name: "Audience value", Description: "Creates an actionable attendee outcome.", Weight: 20, MaxScore: 5},
				},
				EvaluationsPerItem: 3,
			},
		},
		Tasks: []Task{
			{ID: "task_profile", Title: "Confirm your public profile", Description: "Review your role, company, biography, pronouns, and links.", Type: "profile", Required: true, DueAt: at(2026, time.September, 2, 17, 0), AssignedSpeakerIDs: acceptedSpeakers, AcceptedOnly: true},
			{ID: "task_headshot", Title: "Add a high-resolution headshot", Description: "Provide a square or portrait image at least 1200 pixels wide.", Type: "file", Required: true, DueAt: at(2026, time.September, 2, 17, 0), AssignedSpeakerIDs: acceptedSpeakers, AcceptedOnly: true},
			{ID: "task_agreement", Title: "Accept the speaker agreement", Description: "Review recording, conduct, and cancellation terms.", Type: "form", Required: true, DueAt: at(2026, time.September, 10, 17, 0), AssignedSpeakerIDs: acceptedSpeakers, AcceptedOnly: true, FormFields: []FormField{{ID: "agreement", Label: "I accept the speaker agreement", Type: "checkbox", Required: true}}},
			{ID: "task_av", Title: "Tell us about your AV needs", Description: "Share microphones, adapters, demos, and accessibility requirements.", Type: "form", Required: true, DueAt: at(2026, time.September, 15, 17, 0), AssignedSpeakerIDs: acceptedSpeakers, AcceptedOnly: true, FormFields: []FormField{{ID: "microphone", Label: "Microphone preference", Type: "select", Required: true, Options: []string{"Lavalier", "Handheld", "Lectern"}}, {ID: "demo", Label: "Will you run a live demo?", Type: "checkbox"}, {ID: "notes", Label: "Other requirements", Type: "textarea"}}},
			{ID: "task_slides", Title: "Upload presentation slides", Description: "PDF is preferred. Final revisions remain open through event week.", Type: "file", Required: true, DueAt: at(2026, time.October, 5, 17, 0), AssignedSpeakerIDs: acceptedSpeakers, AcceptedOnly: true},
		},
		Resources: []Resource{
			{ID: "res_guide", Title: "Speaker field guide", Kind: "article", Summary: "Arrival, green room, stage timing, and the people who can help.", Body: "Plan to arrive 45 minutes before your session. Check in at the speaker desk, then meet your room producer for sound and display checks.", SortOrder: 1},
			{ID: "res_av", Title: "AV and demo checklist", Kind: "article", Summary: "A short reliability checklist for live demos and presentation files.", Body: "Bring a local copy of every demo, use a 16:9 canvas, embed fonts, and keep a PDF fallback. Network access is available but never guaranteed.", SortOrder: 2},
			{ID: "res_calendar", Title: "Venue and production calendar", Kind: "link", Summary: "Shared production milestones and onsite windows.", URL: "https://calendar.google.com", SortOrder: 3},
			{ID: "res_reference", Title: "Program reference walkthrough", Kind: "embed", Summary: "A walkthrough embedded from an allowlisted video origin.", EmbedURL: "https://www.youtube-nocookie.com/embed/vUuK4Knl7oc", SortOrder: 4},
		},
		EmailTemplates: []EmailTemplate{
			{ID: "tpl_submission_confirmation", Name: "Submission confirmation", Audience: "submitter", Subject: "We received {{submission.title}}", Body: "Hi {{speaker.first_name}},\n\nYour proposal, {{submission.title}}, is safely in our review queue. You can update your profile and follow progress in the speaker portal.\n\nProgram team", ReplyTo: "program@example.com", System: true},
			{ID: "tpl_acceptance", Name: "Acceptance and next steps", Audience: "speaker", Subject: "You're joining {{event.name}}", Body: "Hi {{speaker.first_name}},\n\nWe would love to include {{session.title}} in {{event.name}}. Your portal contains the profile, agreement, AV, and slide tasks. A calendar invite is attached.\n\nProgram team", ReplyTo: "program@example.com", AttachCalendar: true, System: true},
			{ID: "tpl_five_day", Name: "Five-day task reminder", Audience: "speaker", Subject: "A speaker task is due in five days", Body: "Hi {{speaker.first_name}},\n\n{{task.title}} is due on {{task.due_date}}. Open your portal to finish it.\n\nProgram team", ReplyTo: "program@example.com", System: true},
			{ID: "tpl_one_day", Name: "One-day session reminder", Audience: "speaker", Subject: "Tomorrow: {{session.title}}", Body: "Hi {{speaker.first_name}},\n\nYour session begins at {{session.start_time}} in {{session.room}}. Please arrive 45 minutes early. The calendar update is attached.\n\nProgram team", ReplyTo: "program@example.com", AttachCalendar: true, System: true},
		},
		Integrations: []Integration{
			{ID: "integration_accelevents", Kind: "accelevents", Name: "Accelevents", Enabled: false, EventURL: "m31-systems-forum-2026", CredentialsOK: false, LastStatus: "Ready for dry run"},
			{ID: "integration_airtable", Kind: "airtable", Name: "Airtable", Enabled: false, CredentialsOK: false, LastStatus: "Configure a Personal Access Token and base ID"},
		},
		UpdatedAt: now,
	}

	state.Submissions = seedSubmissions(now)
	state.Evaluations = seedEvaluations(now)
	state.Sessions = seedSessions(at)
	state.TaskCompletions = seedTaskCompletions(now)
	state.Communications = seedCommunications(now, at)
	state.SyncRuns = []SyncRun{
		{ID: "sync_seed_preview", Integration: "integration_accelevents", Mode: "dry-run", Status: "complete", Speakers: 8, Sessions: 8, StartedAt: now.Add(-26 * time.Hour), FinishedAt: now.Add(-26*time.Hour + 2*time.Second), Summary: "Validated 8 speaker payloads and 8 session payloads; no external requests sent."},
	}
	return state
}

func seedSubmissions(now time.Time) []Submission {
	rows := []struct {
		id, title, abstract, format, category, track, level, status, queue, owner string
		speakers                                                                  []string
		daysAgo                                                                   int
	}{
		{"sub_memory", "Memory Without Mystery: Auditable Context for Long-Running Agents", "A field report on durable context, selective recall, and the traces operators need when an agent changes its mind.", "Talk", "agents", "track-agents", "Advanced", SubmissionAccepted, "agents-in-production", "Maya Chen", []string{"spk_maya"}, 51},
		{"sub_rules", "Rules That Can Explain Themselves", "How to move changing business decisions out of conditionals and into a governed language without losing performance.", "Talk", "governance", "track-governance", "Intermediate", SubmissionAccepted, "governed-systems", "Theo Okafor", []string{"spk_theo"}, 49},
		{"sub_browser", "The Browser Is a Render Target", "A server-first interface architecture that adds islands, engines, and collaboration only when the interaction earns them.", "Talk", "interfaces", "track-interfaces", "Intermediate", SubmissionAccepted, "new-interfaces", "Lina Martinez", []string{"spk_lina"}, 46},
		{"sub_vectors", "Vectors at the Edge: Compression Without Regret", "Practical vector compression, prepared query scoring, and quality measurements for constrained deployments.", "Talk", "infrastructure", "track-infra", "Advanced", SubmissionAccepted, "open-infrastructure", "Priya Nair", []string{"spk_priya"}, 44},
		{"sub_eval", "Evaluation Plans That Survive Contact with Reality", "A workshop for designing multi-round evaluation that gives reviewers a usable rubric and program teams a defensible decision trail.", "Workshop", "agents", "track-agents", "Intermediate", SubmissionAcceptedQueue, "agents-in-production", "Maya Chen", []string{"spk_samira"}, 39},
		{"sub_workspace", "When Agents Share a Workspace", "Patterns for presence, shared findings, convergence, and the moments when a plain server-owned workflow is still better.", "Talk", "interfaces", "track-interfaces", "Advanced", SubmissionPending, "new-interfaces", "Lina Martinez", []string{"spk_jonah"}, 35},
		{"sub_ai_review", "AI Reviewers Need Rubrics, Not Vibes", "A practical guide to using model-assisted proposal review as a second opinion with criteria, provenance, and human control.", "Talk", "governance", "track-governance", "Intermediate", SubmissionAcceptedQueue, "governed-systems", "Theo Okafor", []string{"spk_noor"}, 31},
		{"sub_gpu", "Go on the GPU: A Pragmatic Field Guide", "What works today when Go teams need GPU inference and rendering, and how to preserve a reliable fallback.", "Talk", "infrastructure", "track-infra", "Advanced", SubmissionAccepted, "open-infrastructure", "Priya Nair", []string{"spk_elliot"}, 29},
		{"sub_demo", "Just Add an Agent", "A tour of several agent demos and prompt patterns.", "Talk", "agents", "track-agents", "Introductory", SubmissionDeclined, "agents-in-production", "Maya Chen", []string{"spk_grace"}, 26},
		{"sub_portals", "Speaker Ops Without the Spreadsheet", "How an open-source program team replaced inbox follow-ups with self-service tasks, forms, resources, and clear status.", "Talk", "interfaces", "track-interfaces", "Introductory", SubmissionAccepted, "new-interfaces", "Lina Martinez", []string{"spk_ben"}, 23},
		{"sub_open_models", "Small Models, Sealed Artifacts", "A deployment pattern for deterministic, signed, weights-only model bundles in sensitive environments.", "Talk", "infrastructure", "track-infra", "Advanced", SubmissionPending, "open-infrastructure", "Priya Nair", []string{"spk_grace"}, 18},
		{"sub_accessibility", "Accessible Agent Interfaces Beyond the Chat Box", "A review of keyboard, screen reader, reduced-motion, and failure-state requirements for agentic tools.", "Panel", "interfaces", "track-interfaces", "Intermediate", SubmissionPending, "new-interfaces", "Lina Martinez", []string{"spk_noor", "spk_grace"}, 12},
	}

	result := make([]Submission, 0, len(rows))
	for _, row := range rows {
		submittedAt := now.Add(-time.Duration(row.daysAgo) * 24 * time.Hour)
		answers := map[string]string{"format": row.format, "category": row.category, "level": row.level}
		if row.format == "Workshop" {
			answers["workshop_needs"] = "Cabaret seating for 60, two handheld microphones, and reliable guest Wi-Fi."
		}
		result = append(result, Submission{
			ID: row.id, EventID: "evt_m31_forum_2026", FormID: "form_cfp_2026", Title: row.title,
			Abstract: row.abstract, Format: row.format, CategoryID: row.category, TrackID: row.track,
			Level: row.level, Tags: []string{row.category, "evidence"}, SpeakerIDs: row.speakers,
			Status: row.status, RoutedQueue: row.queue, RoutedOwner: row.owner,
			RuleTrace: []string{"category matched", "routed by rules/cfp-routing.arb", "owner assigned: " + row.owner},
			Answers:   answers, SubmittedAt: submittedAt, UpdatedAt: submittedAt.Add(4 * time.Hour),
		})
	}
	return result
}

func seedEvaluations(now time.Time) []Evaluation {
	return []Evaluation{
		{ID: "eval_001", PlanID: "plan_round_one", SubmissionID: "sub_memory", ReviewerID: "rev_ada", Scores: map[string]float64{"relevance": 5, "clarity": 4, "originality": 5, "applicability": 5}, Comments: "Exceptionally concrete and operator-focused.", Recommendation: "strong_yes", Source: "human", CreatedAt: now.Add(-11 * 24 * time.Hour), UpdatedAt: now.Add(-11 * 24 * time.Hour)},
		{ID: "eval_002", PlanID: "plan_round_one", SubmissionID: "sub_memory", ReviewerID: "rev_marcus", Scores: map[string]float64{"relevance": 5, "clarity": 5, "originality": 4, "applicability": 5}, Comments: "Clear evidence and a strong takeaway.", Recommendation: "strong_yes", Source: "human", CreatedAt: now.Add(-10 * 24 * time.Hour), UpdatedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "eval_003", PlanID: "plan_round_one", SubmissionID: "sub_rules", ReviewerID: "rev_ada", Scores: map[string]float64{"relevance": 5, "clarity": 5, "originality": 4, "applicability": 5}, Comments: "Directly advances the governance theme.", Recommendation: "yes", Source: "human", CreatedAt: now.Add(-10 * 24 * time.Hour), UpdatedAt: now.Add(-10 * 24 * time.Hour)},
		{ID: "eval_004", PlanID: "plan_round_one", SubmissionID: "sub_browser", ReviewerID: "rev_ines", Scores: map[string]float64{"relevance": 5, "clarity": 4, "originality": 5, "applicability": 4}, Comments: "Distinctive architecture with a compelling interface thesis.", Recommendation: "strong_yes", Source: "human", CreatedAt: now.Add(-9 * 24 * time.Hour), UpdatedAt: now.Add(-9 * 24 * time.Hour)},
		{ID: "eval_005", PlanID: "plan_round_one", SubmissionID: "sub_vectors", ReviewerID: "rev_marcus", Scores: map[string]float64{"relevance": 4, "clarity": 4, "originality": 4, "applicability": 5}, Comments: "Technical but grounded in measurements.", Recommendation: "yes", Source: "human", CreatedAt: now.Add(-8 * 24 * time.Hour), UpdatedAt: now.Add(-8 * 24 * time.Hour)},
		{ID: "eval_006", PlanID: "plan_round_two", SubmissionID: "sub_eval", ReviewerID: "rev_ada", Scores: map[string]float64{"program_fit": 5, "evidence": 5, "novelty": 4, "audience_value": 5}, Comments: "A useful working session and a strong complement to the review track.", Recommendation: "strong_yes", Source: "human", CreatedAt: now.Add(-2 * 24 * time.Hour), UpdatedAt: now.Add(-2 * 24 * time.Hour)},
	}
}

func seedSessions(at func(int, time.Month, int, int, int) time.Time) []Session {
	return []Session{
		{ID: "ses_memory", EventID: "evt_m31_forum_2026", SubmissionID: "sub_memory", Title: "Memory Without Mystery", Description: "Auditable context for long-running agents.", Format: "Talk", TrackID: "track-agents", RoomID: "room-meridian", SpeakerIDs: []string{"spk_maya"}, StartsAt: at(2026, time.October, 15, 9, 0), EndsAt: at(2026, time.October, 15, 9, 45), Status: "published"},
		{ID: "ses_rules", EventID: "evt_m31_forum_2026", SubmissionID: "sub_rules", Title: "Rules That Can Explain Themselves", Description: "Governed decisions without scattered conditionals.", Format: "Talk", TrackID: "track-governance", RoomID: "room-kepler", SpeakerIDs: []string{"spk_theo"}, StartsAt: at(2026, time.October, 15, 10, 0), EndsAt: at(2026, time.October, 15, 10, 45), Status: "published"},
		{ID: "ses_browser", EventID: "evt_m31_forum_2026", SubmissionID: "sub_browser", Title: "The Browser Is a Render Target", Description: "Server-first interface architecture with selective client work.", Format: "Talk", TrackID: "track-interfaces", RoomID: "room-tycho", SpeakerIDs: []string{"spk_lina"}, StartsAt: at(2026, time.October, 15, 10, 0), EndsAt: at(2026, time.October, 15, 10, 45), Status: "published"},
		{ID: "ses_maintainers", EventID: "evt_m31_forum_2026", Title: "Maintainers' Table: What We Refuse to Hide", Description: "A candid conversation about defaults, failure, and the cost of abstraction.", Format: "Panel", TrackID: "track-governance", RoomID: "room-meridian", SpeakerIDs: []string{"spk_lina", "spk_priya"}, StartsAt: at(2026, time.October, 15, 10, 30), EndsAt: at(2026, time.October, 15, 11, 15), Status: "draft"},
		{ID: "ses_vectors", EventID: "evt_m31_forum_2026", SubmissionID: "sub_vectors", Title: "Vectors at the Edge", Description: "Compression, prepared-query scoring, and portable indexes.", Format: "Talk", TrackID: "track-infra", RoomID: "room-kepler", SpeakerIDs: []string{"spk_priya"}, StartsAt: at(2026, time.October, 15, 10, 30), EndsAt: at(2026, time.October, 15, 11, 15), Status: "published"},
		{ID: "ses_gpu", EventID: "evt_m31_forum_2026", SubmissionID: "sub_gpu", Title: "Go on the GPU", Description: "Practical compiler and runtime paths with reliable fallbacks.", Format: "Talk", TrackID: "track-infra", RoomID: "room-atrium", SpeakerIDs: []string{"spk_elliot"}, StartsAt: at(2026, time.October, 15, 11, 30), EndsAt: at(2026, time.October, 15, 12, 15), Status: "published"},
		{ID: "ses_portals", EventID: "evt_m31_forum_2026", SubmissionID: "sub_portals", Title: "Speaker Ops Without the Spreadsheet", Description: "Self-service portal operations for open events.", Format: "Talk", TrackID: "track-interfaces", RoomID: "room-tycho", SpeakerIDs: []string{"spk_ben"}, StartsAt: at(2026, time.October, 15, 13, 30), EndsAt: at(2026, time.October, 15, 14, 15), Status: "published"},
		{ID: "ses_eval", EventID: "evt_m31_forum_2026", SubmissionID: "sub_eval", Title: "Evaluation Plans That Survive Reality", Description: "A working session on rubrics, rounds, and reviewer operations.", Format: "Workshop", TrackID: "track-agents", RoomID: "room-meridian", SpeakerIDs: []string{"spk_samira"}, StartsAt: at(2026, time.October, 15, 14, 30), EndsAt: at(2026, time.October, 15, 16, 0), Status: "draft"},
	}
}

func seedTaskCompletions(now time.Time) []TaskCompletion {
	completed := func(id, taskID, speakerID, status string, daysAgo int) TaskCompletion {
		stamp := now.Add(-time.Duration(daysAgo) * 24 * time.Hour)
		return TaskCompletion{ID: id, TaskID: taskID, SpeakerID: speakerID, Status: status, Values: map[string]string{}, CompletedAt: stamp, UpdatedAt: stamp}
	}
	return []TaskCompletion{
		completed("done_maya_profile", "task_profile", "spk_maya", TaskApproved, 8),
		{ID: "done_maya_headshot", TaskID: "task_headshot", SpeakerID: "spk_maya", Status: TaskApproved, FileName: "maya-chen-headshot.jpg", ContentType: "image/jpeg", CompletedAt: now.Add(-7 * 24 * time.Hour), UpdatedAt: now.Add(-7 * 24 * time.Hour)},
		completed("done_maya_agreement", "task_agreement", "spk_maya", TaskSubmitted, 4),
		completed("done_theo_profile", "task_profile", "spk_theo", TaskApproved, 9),
		{ID: "done_theo_headshot", TaskID: "task_headshot", SpeakerID: "spk_theo", Status: TaskApproved, FileName: "theo-okafor.png", ContentType: "image/png", CompletedAt: now.Add(-6 * 24 * time.Hour), UpdatedAt: now.Add(-6 * 24 * time.Hour)},
		completed("done_theo_agreement", "task_agreement", "spk_theo", TaskApproved, 3),
		completed("done_theo_av", "task_av", "spk_theo", TaskSubmitted, 1),
		completed("done_lina_profile", "task_profile", "spk_lina", TaskApproved, 10),
		completed("done_lina_agreement", "task_agreement", "spk_lina", TaskSubmitted, 5),
		{ID: "done_lina_headshot", TaskID: "task_headshot", SpeakerID: "spk_lina", Status: TaskDeclined, FileName: "lina-small.jpg", ContentType: "image/jpeg", CompletedAt: now.Add(-5 * 24 * time.Hour), UpdatedAt: now.Add(-2 * 24 * time.Hour)},
		completed("done_priya_profile", "task_profile", "spk_priya", TaskApproved, 7),
		{ID: "done_priya_headshot", TaskID: "task_headshot", SpeakerID: "spk_priya", Status: TaskApproved, FileName: "priya-nair.jpg", ContentType: "image/jpeg", CompletedAt: now.Add(-7 * 24 * time.Hour), UpdatedAt: now.Add(-7 * 24 * time.Hour)},
		completed("done_priya_agreement", "task_agreement", "spk_priya", TaskApproved, 4),
		completed("done_samira_profile", "task_profile", "spk_samira", TaskSubmitted, 2),
		completed("done_elliot_profile", "task_profile", "spk_elliot", TaskApproved, 6),
		{ID: "done_elliot_headshot", TaskID: "task_headshot", SpeakerID: "spk_elliot", Status: TaskSubmitted, FileName: "elliot-park.webp", ContentType: "image/webp", CompletedAt: now.Add(-3 * 24 * time.Hour), UpdatedAt: now.Add(-3 * 24 * time.Hour)},
		completed("done_noor_profile", "task_profile", "spk_noor", TaskApproved, 2),
		completed("done_noor_av", "task_av", "spk_noor", TaskSubmitted, 1),
		completed("done_ben_profile", "task_profile", "spk_ben", TaskApproved, 5),
		completed("done_ben_agreement", "task_agreement", "spk_ben", TaskSubmitted, 1),
	}
}

func seedCommunications(now time.Time, at func(int, time.Month, int, int, int) time.Time) []Communication {
	return []Communication{
		{ID: "comm_accept_maya", TemplateID: "tpl_acceptance", SpeakerID: "spk_maya", SessionID: "ses_memory", Subject: "You're joining M31 Systems Forum 2026", Status: "sent", Provider: "demo-outbox", SentAt: now.Add(-12 * 24 * time.Hour)},
		{ID: "comm_accept_theo", TemplateID: "tpl_acceptance", SpeakerID: "spk_theo", SessionID: "ses_rules", Subject: "You're joining M31 Systems Forum 2026", Status: "sent", Provider: "demo-outbox", SentAt: now.Add(-11 * 24 * time.Hour)},
		{ID: "comm_accept_lina", TemplateID: "tpl_acceptance", SpeakerID: "spk_lina", SessionID: "ses_browser", Subject: "You're joining M31 Systems Forum 2026", Status: "sent", Provider: "demo-outbox", SentAt: now.Add(-11 * 24 * time.Hour)},
		{ID: "comm_profile_samira", TemplateID: "tpl_five_day", SpeakerID: "spk_samira", Subject: "A speaker task is due in five days", Status: "queued", Provider: "scheduler", ScheduledFor: at(2026, time.August, 28, 9, 0)},
		{ID: "comm_profile_elliot", TemplateID: "tpl_five_day", SpeakerID: "spk_elliot", Subject: "A speaker task is due in five days", Status: "queued", Provider: "scheduler", ScheduledFor: at(2026, time.August, 28, 9, 0)},
		{ID: "comm_session_maya", TemplateID: "tpl_one_day", SpeakerID: "spk_maya", SessionID: "ses_memory", Subject: "Tomorrow: Memory Without Mystery", Status: "scheduled", Provider: "scheduler", ScheduledFor: at(2026, time.October, 14, 9, 0)},
	}
}
