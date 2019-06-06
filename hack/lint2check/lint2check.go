/*
Copyright 2019 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"os"
	"log"
	"go/token"
	"net/http"
	"strings"
	"time"
	"fmt"

	"github.com/google/go-github/v25/github"
)

type lintResults struct {
	Issues []issue
	Report reportData
}

type issue struct {
	FromLinter string
	Text string
	Pos token.Position

	LineRange *lineRange
	Replacement *replacement
}

type replacement struct {
	NeedOnlyDelete bool
	NewLines []string
}

type lineRange struct {
	From, To int
}

type reportData struct {
	Warnings []warning
	Linters []linterData
	Error string
}

type linterData struct {
	Name string
	Enabled bool
}

type warning struct {
	Tag string
	Text string
}

type bearerTransport struct {
	token string
}

func (b *bearerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+b.token)
	req.Header.Set("Accept", "application/vnd.github.antiope-preview+json")

	return http.DefaultTransport.RoundTrip(req)
}

func main() {
	if !lintAndSubmit() {
		os.Exit(1)
	}
}

func lintAndSubmit() (succeeded bool) {
	ctx := context.Background()
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Print("must specify a GitHub token in $GITHUB_TOKEN")
		return false
	}
	client := github.NewClient(&http.Client{Transport: &bearerTransport{
		token: token,
	}})

	headSha := os.Getenv("GITHUB_SHA")
	if headSha == "" {
		log.Print("must specify a SHA to register the check against in $GITHUB_SHA")
		return false
	}

	repoRaw := os.Getenv("GITHUB_REPOSITORY")
	if repoRaw == "" || !strings.Contains(repoRaw, "/") {
		log.Print("must specify a repository in $GITHUB_REPOSITORY as owner/repo")
		return false
	}
	repoParts := strings.SplitN(repoRaw, "/", 2)
	repoOwner := repoParts[0]
	repoName := repoParts[1]

	defInProgress := "in_progress"
	checkRun, _, err := client.Checks.CreateCheckRun(ctx, repoOwner, repoName, github.CreateCheckRunOptions{
		Name: "Linters",
		HeadSHA: headSha,
		Status: &defInProgress,
		StartedAt: &github.Timestamp{Time: time.Now()},
	})
	if err != nil {
		log.Print(err)
		return false
	}

	succeeded = true

	if err := runLints(checkRun); err != nil {
		// don't return immediately -- submit things first
		log.Print(err)
		succeeded = false
	}

	defUnknownErr := "**unknown error while linting**"
	if checkRun.Output.Title == nil || checkRun.Output.Summary == nil {
		checkRun.Output.Title = &defUnknownErr
		checkRun.Output.Summary = &defUnknownErr
	}

	log.Printf("Sending check run results %+v", checkRun)
	_, _, err = client.Checks.UpdateCheckRun(ctx, repoOwner, repoName, *checkRun.ID, github.UpdateCheckRunOptions{
		Name: *checkRun.Name,
		Status: checkRun.Status,
		Conclusion: checkRun.Conclusion,
		CompletedAt: checkRun.CompletedAt,
		Output: checkRun.Output,
	})
	if err != nil {
		log.Print(err)
		return false
	}

	log.Print("done")
	return succeeded
}

func runLints(checkRun *github.CheckRun) error {
	// TODO(directxman12); there's probably a way to run this directly,
	// but this is easiest for now
	args := append([]string{"run", "--out-format", "json"}, os.Args[1:]...)
	lintOutRaw, checkErrRaw := exec.Command("golangci-lint", args...).Output()
	// don't return early, since we might get results
	if checkErrRaw != nil {
		log.Print(checkErrRaw)
		if exitErr, isExitErr := checkErrRaw.(*exec.ExitError); isExitErr {
			log.Print(string(exitErr.Stderr))
		}
	}

	// set the completed time so we have a timestamp in case of error return
	checkRun.CompletedAt = &github.Timestamp{Time: time.Now()}

	var lintRes lintResults
	if err := json.Unmarshal(lintOutRaw, &lintRes); err != nil {
		defFailure := "failure"
		checkRun.Conclusion = &defFailure
		return err
	}

	conclusion := "success"
	summary := fmt.Sprintf("%v problems\n\n%v warnings", len(lintRes.Issues), len(lintRes.Report.Warnings))
	switch {
	case len(lintRes.Issues) > 0 || checkErrRaw != nil || lintRes.Report.Error != "":
		conclusion = "failure"
	case len(lintRes.Report.Warnings) > 0:
		conclusion = "neutral"
	}
	checkRun.Conclusion = &conclusion

	if lintRes.Report.Error != "" {
		summary += fmt.Sprintf("\n\nError running linters: %s", lintRes.Report.Error)
	}
	defTitle := "Linter Runs"
	checkRun.Output.Title = &defTitle
	checkRun.Output.Summary = &summary

	var linterLines []string
	for _, linter := range lintRes.Report.Linters {
		if !linter.Enabled {
			continue
		}
		linterLines = append(linterLines, "- "+linter.Name)
	}
	details := fmt.Sprintf("## Enabled Linters\n\n%s\n", strings.Join(linterLines, "\n"))

	if len(lintRes.Report.Warnings) > 0 {
		var warningLines []string
		for _, warning := range lintRes.Report.Warnings {
			warningLines = append(warningLines, fmt.Sprintf("- *%s*: %s", warning.Tag, warning.Text))
		}
		details += fmt.Sprintf("## Warnings\n\n%s\n", strings.Join(warningLines, "\n"))
	}

	checkRun.Output.Text = &details

	var annotations []*github.CheckRunAnnotation
	for i := range lintRes.Issues {
		// don't take references to the iteration variable
		issue := lintRes.Issues[i]
		defFailure := "failure"
		issueDetails := ""

		if issue.Replacement != nil {
			if issue.Replacement.NeedOnlyDelete {
				issueDetails = "*delete these lines*"
			} else {
				issueDetails = fmt.Sprintf("*replace these lines with*:\n\n```go\n%s\n```", strings.Join(issue.Replacement.NewLines, "\n"))
			}
		}

		annot := &github.CheckRunAnnotation{
			Path: &issue.Pos.Filename,
			AnnotationLevel: &defFailure,
			Message: &issue.Text,
			Title: &issue.FromLinter,
			RawDetails: &issueDetails,
		}

		if issue.LineRange != nil {
			annot.StartLine = &issue.LineRange.From
			annot.EndLine = &issue.LineRange.To
		} else {
			annot.StartLine = &issue.Pos.Line
			annot.EndLine = &issue.Pos.Line
			// TODO(directxman12): go-github doesn't support columns yet,
			// re-add this when they do
			// annot.StartColumn = &issue.Pos.Column
			// annot.EndColumn = &issue.Pos.Column
		}

		annotations = append(annotations, annot)
	}

	checkRun.Output.Annotations = annotations
	return checkErrRaw
}
