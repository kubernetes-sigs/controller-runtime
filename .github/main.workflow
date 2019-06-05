workflow "PR Checks" {
    on = "pull_request"
    resolves = ["verify-emoji"]
}

action "verify-emoji" {
    uses = "kubernetes-sigs/controller-runtime/hack/release@actions-test"
    secrets = ["GITHUB_TOKEN"]
}
