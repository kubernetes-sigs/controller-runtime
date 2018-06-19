package actions

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/pagination"
)

// ListOpts params
type ListOpts struct {
	Limit         int    `q:"limit"`
	Marker        string `q:"marker"`
	Sort          string `q:"sort"`
	GlobalProject string `q:"global_project"`
	Name          string `q:"name"`
	Target        string `q:"target"`
	Action        string `q:"action"`
	Status        string `q:"status"`
}

// ListOptsBuilder builds query string for the ListOpts
type ListOptsBuilder interface {
	ToActionListQuery() (string, error)
}

// ToClusterListQuery builds a query string from ListOpts
func (opts ListOpts) ToActionListQuery() (string, error) {
	q, err := gophercloud.BuildQueryString(opts)
	return q.String(), err
}

// List instructs OpenStack to provide a list of actions
func List(client *gophercloud.ServiceClient, opts ListOptsBuilder) pagination.Pager {
	url := listURL(client)
	if opts != nil {
		query, err := opts.ToActionListQuery()
		if err != nil {
			return pagination.Pager{Err: err}
		}
		url += query
	}
	return pagination.NewPager(client, url, func(r pagination.PageResult) pagination.Page {
		return ActionPage{pagination.LinkedPageBase{PageResult: r}}
	})
}

// Get retrieves details of a single action. Use Extract to convert its
// result into an action id.
func Get(client *gophercloud.ServiceClient, id string) (r GetResult) {
	_, r.Err = client.Get(getURL(client, id), &r.Body, &gophercloud.RequestOpts{OkCodes: []int{200}})
	return
}
