package api

import "time"

// Run statuses as reported by the API.
const (
	StatusPending         = "pending"
	StatusRunning         = "running"
	StatusSuccessful      = "successful"
	StatusFailed          = "failed"
	StatusPartiallyFailed = "partially_failed"
	StatusCanceled        = "canceled"
	StatusTerminated      = "terminated"
	StatusSkipped         = "skipped"
	StatusNotAllowed      = "not_allowed"
)

// Meta is the pagination information of a list.
type Meta struct {
	From    int  `json:"from"`
	To      int  `json:"to"`
	Total   int  `json:"total"`
	PerPage int  `json:"per_page"`
	HasMore bool `json:"has_more"`
}

// Page is one page of a list endpoint.
type Page[T any] struct {
	Data []T  `json:"data"`
	Meta Meta `json:"meta"`
}

// Tag is a colored label of a resource.
type Tag struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Account is the authenticated user.
type Account struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Timezone      string `json:"timezone"`
	EmailVerified bool   `json:"email_verified"`
	TwoFactor     bool   `json:"2fa_enabled"`
}

// Team groups users and resources.
type Team struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []Member `json:"members"`
}

// Member is a user of a team.
type Member struct {
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	TwoFactor bool   `json:"2fa_enabled"`
}

// Server is an SSH target.
type Server struct {
	ID        string     `json:"id"`
	TeamID    string     `json:"team_id"`
	Name      string     `json:"name"`
	Host      string     `json:"host"`
	Port      int        `json:"port"`
	Connected bool       `json:"connected"`
	Tags      []Tag      `json:"tags"`
	Metrics   *Metrics   `json:"metrics"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// Metrics is the latest measured state of a server. Every value may be missing.
type Metrics struct {
	Hostname        string     `json:"hostname"`
	OperatingSystem string     `json:"operating_system"`
	Kernel          string     `json:"kernel"`
	Architecture    string     `json:"architecture"`
	CPU             CPU        `json:"cpu"`
	Load            Load       `json:"load"`
	Memory          Usage      `json:"memory"`
	Disk            Usage      `json:"disk"`
	MeasuredAt      *time.Time `json:"measured_at"`
}

// CPU describes the processor and its usage.
type CPU struct {
	Model        string   `json:"model"`
	Cores        *int     `json:"cores"`
	UsagePercent *float64 `json:"usage_percent"`
}

// Load is the load average.
type Load struct {
	Avg1  *float64 `json:"avg_1"`
	Avg5  *float64 `json:"avg_5"`
	Avg15 *float64 `json:"avg_15"`
}

// Usage is a used/total resource like memory or disk.
type Usage struct {
	TotalBytes  *float64 `json:"total_bytes"`
	UsedBytes   *float64 `json:"used_bytes"`
	UsedPercent *float64 `json:"used_percent"`
}

// Variable is an environment variable of a server, team or account.
type Variable struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

// VariableDefinition is a variable an action or workflow needs on execution.
type VariableDefinition struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Values string `json:"values"`
}

// Variable types.
const (
	VariableText      = "text"
	VariableSelection = "selection"
)

// Action is a list of steps executed on servers.
type Action struct {
	ID        string               `json:"id"`
	TeamID    string               `json:"team_id"`
	Name      string               `json:"name"`
	Steps     []ActionStep         `json:"steps"`
	Variables []VariableDefinition `json:"variables"`
	// AllowedServers are the servers it is restricted to, nil if it may run on all servers of its team.
	AllowedServers *[]Reference `json:"allowed_servers"`
	Tags           []Tag        `json:"tags"`
	CreatedAt      *time.Time   `json:"created_at"`
	UpdatedAt      *time.Time   `json:"updated_at"`
}

// ActionStep is one step of an action.
type ActionStep struct {
	Order     int            `json:"order"`
	Type      string         `json:"type"`
	Name      string         `json:"name"`
	Execution map[string]any `json:"execution"`
}

// Workflow chains actions.
type Workflow struct {
	ID        string               `json:"id"`
	TeamID    string               `json:"team_id"`
	Name      string               `json:"name"`
	Steps     []WorkflowStep       `json:"steps"`
	Variables []VariableDefinition `json:"variables"`
	// AllowedServers are the servers it is restricted to, nil if it may run on all servers of its team.
	AllowedServers *[]Reference `json:"allowed_servers"`
	Tags           []Tag        `json:"tags"`
	CreatedAt      *time.Time   `json:"created_at"`
	UpdatedAt      *time.Time   `json:"updated_at"`
}

// WorkflowStep runs one action within a workflow.
type WorkflowStep struct {
	Order     int            `json:"order"`
	Action    *Reference     `json:"action"`
	Execution map[string]any `json:"execution"`
}

// Reference points to another resource by id and name.
type Reference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type,omitempty"`
}

