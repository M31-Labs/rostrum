package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const CurrentSchemaVersion = 1

type State struct {
	SchemaVersion   int              `json:"schemaVersion"`
	Event           Event            `json:"event"`
	Forms           []SubmissionForm `json:"forms"`
	Speakers        []Speaker        `json:"speakers"`
	Submissions     []Submission     `json:"submissions"`
	Reviewers       []Reviewer       `json:"reviewers"`
	ReviewPlans     []ReviewPlan     `json:"reviewPlans"`
	Evaluations     []Evaluation     `json:"evaluations"`
	Sessions        []Session        `json:"sessions"`
	Tasks           []Task           `json:"tasks"`
	TaskCompletions []TaskCompletion `json:"taskCompletions"`
	Resources       []Resource       `json:"resources"`
	EmailTemplates  []EmailTemplate  `json:"emailTemplates"`
	Communications  []Communication  `json:"communications"`
	Integrations    []Integration    `json:"integrations"`
	SyncRuns        []SyncRun        `json:"syncRuns"`
	UpdatedAt       time.Time        `json:"updatedAt"`
}

type Event struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Slug        string     `json:"slug"`
	Type        string     `json:"type"`
	WebsiteURL  string     `json:"websiteUrl"`
	Location    string     `json:"location"`
	TimeZone    string     `json:"timeZone"`
	StartsAt    time.Time  `json:"startsAt"`
	EndsAt      time.Time  `json:"endsAt"`
	Theme       string     `json:"theme"`
	Description string     `json:"description"`
	LogoURL     string     `json:"logoUrl"`
	Tracks      []Track    `json:"tracks"`
	Rooms       []Room     `json:"rooms"`
	Categories  []Category `json:"categories"`
	Formats     []string   `json:"formats"`
	Levels      []string   `json:"levels"`
}

type Track struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

type Room struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Capacity int    `json:"capacity"`
}

type Category struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	OwnerName  string `json:"ownerName"`
	OwnerEmail string `json:"ownerEmail"`
	TrackID    string `json:"trackId"`
}

type SubmissionForm struct {
	ID                   string         `json:"id"`
	EventID              string         `json:"eventId"`
	Name                 string         `json:"name"`
	ExternalTitle        string         `json:"externalTitle"`
	Slug                 string         `json:"slug"`
	Kind                 string         `json:"kind"`
	Status               string         `json:"status"`
	WelcomeHeading       string         `json:"welcomeHeading"`
	WelcomeBody          string         `json:"welcomeBody"`
	CloseAt              time.Time      `json:"closeAt"`
	RedirectToPortal     bool           `json:"redirectToPortal"`
	SendConfirmation     bool           `json:"sendConfirmation"`
	ConfirmationTemplate string         `json:"confirmationTemplate"`
	RuleFile             string         `json:"ruleFile"`
	Fields               []FormField    `json:"fields"`
	QuestionRules        []QuestionRule `json:"questionRules"`
}

type FormField struct {
	ID          string   `json:"id"`
	Section     string   `json:"section"`
	Label       string   `json:"label"`
	Type        string   `json:"type"`
	Required    bool     `json:"required"`
	Locked      bool     `json:"locked"`
	Placeholder string   `json:"placeholder"`
	Help        string   `json:"help"`
	Options     []string `json:"options"`
	MaxLength   int      `json:"maxLength"`
}

type QuestionRule struct {
	ID            string `json:"id"`
	SourceFieldID string `json:"sourceFieldId"`
	Operator      string `json:"operator"`
	Value         string `json:"value"`
	TargetFieldID string `json:"targetFieldId"`
	Effect        string `json:"effect"`
	Description   string `json:"description"`
}

