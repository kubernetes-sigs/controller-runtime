package messages

import (
	"net/url"

	"github.com/gophercloud/gophercloud"
)

const ApiVersion = "v2"
const ApiName = "queues"

func createURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages")
}

func listURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages")
}

func getURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages")
}

// Builds next page full url based on current url.
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

func deleteURL(client *gophercloud.ServiceClient, queueName string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages")
}

func DeleteMessageURL(client *gophercloud.ServiceClient, queueName string, messageID string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages", messageID)
}

func messageURL(client *gophercloud.ServiceClient, queueName string, messageID string) string {
	return client.ServiceURL(ApiVersion, ApiName, queueName, "messages", messageID)
}
