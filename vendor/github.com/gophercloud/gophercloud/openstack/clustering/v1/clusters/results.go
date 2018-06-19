package clusters

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

// CreateResult is the response of a Create operations.
type CreateResult struct {
	commonResult
}

// GetResult is the response of a Get operations.
type GetResult struct {
	commonResult
}

type Cluster struct {
	Config          map[string]interface{} `json:"config"`
	CreatedAt       time.Time              `json:"-"`
	Data            map[string]interface{} `json:"data"`
	Dependents      map[string]interface{} `json:"dependents"`
	DesiredCapacity int                    `json:"desired_capacity"`
	Domain          string                 `json:"domain"`
	ID              string                 `json:"id"`
	InitAt          time.Time              `json:"-"`
	MaxSize         int                    `json:"max_size"`
	Metadata        map[string]interface{} `json:"metadata"`
	MinSize         int                    `json:"min_size"`
	Name            string                 `json:"name"`
	Nodes           []string               `json:"nodes"`
	Policies        []string               `json:"policies"`
	ProfileID       string                 `json:"profile_id"`
	ProfileName     string                 `json:"profile_name"`
	Project         string                 `json:"project"`
	Status          string                 `json:"status"`
	StatusReason    string                 `json:"status_reason"`
	Timeout         int                    `json:"timeout"`
	UpdatedAt       time.Time              `json:"-"`
	User            string                 `json:"user"`
}

func (r commonResult) Extract() (*Cluster, error) {
	var s struct {
		Cluster *Cluster `json:"cluster"`
	}
	err := r.ExtractInto(&s)
	if err != nil {
		return s.Cluster, err
	}

	return s.Cluster, nil
}

// ClusterPage contains a single page of all clusters from a List call.
type ClusterPage struct {
	pagination.LinkedPageBase
}

func (page ClusterPage) IsEmpty() (bool, error) {
	clusters, err := ExtractClusters(page)
	return len(clusters) == 0, err
}

// ExtractCluster provides access to the list of clusters in a page acquired from the List operation.
func ExtractClusters(r pagination.Page) ([]Cluster, error) {
	var s struct {
		Clusters []Cluster `json:"clusters"`
	}
	err := (r.(ClusterPage)).ExtractInto(&s)
	return s.Clusters, err
}

func (r *Cluster) UnmarshalJSON(b []byte) error {
	type tmp Cluster
	var s struct {
		tmp
		CreatedAt interface{} `json:"created_at"`
		InitAt    interface{} `json:"init_at"`
		UpdatedAt interface{} `json:"updated_at"`
	}

	err := json.Unmarshal(b, &s)
	if err != nil {
		return err
	}
	*r = Cluster(s.tmp)

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

	switch t := s.InitAt.(type) {
	case string:
		if t != "" {
			r.InitAt, err = time.Parse(gophercloud.RFC3339Milli, t)
			if err != nil {
				return err
			}
		}
	case nil:
		r.InitAt = time.Time{}
	default:
		return fmt.Errorf("Invalid type for time. type=%v", reflect.TypeOf(s.InitAt))
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
