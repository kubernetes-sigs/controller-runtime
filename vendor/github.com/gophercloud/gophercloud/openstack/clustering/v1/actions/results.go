package actions

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// commonResult is the response of a base result.
type commonResult struct {
	gophercloud.Result
}

// GetResult is the response of a Get operations.
type GetResult struct {
	commonResult
}

// ActionPage contains a single page of all actions from a ListDetails call.
type ActionPage struct {
	pagination.LinkedPageBase
}

// Action represents a Detailed Action
type Action struct {
	Action       string                 `json:"action"`
	Cause        string                 `json:"cause"`
	CreatedAt    time.Time              `json:"-"`
	Data         map[string]interface{} `json:"data"`
	DependedBy   []string               `json:"depended_by"`
	DependsOn    []string               `json:"depends_on"`
	StartTime    float64                `json:"start_time"`
	EndTime      float64                `json:"end_time"`
	ID           string                 `json:"id"`
	Inputs       map[string]interface{} `json:"inputs"`
	Interval     int                    `json:"interval"`
	Name         string                 `json:"name"`
	Outputs      map[string]interface{} `json:"outputs"`
	Owner        string                 `json:"owner"`
	Project      string                 `json:"project"`
	Status       string                 `json:"status"`
	StatusReason string                 `json:"status_reason"`
	Target       string                 `json:"target"`
	Timeout      int                    `json:"timeout"`
	UpdatedAt    time.Time              `json:"-"`
	User         string                 `json:"user"`
}

func (r commonResult) Extract() (*Action, error) {
	var s struct {
		Action *Action `json:"action"`
	}
	err := r.ExtractInto(&s)
	return s.Action, err
}

// ExtractActions provides access to the list of actions in a page acquired from the List operation.
func ExtractActions(r pagination.Page) ([]Action, error) {
	var s struct {
		Actions []Action `json:"actions"`
	}
	err := (r.(ActionPage)).ExtractInto(&s)
	return s.Actions, err
}

// IsEmpty determines if a ActionPage contains any results.
func (r ActionPage) IsEmpty() (bool, error) {
	actions, err := ExtractActions(r)
	return len(actions) == 0, err
}

func (r *Action) UnmarshalJSON(b []byte) error {
	type tmp Action
	var s struct {
		tmp
		CreatedAt interface{} `json:"created_at"`
		UpdatedAt interface{} `json:"updated_at"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = Action(s.tmp)

	switch t := s.CreatedAt.(type) {
	case string:
		if t != "" {
			r.CreatedAt, err = time.Parse(gophercloud.RFC3339Milli, t)
			if err != nil {
				return err
			}
		}
	case nil:
		r.CreatedAt = time.Time{}
	default:
		return fmt.Errorf("Invalid type for time. type=%v", reflect.TypeOf(s.CreatedAt))
	}

	switch t := s.UpdatedAt.(type) {
	case string:
		if t != "" {
			r.UpdatedAt, err = time.Parse(gophercloud.RFC3339Milli, t)
			if err != nil {
				return err
			}
		}
	case nil:
		r.UpdatedAt = time.Time{}
	default:
		return fmt.Errorf("Invalid type for time. type=%v", reflect.TypeOf(s.UpdatedAt))
	}

	return nil
}
