workflow "PR Checks" {
    on = "pull_request"
    resolves = ["verify-emoji"]
}

action "verify-emoji" {
    uses = "./hack/release"
    secrets = ["GITHUB_TOKEN"]
}
