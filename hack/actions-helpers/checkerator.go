package main

import (
	"os"
	"context"
	"log"
	"net/http"
	"io/ioutil"
	"encoding/json"

	"github.com/google/go-github/v29/github"
	"golang.org/x/oauth2"
)

func tokenClientFor(ctx context.Context, token string) *http.Client {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	return oauth2.NewClient(ctx, tokenSource)
}

func main() {
	ctx := context.Background()

	// load the event data
	ghEvt := os.Getenv("GITHUB_EVENT_NAME")
	if ghEvt == "" {
		log.Fatal("unable to figure out event that triggered this execution (check GITHUB_EVENT_NAME)")
	}
	ghEvtDataPath := os.Getenv("GITHUB_EVENT_PATH")
	if ghEvtDataPath == "" {
		log.Fatal("unable to figure out path to GitHub event data (check GITHUB_EVENT_PATH)")
	}

	rawEventData, err := ioutil.ReadFile(ghEvtDataPath)
	if err != nil {
		log.Fatalf("unable to read GitHub event data: %v", err)
	}
	var eventData github.Event
	if err := json.Unmarshal(rawEventData, &eventData); err != nil {
		log.Fatalf("unable to unmarshal GitHub event data: %v", err)
	}

	// set up the client
	ghToken := os.Getenv("INPUT_GITHUB_TOKEN")
	if ghToken == "" {
		log.Fatal("no GitHub access token defined (specify INPUT_GITHUB_TOKEN via `with: {github_token: <the secret syntax>}`)")
	}
	_ = github.NewClient(tokenClientFor(ctx, ghToken))

	if eventData.Type == nil {
		log.Fatal("GitHub event data didn't list a type:\n%s", string(rawEventData))
	}
	switch *eventData.Type {
	case "check_suite":
		log.Printf("check suite:\n%s", rawEventData)
	case "check_run":
		log.Printf("check run:\n%s", rawEventData)
	case "pull_request":
		log.Printf("pull request:\n%s", rawEventData)
		// TODO
	default:
		log.Fatalf("unknown event type %q in GitHub event data", *eventData.Type)
	}
}