type Speaker struct {
	ID          string    `json:"id"`
	FirstName   string    `json:"firstName"`
	LastName    string    `json:"lastName"`
	Email       string    `json:"email"`
	Pronouns    string    `json:"pronouns"`
	Role        string    `json:"role"`
	Company     string    `json:"company"`
	Biography   string    `json:"biography"`
	HeadshotURL string    `json:"headshotUrl"`
	LinkedInURL string    `json:"linkedInUrl"`
	WebsiteURL  string    `json:"websiteUrl"`
	City        string    `json:"city"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (speaker Speaker) Name() string {
	return strings.TrimSpace(speaker.FirstName + " " + speaker.LastName)
}

func (speaker Speaker) Initials() string {
	first := firstRune(speaker.FirstName)
	last := firstRune(speaker.LastName)
	return strings.ToUpper(first + last)
}

type Submission struct {
	ID          string            `json:"id"`
	EventID     string            `json:"eventId"`
	FormID      string            `json:"formId"`
	Title       string            `json:"title"`
	Abstract    string            `json:"abstract"`
	Format      string            `json:"format"`
	CategoryID  string            `json:"categoryId"`
	TrackID     string            `json:"trackId"`
	Level       string            `json:"level"`
	Tags        []string          `json:"tags"`
	SpeakerIDs  []string          `json:"speakerIds"`
	Status      string            `json:"status"`
	RoutedQueue string            `json:"routedQueue"`
	RoutedOwner string            `json:"routedOwner"`
	RuleTrace   []string          `json:"ruleTrace"`
	Answers     map[string]string `json:"answers"`
	SubmittedAt time.Time         `json:"submittedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

const (
	SubmissionPending       = "pending"
	SubmissionAcceptedQueue = "accepted_queue"
	SubmissionAccepted      = "accepted"
	SubmissionDeclineQueue  = "decline_queue"
	SubmissionDeclined      = "declined"
)

var SubmissionStatuses = []string{
	SubmissionPending,
	SubmissionAcceptedQueue,
	SubmissionAccepted,
	SubmissionDeclineQueue,
	SubmissionDeclined,
}

type ReviewPlan struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name"`
	Round              int               `json:"round"`
	Status             string            `json:"status"`
	Instructions       string            `json:"instructions"`
	DueAt              time.Time         `json:"dueAt"`
	Anonymous          bool              `json:"anonymous"`
	WeeklyReminders    bool              `json:"weeklyReminders"`
	IncludeFiles       bool              `json:"includeFiles"`
	AIAssist           bool              `json:"aiAssist"`
	ReviewerIDs        []string          `json:"reviewerIds"`
	SubmissionIDs      []string          `json:"submissionIds"`
	Criteria           []RubricCriterion `json:"criteria"`
	EvaluationsPerItem int               `json:"evaluationsPerItem"`
}

type Reviewer struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Email     string   `json:"email"`
	Expertise []string `json:"expertise"`
	Kind      string   `json:"kind"`
}

type RubricCriterion struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Weight      float64 `json:"weight"`
	MaxScore    float64 `json:"maxScore"`
}

type Evaluation struct {
	ID             string             `json:"id"`
	PlanID         string             `json:"planId"`
	SubmissionID   string             `json:"submissionId"`
	ReviewerID     string             `json:"reviewerId"`
	Scores         map[string]float64 `json:"scores"`
	Comments       string             `json:"comments"`
	Recommendation string             `json:"recommendation"`
	Source         string             `json:"source"`
	Model          string             `json:"model"`
	CreatedAt      time.Time          `json:"createdAt"`
	UpdatedAt      time.Time          `json:"updatedAt"`
}

type Session struct {
	ID              string    `json:"id"`
	EventID         string    `json:"eventId"`
	SubmissionID    string    `json:"submissionId"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	Format          string    `json:"format"`
	TrackID         string    `json:"trackId"`
	RoomID          string    `json:"roomId"`
	SpeakerIDs      []string  `json:"speakerIds"`
	StartsAt        time.Time `json:"startsAt"`
	EndsAt          time.Time `json:"endsAt"`
	Status          string    `json:"status"`
	ExternalID      string    `json:"externalId"`
	LastPublishedAt time.Time `json:"lastPublishedAt"`
}

func (session Session) Scheduled() bool {
	return !session.StartsAt.IsZero() && !session.EndsAt.IsZero()
}

type Task struct {
	ID                 string      `json:"id"`
	Title              string      `json:"title"`
	Description        string      `json:"description"`
	Type               string      `json:"type"`
	Required           bool        `json:"required"`
	DueAt              time.Time   `json:"dueAt"`
	AssignedSpeakerIDs []string    `json:"assignedSpeakerIds"`
	FormFields         []FormField `json:"formFields"`
	AcceptedOnly       bool        `json:"acceptedOnly"`
}

type TaskCompletion struct {
	ID          string            `json:"id"`
	TaskID      string            `json:"taskId"`
	SpeakerID   string            `json:"speakerId"`
	Status      string            `json:"status"`
	Values      map[string]string `json:"values"`
	FileName    string            `json:"fileName"`
	ContentType string            `json:"contentType"`
	StoredPath  string            `json:"storedPath"`
	CompletedAt time.Time         `json:"completedAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

const (
	TaskOutstanding = "outstanding"
	TaskSubmitted   = "submitted"
	TaskApproved    = "approved"
	TaskDeclined    = "declined"
)

type Resource struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Kind      string `json:"kind"`
	Summary   string `json:"summary"`
	Body      string `json:"body"`
	URL       string `json:"url"`
	EmbedURL  string `json:"embedUrl"`
	SortOrder int    `json:"sortOrder"`
}