// Pipe is an automation: a trigger followed by steps.
type Pipe struct {
	ID        string      `json:"id"`
	TeamID    string      `json:"team_id"`
	Name      string      `json:"name"`
	Active    bool        `json:"active"`
	Trigger   PipeTrigger `json:"trigger"`
	Steps     []PipeStep  `json:"steps"`
	Tags      []Tag       `json:"tags"`
	CreatedAt *time.Time  `json:"created_at"`
	UpdatedAt *time.Time  `json:"updated_at"`
}

// PipeTrigger starts a pipe.
type PipeTrigger struct {
	Type      string         `json:"type"`
	Event     string         `json:"event"`
	Execution map[string]any `json:"execution"`
}

// PipeStep is one step of a pipe.
type PipeStep struct {
	Order     int            `json:"order"`
	Type      string         `json:"type"`
	Event     string         `json:"event"`
	Execution map[string]any `json:"execution"`
}

// Template is a file which can be copied to servers.
type Template struct {
	ID        string     `json:"id"`
	TeamID    string     `json:"team_id"`
	Name      string     `json:"name"`
	FileName  string     `json:"file_name"`
	Content   string     `json:"content"`
	Tags      []Tag      `json:"tags"`
	CreatedAt *time.Time `json:"created_at"`
	UpdatedAt *time.Time `json:"updated_at"`
}

// Run is one execution of an action, workflow or pipe.
type Run struct {
	ID          string      `json:"id"`
	TeamID      string      `json:"team_id"`
	Status      string      `json:"status"`
	Executable  *Reference  `json:"executable"`
	TriggeredBy *Person     `json:"triggered_by"`
	Executions  []Execution `json:"executions"`
	Runs        []string    `json:"runs"`
	CreatedAt   *time.Time  `json:"created_at"`
	StartedAt   *time.Time  `json:"started_at"`
	FinishedAt  *time.Time  `json:"finished_at"`
}

// Finished tells whether the run is over.
func (r *Run) Finished() bool {
	return r.FinishedAt != nil
}

// Person is a user referenced by name.
type Person struct {
	Name string `json:"name"`
}

// Execution is the run on one server.
type Execution struct {
	ID         string              `json:"id"`
	RunID      string              `json:"run_id"`
	Server     ServerRef           `json:"server"`
	Status     string              `json:"status"`
	Steps      []Step              `json:"steps"`
	Variables  []ExecutionVariable `json:"variables"`
	StartedAt  *time.Time          `json:"started_at"`
	FinishedAt *time.Time          `json:"finished_at"`
}

// ServerRef is the short description of a server.
type ServerRef struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Host      string `json:"host"`
	Connected bool   `json:"connected"`
}

// ExecutionVariable is a variable value used by an execution, secrets have no value.
type ExecutionVariable struct {
	Name   string  `json:"name"`
	Value  *string `json:"value"`
	Secret bool    `json:"secret"`
}

// Step types.
const (
	StepAction   = "ActionStep"
	StepWorkflow = "WorkflowStep"
)

// Step is the result of a workflow step or an action step.
type Step struct {
	Type                string       `json:"type"`
	Name                string       `json:"name"`
	Status              string       `json:"status"`
	Reason              *string      `json:"reason"`
	SkippedByConditions bool         `json:"skipped_by_conditions"`
	StreamID            *string      `json:"stream_id"`
	Steps               []Step       `json:"steps"`
	Details             *StepDetails `json:"details"`
	StartedAt           *time.Time   `json:"started_at"`
	FinishedAt          *time.Time   `json:"finished_at"`
}

// Stream returns the id of the output stream or an empty string.
func (s *Step) Stream() string {
	if s.StreamID == nil {
		return ""
	}
	return *s.StreamID
}

// StepDetails is the command and output of an action step.
type StepDetails struct {
	ExitCode *int   `json:"exit_code"`
	Type     string `json:"type"`
	Command  string `json:"command"`
	Output   string `json:"output"`
}

// RunRequest starts an action or a workflow.
type RunRequest struct {
	ActionID   string            `json:"action_id,omitempty"`
	WorkflowID string            `json:"workflow_id,omitempty"`
	ServerIDs  []string          `json:"server_ids"`
	Variables  map[string]string `json:"variables,omitempty"`
}

// Ticket authenticates a connection to the gateway.
type Ticket struct {
	URL       string     `json:"url"`
	Ticket    string     `json:"ticket"`
	ExpiresAt *time.Time `json:"expires_at"`
}

// Session is a terminal session with the attach ticket of the current user.
type Session struct {
	ID     string    `json:"id"`
	Server ServerRef `json:"server"`
	Owner  string    `json:"owner"`
	Role   string    `json:"role"`
	Ticket string    `json:"ticket"`
}

// Status is the authentication state of the token.
type Status struct {
	Authenticated bool   `json:"authenticated"`
	Time          string `json:"time"`
}
