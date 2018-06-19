package profiles

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// commonResult is the response of a base result.
type commonResult struct {
	gophercloud.Result
}

// CreateResult is the response of a Create operation.
type CreateResult struct {
	commonResult
}

// GetResult is the response of a Get operations.
type GetResult struct {
	commonResult
}

// Extract provides access to Profile returned by the Get and Create functions.
func (r commonResult) Extract() (*Profile, error) {
	var s struct {
		Profile *Profile `json:"profile"`
	}
	err := r.ExtractInto(&s)
	return s.Profile, err
}

// ExtractProfiles provides access to the list of profiles in a page from the List operation.
func ExtractProfiles(r pagination.Page) ([]Profile, error) {
	var s struct {
		Profiles []Profile `json:"profiles"`
	}
	err := (r.(ProfilePage)).ExtractInto(&s)
	return s.Profiles, err
}

type Spec struct {
	Type       string                 `json:"type"`
	Version    string                 `json:"version"`
	Properties map[string]interface{} `json:"properties"`
}

// Profile represent a detailed profile
type Profile struct {
	CreatedAt time.Time              `json:"-"`
	Domain    string                 `json:"domain"`
	ID        string                 `json:"id"`
	Metadata  map[string]interface{} `json:"metadata"`
	Name      string                 `json:"name"`
	Project   string                 `json:"project"`
	Spec      Spec                   `json:"spec"`
	Type      string                 `json:"type"`
	UpdatedAt time.Time              `json:"-"`
	User      string                 `json:"user"`
}

type ProfilePage struct {
	pagination.LinkedPageBase
}

// IsEmpty determines if a ProfilePage contains any results.
func (page ProfilePage) IsEmpty() (bool, error) {
	profiles, err := ExtractProfiles(page)
	return len(profiles) == 0, err
}

func (r *Spec) UnmarshalJSON(b []byte) error {
	type tmp Spec
	var s struct {
		tmp
		Version interface{} `json:"version"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = Spec(s.tmp)

	switch s.Version.(type) {
	case nil:
		r.Version = ""
	case float32, float64, int, int32, int64:
		r.Version = strconv.FormatFloat(s.Version.(float64), 'f', -1, 64)
		if !strings.Contains(r.Version, ".") {
			r.Version = strconv.FormatFloat(s.Version.(float64), 'f', 1, 64)
		}
	case string:
		r.Version = s.Version.(string)
	default:
		return fmt.Errorf("Invalid type for Spec Version. type=%v", reflect.TypeOf(s.Version))
	}

	return nil
}

func (r *Profile) UnmarshalJSON(b []byte) error {
	type tmp Profile
	var s struct {
		tmp
		CreatedAt interface{} `json:"created_at"`
		UpdatedAt interface{} `json:"updated_at"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = Profile(s.tmp)

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
