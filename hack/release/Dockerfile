FROM k8s.gcr.io/debian-base:v1.0.0

LABEL com.github.actions.name="KubeBuilder PR Emoji"
LABEL com.github.actions.name="Verify that KubeBuilder release notes emoji are present on the PR"
LABEL com.github.actions.icon="git-pull-request"
LABEL com.github.actions.color="blue"

RUN apt-get update -y && apt-get install -y bash jq curl

COPY common.sh /common.sh
COPY verify-emoji.sh /verify-emoji.sh

ENTRYPOINT ["/verify-emoji.sh"]