type EmailTemplate struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Audience       string `json:"audience"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	ReplyTo        string `json:"replyTo"`
	AttachCalendar bool   `json:"attachCalendar"`
	System         bool   `json:"system"`
}

type Communication struct {
	ID           string    `json:"id"`
	TemplateID   string    `json:"templateId"`
	SpeakerID    string    `json:"speakerId"`
	SessionID    string    `json:"sessionId"`
	Subject      string    `json:"subject"`
	Status       string    `json:"status"`
	Provider     string    `json:"provider"`
	ScheduledFor time.Time `json:"scheduledFor"`
	SentAt       time.Time `json:"sentAt"`
	Error        string    `json:"error"`
}

type Integration struct {
	ID            string    `json:"id"`
	Kind          string    `json:"kind"`
	Name          string    `json:"name"`
	Enabled       bool      `json:"enabled"`
	EventURL      string    `json:"eventUrl"`
	CredentialsOK bool      `json:"credentialsOk"`
	LastSyncAt    time.Time `json:"lastSyncAt"`
	LastStatus    string    `json:"lastStatus"`
}

type SyncRun struct {
	ID          string    `json:"id"`
	Integration string    `json:"integration"`
	Mode        string    `json:"mode"`
	Status      string    `json:"status"`
	Speakers    int       `json:"speakers"`
	Sessions    int       `json:"sessions"`
	StartedAt   time.Time `json:"startedAt"`
	FinishedAt  time.Time `json:"finishedAt"`
	Summary     string    `json:"summary"`
}

func (state *State) Form(id string) (*SubmissionForm, bool) {
	for index := range state.Forms {
		if state.Forms[index].ID == id || state.Forms[index].Slug == id {
			return &state.Forms[index], true
		}
	}
	return nil, false
}

func (state *State) Speaker(id string) (*Speaker, bool) {
	for index := range state.Speakers {
		if state.Speakers[index].ID == id {
			return &state.Speakers[index], true
		}
	}
	return nil, false
}

func (state *State) Submission(id string) (*Submission, bool) {
	for index := range state.Submissions {
		if state.Submissions[index].ID == id {
			return &state.Submissions[index], true
		}
	}
	return nil, false
}

func (state *State) Session(id string) (*Session, bool) {
	for index := range state.Sessions {
		if state.Sessions[index].ID == id {
			return &state.Sessions[index], true
		}
	}
	return nil, false
}

func (state *State) ReviewPlan(id string) (*ReviewPlan, bool) {
	for index := range state.ReviewPlans {
		if state.ReviewPlans[index].ID == id {
			return &state.ReviewPlans[index], true
		}
	}
	return nil, false
}

func (state *State) Task(id string) (*Task, bool) {
	for index := range state.Tasks {
		if state.Tasks[index].ID == id {
			return &state.Tasks[index], true
		}
	}
	return nil, false
}

func (state *State) Completion(taskID, speakerID string) (*TaskCompletion, bool) {
	for index := range state.TaskCompletions {
		completion := &state.TaskCompletions[index]
		if completion.TaskID == taskID && completion.SpeakerID == speakerID {
			return completion, true
		}
	}
	return nil, false
}

func (state State) Track(id string) (Track, bool) {
	for _, track := range state.Event.Tracks {
		if track.ID == id {
			return track, true
		}
	}
	return Track{}, false
}

func (state State) Room(id string) (Room, bool) {
	for _, room := range state.Event.Rooms {
		if room.ID == id {
			return room, true
		}
	}
	return Room{}, false
}

func (state State) Category(id string) (Category, bool) {
	for _, category := range state.Event.Categories {
		if category.ID == id {
			return category, true
		}
	}
	return Category{}, false
}

func (state State) SpeakerTasks(speakerID string) []Task {
	tasks := make([]Task, 0)
	for _, task := range state.Tasks {
		if contains(task.AssignedSpeakerIDs, speakerID) {
			tasks = append(tasks, task)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].DueAt.Before(tasks[j].DueAt) })
	return tasks
}

func (state State) OutstandingTaskCount(speakerID string) int {
	count := 0
	for _, task := range state.SpeakerTasks(speakerID) {
		completion, found := state.completionValue(task.ID, speakerID)
		if !found || completion.Status == TaskOutstanding || completion.Status == TaskDeclined {
			count++
		}
	}
	return count
}

func (state State) completionValue(taskID, speakerID string) (TaskCompletion, bool) {
	for _, completion := range state.TaskCompletions {
		if completion.TaskID == taskID && completion.SpeakerID == speakerID {
			return completion, true
		}
	}
	return TaskCompletion{}, false
}

func (state State) Validate() error {
	if state.Event.ID == "" || state.Event.Name == "" || state.Event.Slug == "" {
		return errors.New("event id, name, and slug are required")
	}
	if !state.Event.EndsAt.After(state.Event.StartsAt) {
		return errors.New("event end must be after start")
	}
	if err := validateUniqueIDs(state); err != nil {
		return err
	}
	for _, session := range state.Sessions {
		if session.Scheduled() && !session.EndsAt.After(session.StartsAt) {
			return fmt.Errorf("session %s ends before it starts", session.ID)
		}
	}
	for _, plan := range state.ReviewPlans {
		if len(plan.Criteria) == 0 {
			return fmt.Errorf("review plan %s has no rubric criteria", plan.ID)
		}
		weight := 0.0
		for _, criterion := range plan.Criteria {
			weight += criterion.Weight
		}
		if weight < 99.99 || weight > 100.01 {
			return fmt.Errorf("review plan %s rubric weight is %.2f, want 100", plan.ID, weight)
		}
	}
	return nil
}

func validateUniqueIDs(state State) error {
	seen := map[string]string{}
	groups := []struct {
		name string
		ids  []string
	}{
		{"form", collectIDs(len(state.Forms), func(i int) string { return state.Forms[i].ID })},
		{"speaker", collectIDs(len(state.Speakers), func(i int) string { return state.Speakers[i].ID })},
		{"submission", collectIDs(len(state.Submissions), func(i int) string { return state.Submissions[i].ID })},
		{"reviewer", collectIDs(len(state.Reviewers), func(i int) string { return state.Reviewers[i].ID })},
		{"review plan", collectIDs(len(state.ReviewPlans), func(i int) string { return state.ReviewPlans[i].ID })},
		{"evaluation", collectIDs(len(state.Evaluations), func(i int) string { return state.Evaluations[i].ID })},
		{"session", collectIDs(len(state.Sessions), func(i int) string { return state.Sessions[i].ID })},
		{"task", collectIDs(len(state.Tasks), func(i int) string { return state.Tasks[i].ID })},
		{"task completion", collectIDs(len(state.TaskCompletions), func(i int) string { return state.TaskCompletions[i].ID })},
		{"resource", collectIDs(len(state.Resources), func(i int) string { return state.Resources[i].ID })},
		{"email template", collectIDs(len(state.EmailTemplates), func(i int) string { return state.EmailTemplates[i].ID })},
		{"communication", collectIDs(len(state.Communications), func(i int) string { return state.Communications[i].ID })},
	}
	for _, group := range groups {
		for _, id := range group.ids {
			if id == "" {
				return fmt.Errorf("%s has an empty id", group.name)
			}
			if previous, exists := seen[id]; exists {
				return fmt.Errorf("id %s is shared by %s and %s", id, previous, group.name)
			}
			seen[id] = group.name
		}
	}
	return nil
}

func collectIDs(count int, value func(int) string) []string {
	ids := make([]string, count)
	for index := 0; index < count; index++ {
		ids[index] = value(index)
	}
	return ids
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func firstRune(value string) string {
	for _, character := range strings.TrimSpace(value) {
		return string(character)
	}
	return ""
}
