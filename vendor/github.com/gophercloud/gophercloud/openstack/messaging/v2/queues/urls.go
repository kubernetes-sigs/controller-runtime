package queues

import (
	"net/url"

	"github.com/gophercloud/gophercloud"
)

const ApiVersion = "v2"
const ApiName = "queues"

func commonURL(client *gophercloud.ServiceClient) string {
	return client.ServiceURL(ApiVersion, ApiName)
}

func createURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName)
}

func listURL(client *gophercloud.ServiceClient) string {
	return commonURL(client)
}

// builds next page full url based on current url
func nextPageURL(currentURL string, next string) (string, error) {
	base, err := url.Parse(currentURL)
	if err != nil {
		return "", err
	}
	rel, err := url.Parse(next)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(rel).String(), nil
}

func updateURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName)
}

func getURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName)
}

func deleteURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName)
}

func statURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "stats")
}

func shareURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "share")
}

func purgeURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "purge")
}
