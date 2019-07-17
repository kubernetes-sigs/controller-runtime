workflow "PR Checks" {
    on = "pull_request"
    resolves = ["verify-emoji"]
}

workflow "Linters and Test" {
    on = "push"
    resolves = ["lint"]
}


action "verify-emoji" {
    uses = "./hack/release"
    secrets = ["GITHUB_TOKEN"]
}

action "lint" {
    uses = "docker://gcr.io/kubebuilder/lint2check"
    secrets = ["GITHUB_TOKEN"]
    args = ["./pkg/...", "./examples/..."]
}