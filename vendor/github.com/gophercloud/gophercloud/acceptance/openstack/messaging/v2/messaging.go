package v2

import (
	"testing"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/acceptance/tools"
	"github.com/gophercloud/gophercloud/openstack/messaging/v2/messages"
	"github.com/gophercloud/gophercloud/openstack/messaging/v2/queues"
	"github.com/gophercloud/gophercloud/pagination"
)

func CreateQueue(t *testing.T, client *gophercloud.ServiceClient) (string, error) {
	queueName := tools.RandomString("ACPTTEST", 5)

	t.Logf("Attempting to create Queue: %s", queueName)

	createOpts := queues.CreateOpts{
		QueueName:                  queueName,
		MaxMessagesPostSize:        262143,
		DefaultMessageTTL:          3700,
		DefaultMessageDelay:        25,
		DeadLetterQueueMessagesTTL: 3500,
		MaxClaimCount:              10,
		Extra:                      map[string]interface{}{"description": "Test Queue for Gophercloud acceptance tests."},
	}

	createErr := queues.Create(client, createOpts).ExtractErr()
	if createErr != nil {
		t.Fatalf("Unable to create Queue: %v", createErr)
	}

	GetQueue(t, client, queueName)

	t.Logf("Created Queue: %s", queueName)
	return queueName, nil
}

func DeleteQueue(t *testing.T, client *gophercloud.ServiceClient, queueName string) {
	t.Logf("Attempting to delete Queue: %s", queueName)
	err := queues.Delete(client, queueName).ExtractErr()
	if err != nil {
		t.Fatalf("Unable to delete Queue %s: %v", queueName, err)
	}

	t.Logf("Deleted Queue: %s", queueName)
}

func GetQueue(t *testing.T, client *gophercloud.ServiceClient, queueName string) (queues.QueueDetails, error) {
	t.Logf("Attempting to get Queue: %s", queueName)
	queue, err := queues.Get(client, queueName).Extract()
	if err != nil {
		t.Fatalf("Unable to get Queue %s: %v", queueName, err)
	}
	return queue, nil
}

func CreateShare(t *testing.T, client *gophercloud.ServiceClient, queueName string) (queues.QueueShare, error) {
	t.Logf("Attempting to create share for queue: %s", queueName)

	shareOpts := queues.ShareOpts{
		Paths:   []queues.SharePath{queues.PathMessages},
		Methods: []queues.ShareMethod{queues.MethodPost},
	}

	share, err := queues.Share(client, queueName, shareOpts).Extract()

	return share, err
}

func CreateMessage(t *testing.T, client *gophercloud.ServiceClient, queueName string) (messages.ResourceList, error) {
	t.Logf("Attempting to add message to Queue: %s", queueName)
	createOpts := messages.BatchCreateOpts{
		messages.CreateOpts{
			TTL:  300,
			Body: map[string]interface{}{"Key": tools.RandomString("ACPTTEST", 8)},
		},
	}

	resource, err := messages.Create(client, queueName, createOpts).Extract()
	if err != nil {
		t.Fatalf("Unable to add message to queue %s: %v", queueName, err)
	} else {
		t.Logf("Successfully added message to queue: %s", queueName)
	}

	return resource, err
}

func ListMessages(t *testing.T, client *gophercloud.ServiceClient, queueName string) ([]messages.Message, error) {
	listOpts := messages.ListOpts{}
	var allMessages []messages.Message
	var listErr error

	t.Logf("Attempting to list messages on queue: %s", queueName)
	pager := messages.List(client, queueName, listOpts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		allMessages, listErr = messages.ExtractMessages(page)
		if listErr != nil {
			t.Fatalf("Unable to extract messages: %v", listErr)
		}

		for _, message := range allMessages {
			tools.PrintResource(t, message)
		}

		return true, nil
	})
	return allMessages, err
}
